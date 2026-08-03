package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

func TestKeyFormModel_Submit_GeneratesEd25519Key(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newKeyFormModel(keySvc)
	m.label.SetValue("minha chave")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava tea.Cmd de submissão")
	}
	results := resolveMsgs(startAsyncCmdFor(t, cmd))
	generated, ok := findKeyGenerated(results)
	if !ok {
		t.Fatal("esperava keyGeneratedMsg")
	}
	if generated.err != nil {
		t.Fatalf("GenerateKey: %v", generated.err)
	}
	if generated.key.Metadata.Label != "minha chave" {
		t.Fatalf("esperava label 'minha chave', obteve %q", generated.key.Metadata.Label)
	}

	next, popCmd := m.Update(generated)
	if next != screen(m) {
		t.Fatal("esperava permanecer no mesmo model após sucesso (só o Cmd navega)")
	}
	if popCmd == nil {
		t.Fatal("esperava popScreenMsg após sucesso")
	}
	if _, ok := popCmd().(popScreenMsg); !ok {
		t.Fatal("esperava popScreenMsg após GenerateKey bem-sucedido")
	}
}

func TestKeyFormModel_PassphraseMismatch_BlocksSubmit(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newKeyFormModel(keySvc)
	m.passphrase.SetValue("abc123")
	m.passphraseConfirm.SetValue("diferente")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("não deveria disparar GenerateKey com passphrases divergentes")
	}
	if m.validationErr == nil {
		t.Fatal("esperava validationErr definido")
	}
}

func TestKeyFormModel_InvalidRSABits_BlocksSubmit(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newKeyFormModel(keySvc)
	m.algorithm = core.AlgorithmRSA
	m.rsaBits.SetValue("1024")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("não deveria disparar GenerateKey com RSABits < 4096")
	}
	if m.validationErr == nil {
		t.Fatal("esperava validationErr definido")
	}
}

func TestKeyFormModel_OverwriteConflict_RequestsConfirm(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	if _, err := keySvc.GenerateKey(core.GenerateKeyOptions{FileName: "id_ed25519"}); err != nil {
		t.Fatalf("GenerateKey inicial: %v", err)
	}

	m := newKeyFormModel(keySvc)
	m.fileName.SetValue("id_ed25519")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	results := resolveMsgs(startAsyncCmdFor(t, cmd))
	generated, ok := findKeyGenerated(results)
	if !ok {
		t.Fatal("esperava keyGeneratedMsg")
	}
	if generated.err == nil {
		t.Fatal("esperava erro de arquivo já existente")
	}

	_, popOrConfirmCmd := m.Update(generated)
	if popOrConfirmCmd == nil {
		t.Fatal("esperava requestConfirmMsg")
	}
	msg := popOrConfirmCmd()
	confirmMsg, ok := msg.(requestConfirmMsg)
	if !ok {
		t.Fatalf("esperava requestConfirmMsg, obteve %T", msg)
	}
	if !confirmMsg.danger {
		t.Fatal("conflito de sobrescrita deveria ser danger=true")
	}
}

func TestKeyFormModel_Esc_PopsScreen(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newKeyFormModel(keySvc)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esperava popScreenMsg")
	}
	if _, ok := cmd().(popScreenMsg); !ok {
		t.Fatal("esperava popScreenMsg")
	}
}

func TestKeyFormModel_EnterOnNotes_DoesNotSubmit(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newKeyFormModel(keySvc)
	m.setFocus(fieldNotes)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(startAsyncMsg); ok {
			t.Fatal("enter em Notes não deveria submeter o formulário")
		}
	}
}

func findKeyGenerated(msgs []tea.Msg) (keyGeneratedMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(keyGeneratedMsg); ok {
			return m, true
		}
	}
	return keyGeneratedMsg{}, false
}
