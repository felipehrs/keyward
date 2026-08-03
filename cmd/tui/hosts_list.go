package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// hostsLoadedMsg é o resultado de ConfigService.ListHosts, entregue pelo
// root via asyncResultMsg (ver async.go/model.go).
type hostsLoadedMsg struct {
	hosts []core.Host
	err   error
}

// hostItem adapta core.Host para list.DefaultItem.
type hostItem struct{ host core.Host }

func (i hostItem) Title() string { return strings.Join(i.host.Patterns, ", ") }

func (i hostItem) Description() string {
	var fields []string
	if i.host.HostName != "" {
		fields = append(fields, "HostName "+i.host.HostName)
	}
	if i.host.User != "" {
		fields = append(fields, "User "+i.host.User)
	}
	if i.host.Port != "" {
		fields = append(fields, "Port "+i.host.Port)
	}
	desc := "-"
	if len(fields) > 0 {
		desc = strings.Join(fields, " · ")
	}
	return desc + "  " + StyleMuted.Render(i.host.SourceFile)
}

func (i hostItem) FilterValue() string { return i.Title() }

// hostsListModel é a tela de listagem somente-leitura de hosts
// (ConfigService.ListHosts). Recarrega sempre que fica ativa (Init) e sob
// pedido explícito (tecla r) — não confia em estado em memória "antigo",
// já que metadata.json/config não têm lock entre processos.
type hostsListModel struct {
	configSvc core.ConfigService
	list      list.Model
}

func newHostsListModel(configSvc core.ConfigService) *hostsListModel {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.Title = "Hosts"
	l.DisableQuitKeybindings() // "q" não deve sair da TUI fora do menu principal
	return &hostsListModel{configSvc: configSvc, list: l}
}

func (m *hostsListModel) Init() tea.Cmd {
	return m.reload()
}

func (m *hostsListModel) reload() tea.Cmd {
	svc := m.configSvc
	return startAsync(func() tea.Msg {
		hosts, err := svc.ListHosts()
		return hostsLoadedMsg{hosts: hosts, err: err}
	})
}

// requestRemove abre o modal de confirmação para RemoveHost — nunca
// remove diretamente, já que é uma ação destrutiva (seção 6 da spec).
func (m *hostsListModel) requestRemove(host core.Host) tea.Cmd {
	svc := m.configSvc
	patterns := host.Patterns
	sourceFile := host.SourceFile
	onConfirm := startAsync(func() tea.Msg {
		return hostMutatedMsg{err: svc.RemoveHost(sourceFile, patterns)}
	})
	return func() tea.Msg {
		return requestConfirmMsg{
			title:     "Remover host?",
			body:      "O bloco Host " + strings.Join(patterns, ", ") + " em " + sourceFile + " será removido permanentemente.",
			danger:    true,
			onConfirm: onConfirm,
		}
	}
}

func (m *hostsListModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case hostMutatedMsg: // resultado de RemoveHost disparado por requestRemove
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg(typed) }
		}
		return m, m.reload()

	case hostsLoadedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		items := make([]list.Item, len(typed.hosts))
		for i, h := range typed.hosts {
			items[i] = hostItem{host: h}
		}
		return m, m.list.SetItems(items)

	case tea.WindowSizeMsg:
		height := max(typed.Height-2, 0) // reserva espaço pro rodapé de atalhos próprio
		m.list.SetSize(typed.Width, height)
		return m, nil

	case tea.KeyMsg:
		// Enquanto o usuário digita um filtro (list.Filtering), toda tecla
		// deve ir pro campo de filtro — inclusive letras que colidem com
		// nossos atalhos (n, d, r) — nunca interceptada aqui.
		if m.list.FilterState() != list.Filtering {
			switch {
			case key.Matches(typed, keyBack):
				return m, func() tea.Msg { return popScreenMsg{} }
			case key.Matches(typed, keyReload):
				return m, m.reload()
			case key.Matches(typed, keyNew):
				form := newHostFormModel(m.configSvc)
				return m, func() tea.Msg { return pushScreenMsg{screen: form} }
			case key.Matches(typed, keyEnter):
				item, ok := m.list.SelectedItem().(hostItem)
				if !ok {
					return m, nil
				}
				form := newHostEditFormModel(m.configSvc, item.host)
				return m, func() tea.Msg { return pushScreenMsg{screen: form} }
			case key.Matches(typed, keyDelete):
				item, ok := m.list.SelectedItem().(hostItem)
				if !ok {
					return m, nil
				}
				return m, m.requestRemove(item.host)
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *hostsListModel) View() string {
	return m.list.View() + "\n" + StyleHelp.Render("↑/↓ navegar · n novo · enter editar · d remover · r recarregar · esc voltar")
}
