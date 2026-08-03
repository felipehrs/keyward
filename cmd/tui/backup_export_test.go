package main

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

func newLoadedExportModel(t *testing.T) *backupExportModel {
	t.Helper()
	configSvc, keySvc, backupSvc := newTestServices(t)
	if err := configSvc.AddHost("", core.HostSpec{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	if _, err := keySvc.GenerateKey(core.GenerateKeyOptions{Label: "minha chave"}); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	m := newBackupExportModel(configSvc, keySvc, backupSvc)
	loaded, ok := findBackupSelectionLoaded(resolveMsgs(startAsyncCmdFor(t, m.Init())))
	if !ok {
		t.Fatal("esperava backupSelectionLoadedMsg")
	}
	if loaded.err != nil {
		t.Fatalf("reload: %v", loaded.err)
	}
	m.Update(loaded)
	return m
}

func TestBackupExportModel_Init_LoadsHostsAndKeys(t *testing.T) {
	m := newLoadedExportModel(t)
	if !m.loaded || len(m.hosts) != 1 || len(m.keys) != 1 {
		t.Fatalf("esperava 1 host e 1 chave carregados, obteve hosts=%d keys=%d", len(m.hosts), len(m.keys))
	}
}

func TestBackupExportModel_ToggleAndPrivateFlag(t *testing.T) {
	m := newLoadedExportModel(t)

	// cursor 0 = host; espaço inclui.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.hosts[0].included {
		t.Fatal("esperava host incluído após espaço")
	}

	// move pro item de chave (índice 1) e inclui.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.keys[0].included {
		t.Fatal("esperava chave incluída após espaço")
	}
	if m.hasPrivateSelected() {
		t.Fatal("não deveria ter privada selecionada ainda")
	}

	// 'p' alterna incluir privada.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if !m.keys[0].includePrivate || !m.hasPrivateSelected() {
		t.Fatal("esperava includePrivate=true após 'p'")
	}

	// desmarcar a chave também zera includePrivate.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.keys[0].includePrivate {
		t.Fatal("desmarcar a chave deveria zerar includePrivate")
	}
}

func TestBackupExportModel_Submit_EmptyDest_BlocksSubmit(t *testing.T) {
	m := newLoadedExportModel(t)
	m.cursor = m.destPathRow()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("não deveria submeter sem destino")
	}
	if m.validationErr == nil {
		t.Fatal("esperava validationErr definido")
	}
}

func TestBackupExportModel_Submit_WithPrivateKey_RequestsDangerConfirm(t *testing.T) {
	m := newLoadedExportModel(t)
	m.keys[0].included = true
	m.keys[0].includePrivate = true
	dest := filepath.Join(t.TempDir(), "backup.tar.gz")
	m.destPath.SetValue(dest)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava requestConfirmMsg")
	}
	confirmMsg, ok := cmd().(requestConfirmMsg)
	if !ok {
		t.Fatalf("esperava requestConfirmMsg, obteve %T", confirmMsg)
	}
	if !confirmMsg.danger {
		t.Fatal("export com chave privada deveria ser danger=true")
	}

	results := resolveMsgs(startAsyncCmdFor(t, confirmMsg.onConfirm))
	done, ok := findBackupExportDone(results)
	if !ok {
		t.Fatal("esperava backupExportDoneMsg")
	}
	if done.err != nil {
		t.Fatalf("Export: %v", done.err)
	}
}

func TestBackupExportModel_Submit_WithoutPrivateKey_ConfirmNotDanger(t *testing.T) {
	m := newLoadedExportModel(t)
	m.hosts[0].included = true
	dest := filepath.Join(t.TempDir(), "backup.tar.gz")
	m.destPath.SetValue(dest)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava requestConfirmMsg")
	}
	confirmMsg, ok := cmd().(requestConfirmMsg)
	if !ok {
		t.Fatalf("esperava requestConfirmMsg, obteve %T", confirmMsg)
	}
	if confirmMsg.danger {
		t.Fatal("export sem chave privada não deveria ser danger=true")
	}
}

func TestBackupExportModel_Esc_PopsScreen(t *testing.T) {
	configSvc, keySvc, backupSvc := newTestServices(t)
	m := newBackupExportModel(configSvc, keySvc, backupSvc)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esperava popScreenMsg")
	}
	if _, ok := cmd().(popScreenMsg); !ok {
		t.Fatal("esperava popScreenMsg")
	}
}

func findBackupSelectionLoaded(msgs []tea.Msg) (backupSelectionLoadedMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(backupSelectionLoadedMsg); ok {
			return m, true
		}
	}
	return backupSelectionLoadedMsg{}, false
}

func findBackupExportDone(msgs []tea.Msg) (backupExportDoneMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(backupExportDoneMsg); ok {
			return m, true
		}
	}
	return backupExportDoneMsg{}, false
}
