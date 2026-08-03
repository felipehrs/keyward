package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

func TestSettingsModel_Init_LoadsDefaults(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newSettingsModel(keySvc)

	loaded, ok := findSettingsLoaded(resolveMsgs(startAsyncCmdFor(t, m.Init())))
	if !ok {
		t.Fatal("esperava settingsLoadedMsg")
	}
	if loaded.err != nil {
		t.Fatalf("Settings: %v", loaded.err)
	}
	if loaded.settings.AlertThresholdDays != 30 {
		t.Fatalf("esperava default AlertThresholdDays=30, obteve %d", loaded.settings.AlertThresholdDays)
	}

	m.Update(loaded)
	if !m.loaded || m.alertThresholdDays.Value() != "30" {
		t.Fatalf("esperava campo populado com '30', obteve %q", m.alertThresholdDays.Value())
	}
	if m.defaultAlgorithm != core.AlgorithmEd25519 {
		t.Fatalf("esperava default ed25519, obteve %q", m.defaultAlgorithm)
	}
}

func TestSettingsModel_Submit_PersistsChanges(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newSettingsModel(keySvc)
	loaded, _ := findSettingsLoaded(resolveMsgs(startAsyncCmdFor(t, m.Init())))
	m.Update(loaded)

	m.alertThresholdDays.SetValue("45")
	m.defaultAlgorithm = core.AlgorithmRSA

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava tea.Cmd de submissão")
	}
	results := resolveMsgs(startAsyncCmdFor(t, cmd))
	saved, ok := findSettingsSaved(results)
	if !ok {
		t.Fatal("esperava settingsSavedMsg")
	}
	if saved.err != nil {
		t.Fatalf("UpdateSettings: %v", saved.err)
	}

	current, err := keySvc.Settings()
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if current.AlertThresholdDays != 45 || current.DefaultAlgorithm != core.AlgorithmRSA {
		t.Fatalf("esperava settings persistidas, obteve %+v", current)
	}
}

func TestSettingsModel_InvalidThreshold_BlocksSubmit(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newSettingsModel(keySvc)
	loaded, _ := findSettingsLoaded(resolveMsgs(startAsyncCmdFor(t, m.Init())))
	m.Update(loaded)

	m.alertThresholdDays.SetValue("não é um número")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("não deveria submeter com AlertThresholdDays inválido")
	}
	if m.validationErr == nil {
		t.Fatal("esperava validationErr definido")
	}
}

func TestSettingsModel_Esc_PopsScreen(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	m := newSettingsModel(keySvc)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esperava popScreenMsg")
	}
	if _, ok := cmd().(popScreenMsg); !ok {
		t.Fatal("esperava popScreenMsg")
	}
}

func findSettingsLoaded(msgs []tea.Msg) (settingsLoadedMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(settingsLoadedMsg); ok {
			return m, true
		}
	}
	return settingsLoadedMsg{}, false
}

func findSettingsSaved(msgs []tea.Msg) (settingsSavedMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(settingsSavedMsg); ok {
			return m, true
		}
	}
	return settingsSavedMsg{}, false
}
