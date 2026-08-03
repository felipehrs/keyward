package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// menuItem é uma entrada do menu principal. build é nil enquanto a tela de
// destino ainda não foi implementada (etapas futuras do plano) — nesse
// caso a seleção só mostra um erro não-fatal, sem travar a navegação.
type menuItem struct {
	label string
	build func() screen
}

type menuModel struct {
	items  []menuItem
	cursor int
}

func newMenuModel(configSvc core.ConfigService, keySvc core.KeyService, backupSvc core.BackupService) *menuModel {
	return &menuModel{
		items: []menuItem{
			{label: "Hosts", build: func() screen { return newHostsListModel(configSvc) }},
			{label: "Chaves", build: func() screen { return newKeysListModel(keySvc) }},
			{label: "Backup", build: func() screen { return newBackupMenuModel(configSvc, keySvc, backupSvc) }},
			{label: "Configurações", build: func() screen { return newSettingsModel(keySvc) }},
		},
	}
}

func (m *menuModel) Init() tea.Cmd { return nil }

func (m *menuModel) Update(msg tea.Msg) (screen, tea.Cmd) {
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
	case key.Matches(keyMsg, keyQuit):
		return m, tea.Quit
	}
	return m, nil
}

func (m *menuModel) View() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("keyward") + "\n\n")
	for i, item := range m.items {
		cursor := "  "
		label := item.label
		if i == m.cursor {
			cursor = "> "
			label = StyleSelected.Render(label)
		}
		b.WriteString(cursor + label + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("↑/↓ navegar · enter selecionar · q sair"))
	return b.String()
}
