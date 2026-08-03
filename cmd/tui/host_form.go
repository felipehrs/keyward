package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// hostMutatedMsg é o resultado de AddHost/ReplaceHost/RemoveHost — os três
// só precisam sinalizar sucesso/erro; em sucesso a tela volta pra
// hosts_list, que recarrega automaticamente ao ficar ativa de novo (ver
// popScreenMsg em model.go).
type hostMutatedMsg struct{ err error }

// Índices dos campos do formulário de host, na ordem de navegação.
const (
	hostFieldPatterns = iota
	hostFieldHostName
	hostFieldUser
	hostFieldPort
	hostFieldIdentityFile
	hostFieldCount
)

// hostFormModel é o formulário de host, usado tanto para adicionar
// (AddHost, sem confirmação — não é destrutivo, o core recusa duplicata)
// quanto para editar um host existente (ReplaceHost, sempre com
// confirmação — sobrescreve o bloco inteiro). Só opera no arquivo em que o
// host já vive (oldHost.SourceFile) ao editar, ou no arquivo principal ao
// criar — não há seleção de arquivo de destino nesta etapa.
type hostFormModel struct {
	configSvc core.ConfigService

	editing bool
	oldHost core.Host // só relevante quando editing == true

	focus int

	patterns     textinput.Model
	hostName     textinput.Model
	user         textinput.Model
	port         textinput.Model
	identityFile textinput.Model

	validationErr error
}

func newHostFormModel(configSvc core.ConfigService) *hostFormModel {
	m := newBareHostFormModel(configSvc)
	m.patterns.Placeholder = "ex: bastion, bastion.example.com"
	m.setFocus(hostFieldPatterns)
	return m
}

func newHostEditFormModel(configSvc core.ConfigService, host core.Host) *hostFormModel {
	m := newBareHostFormModel(configSvc)
	m.editing = true
	m.oldHost = host
	m.patterns.SetValue(strings.Join(host.Patterns, ", "))
	m.hostName.SetValue(host.HostName)
	m.user.SetValue(host.User)
	m.port.SetValue(host.Port)
	m.identityFile.SetValue(strings.Join(host.IdentityFile, ", "))
	m.setFocus(hostFieldPatterns)
	return m
}

func newBareHostFormModel(configSvc core.ConfigService) *hostFormModel {
	m := &hostFormModel{
		configSvc:    configSvc,
		patterns:     textinput.New(),
		hostName:     textinput.New(),
		user:         textinput.New(),
		port:         textinput.New(),
		identityFile: textinput.New(),
	}
	m.hostName.Placeholder = "HostName (opcional)"
	m.user.Placeholder = "User (opcional)"
	m.port.Placeholder = "Port (opcional)"
	m.identityFile.Placeholder = "IdentityFile, separados por vírgula (opcional)"
	return m
}

func (m *hostFormModel) Init() tea.Cmd { return nil }

func (m *hostFormModel) inputs() [hostFieldCount]*textinput.Model {
	return [hostFieldCount]*textinput.Model{
		hostFieldPatterns:     &m.patterns,
		hostFieldHostName:     &m.hostName,
		hostFieldUser:         &m.user,
		hostFieldPort:         &m.port,
		hostFieldIdentityFile: &m.identityFile,
	}
}

func (m *hostFormModel) setFocus(field int) tea.Cmd {
	inputs := m.inputs()
	for i, in := range inputs {
		if i != field {
			in.Blur()
		}
	}
	m.focus = field
	return inputs[field].Focus()
}

func (m *hostFormModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case hostMutatedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg(typed) }
		}
		return m, func() tea.Msg { return popScreenMsg{} }

	case tea.KeyMsg:
		switch typed.String() {
		case "esc":
			return m, func() tea.Msg { return popScreenMsg{} }
		case "tab", "down":
			return m, m.setFocus((m.focus + 1) % hostFieldCount)
		case "shift+tab", "up":
			return m, m.setFocus((m.focus - 1 + hostFieldCount) % hostFieldCount)
		case "enter":
			return m, m.submit()
		}
	}

	in := m.inputs()[m.focus]
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	return m, cmd
}

func splitCSV(raw string) []string {
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (m *hostFormModel) submit() tea.Cmd {
	m.validationErr = nil

	patterns := splitCSV(m.patterns.Value())
	if len(patterns) == 0 {
		m.validationErr = fmt.Errorf("informe ao menos um pattern")
		return nil
	}

	spec := core.HostSpec{
		Patterns:     patterns,
		HostName:     m.hostName.Value(),
		User:         m.user.Value(),
		Port:         m.port.Value(),
		IdentityFile: splitCSV(m.identityFile.Value()),
	}

	if !m.editing {
		// AddHost não é destrutivo — o core recusa se já existir um bloco
		// com os mesmos Patterns, nunca sobrescreve — por isso não passa
		// pelo modal de confirmação (diferente de editar/remover).
		svc := m.configSvc
		return startAsync(func() tea.Msg {
			return hostMutatedMsg{err: svc.AddHost("", spec)}
		})
	}

	svc := m.configSvc
	oldPatterns := m.oldHost.Patterns
	sourceFile := m.oldHost.SourceFile
	onConfirm := startAsync(func() tea.Msg {
		return hostMutatedMsg{err: svc.ReplaceHost(sourceFile, oldPatterns, spec)}
	})
	return func() tea.Msg {
		return requestConfirmMsg{
			title:     "Substituir host existente?",
			body:      "As diretivas atuais de " + strings.Join(oldPatterns, ", ") + " em " + sourceFile + " serão descartadas e reescritas.",
			danger:    true,
			onConfirm: onConfirm,
		}
	}
}

func (m *hostFormModel) View() string {
	var b strings.Builder
	title := "Novo host"
	if m.editing {
		title = "Editar host " + strings.Join(m.oldHost.Patterns, ", ")
	}
	b.WriteString(StyleTitle.Render(title) + "\n\n")

	b.WriteString("Patterns:     " + m.patterns.View() + "\n")
	b.WriteString("HostName:     " + m.hostName.View() + "\n")
	b.WriteString("User:         " + m.user.View() + "\n")
	b.WriteString("Port:         " + m.port.View() + "\n")
	b.WriteString("IdentityFile: " + m.identityFile.View() + "\n")

	if m.validationErr != nil {
		b.WriteString("\n" + StyleDanger.Render("erro: "+m.validationErr.Error()) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("tab/shift+tab navegar · enter salvar (com confirmação) · esc cancelar"))
	return b.String()
}
