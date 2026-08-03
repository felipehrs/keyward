package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// settingsLoadedMsg é o resultado de KeyService.Settings.
type settingsLoadedMsg struct {
	settings core.AppSettings
	err      error
}

// settingsSavedMsg é o resultado de KeyService.UpdateSettings.
type settingsSavedMsg struct {
	settings core.AppSettings
	err      error
}

const (
	settingsFieldThreshold = iota
	settingsFieldAlgorithm
	settingsFieldCount
)

// settingsModel edita AppSettings (Settings/UpdateSettings) — acessível do
// menu principal e de dentro da lista de chaves, já que AlertThresholdDays
// e DefaultAlgorithm afetam diretamente o que keys_list.go destaca e o
// algoritmo padrão de key_form.go.
type settingsModel struct {
	keySvc core.KeyService

	loaded bool
	focus  int

	alertThresholdDays textinput.Model
	defaultAlgorithm   core.Algorithm

	validationErr error
}

func newSettingsModel(keySvc core.KeyService) *settingsModel {
	m := &settingsModel{keySvc: keySvc, alertThresholdDays: textinput.New()}
	m.alertThresholdDays.Focus()
	return m
}

func (m *settingsModel) Init() tea.Cmd { return m.reload() }

func (m *settingsModel) reload() tea.Cmd {
	svc := m.keySvc
	return startAsync(func() tea.Msg {
		s, err := svc.Settings()
		return settingsLoadedMsg{settings: s, err: err}
	})
}

func (m *settingsModel) setFocus(field int) tea.Cmd {
	m.focus = field
	if field == settingsFieldThreshold {
		return m.alertThresholdDays.Focus()
	}
	m.alertThresholdDays.Blur()
	return nil
}

func (m *settingsModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case settingsLoadedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		m.loaded = true
		m.alertThresholdDays.SetValue(strconv.Itoa(typed.settings.AlertThresholdDays))
		m.defaultAlgorithm = typed.settings.DefaultAlgorithm
		if m.defaultAlgorithm == "" {
			m.defaultAlgorithm = core.AlgorithmEd25519
		}
		return m, nil

	case settingsSavedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		return m, func() tea.Msg { return popScreenMsg{} }

	case tea.KeyMsg:
		switch typed.String() {
		case "esc":
			return m, func() tea.Msg { return popScreenMsg{} }
		case "tab", "down", "shift+tab", "up":
			return m, m.setFocus(1 - m.focus) // só 2 campos — alterna
		case "left", "right":
			if m.focus == settingsFieldAlgorithm {
				if m.defaultAlgorithm == core.AlgorithmEd25519 {
					m.defaultAlgorithm = core.AlgorithmRSA
				} else {
					m.defaultAlgorithm = core.AlgorithmEd25519
				}
				return m, nil
			}
		case "enter":
			return m, m.submit()
		}
	}

	if m.focus == settingsFieldThreshold {
		var cmd tea.Cmd
		m.alertThresholdDays, cmd = m.alertThresholdDays.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *settingsModel) submit() tea.Cmd {
	m.validationErr = nil
	raw := m.alertThresholdDays.Value()
	days, err := strconv.Atoi(raw)
	if err != nil || days < 0 {
		m.validationErr = fmt.Errorf("AlertThresholdDays inválido: %q (deve ser um inteiro >= 0)", raw)
		return nil
	}

	settings := core.AppSettings{AlertThresholdDays: days, DefaultAlgorithm: m.defaultAlgorithm}
	svc := m.keySvc
	return startAsync(func() tea.Msg {
		return settingsSavedMsg{settings: settings, err: svc.UpdateSettings(settings)}
	})
}

func (m *settingsModel) View() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("Configurações") + "\n\n")

	if !m.loaded {
		b.WriteString("carregando...")
		return b.String()
	}

	b.WriteString("AlertThresholdDays: " + m.alertThresholdDays.View() + "\n")

	algo := string(m.defaultAlgorithm)
	if m.focus == settingsFieldAlgorithm {
		algo = StyleSelected.Render(algo)
	}
	b.WriteString("DefaultAlgorithm (←/→): " + algo + "\n")

	if m.validationErr != nil {
		b.WriteString("\n" + StyleDanger.Render("erro: "+m.validationErr.Error()) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("tab alternar campo · enter salvar · esc voltar"))
	return b.String()
}
