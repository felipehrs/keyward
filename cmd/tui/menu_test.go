package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestMenu(t *testing.T) *menuModel {
	t.Helper()
	configSvc, keySvc, backupSvc := newTestServices(t)
	return newMenuModel(configSvc, keySvc, backupSvc)
}

func TestMenuModel_NavigatesCursor(t *testing.T) {
	m := newTestMenu(t)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*menuModel)
	if m.cursor != 1 {
		t.Fatalf("esperava cursor 1 após down, obteve %d", m.cursor)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(*menuModel)
	if m.cursor != 0 {
		t.Fatalf("esperava cursor 0 após up, obteve %d", m.cursor)
	}
}

func TestMenuModel_CursorDoesNotUnderOverflow(t *testing.T) {
	m := newTestMenu(t)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(*menuModel)
	if m.cursor != 0 {
		t.Fatalf("cursor não deveria ir abaixo de 0, obteve %d", m.cursor)
	}

	for i := 0; i < len(m.items)+2; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(*menuModel)
	}
	if m.cursor != len(m.items)-1 {
		t.Fatalf("cursor não deveria passar do último item, obteve %d", m.cursor)
	}
}

func TestMenuModel_SelectImplemented_PushesScreen(t *testing.T) {
	m := newTestMenu(t)
	// cursor 0 = Hosts, já implementado.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava um tea.Cmd")
	}
	msg := cmd()
	push, ok := msg.(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", msg)
	}
	if _, ok := push.screen.(*hostsListModel); !ok {
		t.Fatalf("esperava *hostsListModel, obteve %T", push.screen)
	}
}

func TestMenuModel_SelectSettings_PushesSettingsScreen(t *testing.T) {
	m := newTestMenu(t)
	for range 3 { // move até "Configurações" (índice 3)
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(*menuModel)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava pushScreenMsg")
	}
	push, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", push)
	}
	if _, ok := push.screen.(*settingsModel); !ok {
		t.Fatalf("esperava *settingsModel, obteve %T", push.screen)
	}
}

func TestMenuModel_SelectBackup_PushesBackupMenu(t *testing.T) {
	m := newTestMenu(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*menuModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*menuModel)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava pushScreenMsg")
	}
	push, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", push)
	}
	if _, ok := push.screen.(*backupMenuModel); !ok {
		t.Fatalf("esperava *backupMenuModel, obteve %T", push.screen)
	}
}

// TestBackupMenuModel_SelectImport_PushesImportScreen cobre o último item
// do plano de implementação — com esta etapa, todos os itens de todos os
// menus da TUI têm build (nenhum "ainda não implementado" resta).
func TestBackupMenuModel_SelectImport_PushesImportScreen(t *testing.T) {
	configSvc, keySvc, backupSvc := newTestServices(t)
	m := newBackupMenuModel(configSvc, keySvc, backupSvc)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(*backupMenuModel)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava pushScreenMsg")
	}
	push, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", push)
	}
	if _, ok := push.screen.(*backupImportModel); !ok {
		t.Fatalf("esperava *backupImportModel, obteve %T", push.screen)
	}
}

func TestMenuModel_Quit(t *testing.T) {
	m := newTestMenu(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("esperava tea.Quit")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("esperava tea.QuitMsg, obteve %T", msg)
	}
}
