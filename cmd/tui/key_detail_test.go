package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

func TestKeyDetailModel_Init_LoadsKey(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	generated, err := keySvc.GenerateKey(core.GenerateKeyOptions{Label: "minha chave"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	m := newKeyDetailModel(keySvc, generated.Metadata.Fingerprint)
	results := resolveMsgs(startAsyncCmdFor(t, m.Init()))
	loaded, ok := findKeyDetailLoaded(results)
	if !ok {
		t.Fatal("esperava keyDetailLoadedMsg")
	}
	if loaded.err != nil {
		t.Fatalf("GetKey: %v", loaded.err)
	}

	m.Update(loaded)
	if !m.loaded || m.key.Metadata.Label != "minha chave" {
		t.Fatalf("esperava chave carregada com label 'minha chave', obteve %+v", m.key)
	}
}

func TestKeyDetailModel_NotFound_PopsToList(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newKeyDetailModel(keySvc, "SHA256:inexistente")

	results := resolveMsgs(startAsyncCmdFor(t, m.Init()))
	loaded, ok := findKeyDetailLoaded(results)
	if !ok {
		t.Fatal("esperava keyDetailLoadedMsg")
	}
	if loaded.err == nil {
		t.Fatal("esperava ErrKeyNotFound")
	}

	_, cmd := m.Update(loaded)
	if cmd == nil {
		t.Fatal("esperava tea.Cmd (batch de errMsg + popScreenMsg)")
	}
	msgs := resolveMsgs(cmd)
	foundPop := false
	for _, msg := range msgs {
		if _, ok := msg.(popScreenMsg); ok {
			foundPop = true
		}
	}
	if !foundPop {
		t.Fatal("esperava popScreenMsg ao não encontrar a chave")
	}
}

func TestKeyDetailModel_Unregister_RequestsConfirmation(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	generated, err := keySvc.GenerateKey(core.GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	m := newKeyDetailModel(keySvc, generated.Metadata.Fingerprint)
	loaded, _ := findKeyDetailLoaded(resolveMsgs(startAsyncCmdFor(t, m.Init())))
	m.Update(loaded)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if cmd == nil {
		t.Fatal("esperava requestConfirmMsg")
	}
	confirmMsg, ok := cmd().(requestConfirmMsg)
	if !ok {
		t.Fatalf("esperava requestConfirmMsg, obteve %T", confirmMsg)
	}

	result := resolveMsgs(startAsyncCmdFor(t, confirmMsg.onConfirm))
	var unregistered keyUnregisteredMsg
	found := false
	for _, msg := range result {
		if u, ok := msg.(keyUnregisteredMsg); ok {
			unregistered, found = u, true
		}
	}
	if !found {
		t.Fatal("esperava keyUnregisteredMsg após confirmar")
	}
	if unregistered.err != nil {
		t.Fatalf("Unregister: %v", unregistered.err)
	}

	if _, err := keySvc.GetKey(generated.Metadata.Fingerprint); err == nil {
		t.Fatal("esperava ErrKeyNotFound após unregister (metadata removida)")
	}
}

func TestKeyDetailModel_Edit_PushesMetadataForm(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	generated, err := keySvc.GenerateKey(core.GenerateKeyOptions{Label: "original"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	m := newKeyDetailModel(keySvc, generated.Metadata.Fingerprint)
	loaded, _ := findKeyDetailLoaded(resolveMsgs(startAsyncCmdFor(t, m.Init())))
	m.Update(loaded)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cmd == nil {
		t.Fatal("esperava pushScreenMsg")
	}
	push, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", push)
	}
	form, ok := push.screen.(*keyMetadataFormModel)
	if !ok {
		t.Fatalf("esperava *keyMetadataFormModel, obteve %T", push.screen)
	}
	if form.label.Value() != "original" {
		t.Fatalf("esperava formulário pré-preenchido com 'original', obteve %q", form.label.Value())
	}
}

func TestKeyMetadataFormModel_Submit_UpdatesLabelAndClearsExpiration(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	generated, err := keySvc.GenerateKey(core.GenerateKeyOptions{Label: "original"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	m := newKeyMetadataFormModel(keySvc, generated)
	m.label.SetValue("atualizado")
	// expiresAt já está vazio (chave sem expiração) e origExpiresAt também
	// é "" — então o campo permanece "intocado", não deve tentar limpar.

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava tea.Cmd de submissão")
	}
	results := resolveMsgs(startAsyncCmdFor(t, cmd))
	updated, ok := findKeyMetadataUpdated(results)
	if !ok {
		t.Fatal("esperava keyMetadataUpdatedMsg")
	}
	if updated.err != nil {
		t.Fatalf("UpdateMetadata: %v", updated.err)
	}
	if updated.key.Metadata.Label != "atualizado" {
		t.Fatalf("esperava label 'atualizado', obteve %q", updated.key.Metadata.Label)
	}
}

func TestKeyRegisterFormModel_Submit_RegistersOrphan(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	generated, err := keySvc.GenerateKey(core.GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := keySvc.Unregister(generated.Metadata.Fingerprint); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	m := newKeyRegisterFormModel(keySvc, generated.PrivateKeyPath)
	m.label.SetValue("registrada de novo")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava tea.Cmd de submissão")
	}
	results := resolveMsgs(startAsyncCmdFor(t, cmd))
	registered, ok := findKeyRegistered(results)
	if !ok {
		t.Fatal("esperava keyRegisteredMsg")
	}
	if registered.err != nil {
		t.Fatalf("RegisterKey: %v", registered.err)
	}
	if registered.key.Metadata.Label != "registrada de novo" {
		t.Fatalf("esperava label 'registrada de novo', obteve %q", registered.key.Metadata.Label)
	}
}

func findKeyDetailLoaded(msgs []tea.Msg) (keyDetailLoadedMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(keyDetailLoadedMsg); ok {
			return m, true
		}
	}
	return keyDetailLoadedMsg{}, false
}

func findKeyMetadataUpdated(msgs []tea.Msg) (keyMetadataUpdatedMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(keyMetadataUpdatedMsg); ok {
			return m, true
		}
	}
	return keyMetadataUpdatedMsg{}, false
}

func findKeyRegistered(msgs []tea.Msg) (keyRegisteredMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(keyRegisteredMsg); ok {
			return m, true
		}
	}
	return keyRegisteredMsg{}, false
}
