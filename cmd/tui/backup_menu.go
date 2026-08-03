package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// backupMenuModel é o submenu Export/Import de backup. Diferente do menu
// principal (menu.go), esc volta pra tela anterior em vez de "q" sair do
// programa — só o menu principal encerra a TUI.
type backupMenuModel struct {
	items  []menuItem
	cursor int
}

func newBackupMenuModel(configSvc core.ConfigService, keySvc core.KeyService, backupSvc core.BackupService) *backupMenuModel {
	return &backupMenuModel{
		items: []menuItem{
			{label: "Export", build: func() screen { return newBackupExportModel(configSvc, keySvc, backupSvc) }},
			{label: "Import", build: func() screen { return newBackupImportModel(backupSvc) }},
		},
	}
}

func (m *backupMenuModel) Init() tea.Cmd { return nil }

func (m *backupMenuModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch {
	case key.Matches(keyMsg, keyUp):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(keyMsg, keyDown):
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case key.Matches(keyMsg, keyEnter):
		item := m.items[m.cursor]
		if item.build == nil {
			label := item.label
			return m, func() tea.Msg {
				return errMsg{err: fmt.Errorf("%s: ainda não implementado", label)}
			}
		}
		next := item.build()
		return m, func() tea.Msg { return pushScreenMsg{screen: next} }
	case key.Matches(keyMsg, keyBack):
		return m, func() tea.Msg { return popScreenMsg{} }
	}
	return m, nil
}

func (m *backupMenuModel) View() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("Backup") + "\n\n")
	for i, item := range m.items {
		cursor := "  "
		label := item.label
		if i == m.cursor {
			cursor = "> "
			label = StyleSelected.Render(label)
		}
		b.WriteString(cursor + label + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("↑/↓ navegar · enter selecionar · esc voltar"))
	return b.String()
}
