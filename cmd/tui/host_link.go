package main

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// errNoHostsAvailable/errNoAgentKeysAvailable cobrem enter numa lista vazia
// (nenhum host cadastrado, ou nenhuma chave de agente elegível/registrada
// ainda) — mensagem não-fatal, o usuário só volta e resolve o pré-requisito.
var (
	errNoHostsAvailable     = errors.New("nenhum host disponível — cadastre um host antes de vincular")
	errNoAgentKeysAvailable = errors.New("nenhuma chave de agente registrada disponível — anote uma chave de agente antes de vincular ('o' na tela de Chaves)")
)

// hostKey aplica a convenção documentada em core/metadata.go (HostMetadata.
// HostKey é opaco para core — a convenção é definida e aplicada pelo
// chamador): junta Patterns com um separador que nunca aparece em um
// pattern válido do ssh_config.
func hostKey(patterns []string) string {
	return strings.Join(patterns, "\x00")
}

// hostLinkLoadedMsg é o resultado combinado de ConfigService.ListHosts +
// KeyService.ListHostLinks + KeyService.ListKeys — carregados numa única
// operação assíncrona (ver reload) porque o roteamento de model.go só
// aceita a resposta da última operação disparada (reqID); duas chamadas
// concorrentes fariam uma delas ser descartada por engano.
type hostLinkLoadedMsg struct {
	hosts []core.Host
	links []core.HostMetadata
	keys  []core.Key
	err   error
}

// hostUnlinkedMsg é o resultado de KeyService.UnlinkHostKey.
type hostUnlinkedMsg struct{ err error }

// hostLinkItem adapta um HostMetadata (cruzado com os hosts atuais e as
// chaves de agente conhecidas) para list.DefaultItem, sinalizando vínculo
// órfão quando HostKey não corresponde a nenhum Host.Patterns atual (spec
// ssh-agent-support, "vincular host" — detecção de órfão é responsabilidade
// desta camada, não do core: KeyService nunca importa ConfigService).
type hostLinkItem struct {
	link     core.HostMetadata
	host     core.Host // zero-value quando orphan == true
	orphan   bool
	keyLabel string
}

func (i hostLinkItem) Title() string {
	if i.orphan {
		return StyleDanger.Render("[ÓRFÃO] ") + strings.ReplaceAll(i.link.HostKey, "\x00", ", ")
	}
	return strings.Join(i.host.Patterns, ", ")
}

func (i hostLinkItem) Description() string {
	return StyleMuted.Render("chave: ") + i.keyLabel
}

func (i hostLinkItem) FilterValue() string { return i.Title() + " " + i.keyLabel }

// buildHostLinkItems cruza links com hosts (por hostKey) e com keys (por
// fingerprint) pra decidir órfão e rótulo de exibição.
func buildHostLinkItems(hosts []core.Host, links []core.HostMetadata, keys []core.Key) []list.Item {
	hostByKey := make(map[string]core.Host, len(hosts))
	for _, h := range hosts {
		hostByKey[hostKey(h.Patterns)] = h
	}
	keyByFingerprint := make(map[string]core.Key, len(keys))
	for _, k := range keys {
		if k.Metadata.Fingerprint != "" {
			keyByFingerprint[k.Metadata.Fingerprint] = k
		}
	}

	items := make([]list.Item, 0, len(links))
	for _, link := range links {
		host, matched := hostByKey[link.HostKey]
		keyLabel := link.AgentKeyFingerprint
		if k, ok := keyByFingerprint[link.AgentKeyFingerprint]; ok && k.Metadata.Label != "" {
			keyLabel = k.Metadata.Label
		}
		items = append(items, hostLinkItem{link: link, host: host, orphan: !matched, keyLabel: keyLabel})
	}
	return items
}

// hostLinksModel lista os vínculos host/chave-de-agente persistidos
// (KeyService.ListHostLinks), cruzando com ConfigService.ListHosts pra
// sinalizar órfãos. Recarrega sempre que fica ativa e sob pedido explícito
// (tecla r) — mesmo padrão de hosts_list.go/keys_list.go.
type hostLinksModel struct {
	keySvc    core.KeyService
	configSvc core.ConfigService
	list      list.Model
}

func newHostLinksModel(keySvc core.KeyService, configSvc core.ConfigService) *hostLinksModel {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.Title = "Vínculos host / chave de agente"
	l.DisableQuitKeybindings()
	return &hostLinksModel{keySvc: keySvc, configSvc: configSvc, list: l}
}

func (m *hostLinksModel) Init() tea.Cmd {
	return m.reload()
}

func (m *hostLinksModel) reload() tea.Cmd {
	keySvc := m.keySvc
	configSvc := m.configSvc
	return startAsync(func() tea.Msg {
		hosts, err := configSvc.ListHosts()
		if err != nil {
			return hostLinkLoadedMsg{err: err}
		}
		links, err := keySvc.ListHostLinks()
		if err != nil {
			return hostLinkLoadedMsg{err: err}
		}
		keys, err := keySvc.ListKeys()
		if err != nil {
			return hostLinkLoadedMsg{err: err}
		}
		return hostLinkLoadedMsg{hosts: hosts, links: links, keys: keys}
	})
}

// requestUnlink abre o modal de confirmação para UnlinkHostKey — nunca
// remove diretamente (vínculo órfão nunca é limpo automaticamente, mesmo
// removível manualmente — spec ssh-agent-support).
func (m *hostLinksModel) requestUnlink(item hostLinkItem) tea.Cmd {
	svc := m.keySvc
	hk := item.link.HostKey
	fp := item.link.AgentKeyFingerprint
	onConfirm := startAsync(func() tea.Msg {
		return hostUnlinkedMsg{err: svc.UnlinkHostKey(hk, fp)}
	})
	title := "Remover vínculo?"
	body := "O vínculo entre " + item.Title() + " e a chave " + item.keyLabel + " será removido."
	return func() tea.Msg {
		return requestConfirmMsg{title: title, body: body, danger: false, onConfirm: onConfirm}
	}
}

func (m *hostLinksModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case hostUnlinkedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg(typed) }
		}
		return m, m.reload()

	case hostLinkLoadedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		return m, m.list.SetItems(buildHostLinkItems(typed.hosts, typed.links, typed.keys))

	case tea.WindowSizeMsg:
		height := max(typed.Height-2, 0)
		m.list.SetSize(typed.Width, height)
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() != list.Filtering {
			switch {
			case key.Matches(typed, keyBack):
				return m, func() tea.Msg { return popScreenMsg{} }
			case key.Matches(typed, keyReload):
				return m, m.reload()
			case key.Matches(typed, keyNew):
				form := newHostLinkFormModel(m.keySvc, m.configSvc)
				return m, func() tea.Msg { return pushScreenMsg{screen: form} }
			case key.Matches(typed, keyDelete):
				item, ok := m.list.SelectedItem().(hostLinkItem)
				if !ok {
					return m, nil
				}
				return m, m.requestUnlink(item)
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *hostLinksModel) View() string {
	return m.list.View() + "\n" + StyleHelp.Render("↑/↓ navegar · n novo vínculo · d remover · r recarregar · esc voltar")
}

// --- Criação de vínculo (LinkHostKey) ---

// hostLinkedMsg é o resultado de KeyService.LinkHostKey.
type hostLinkedMsg struct{ err error }

// hostLinkFormLoadedMsg carrega, numa única operação assíncrona, os hosts
// disponíveis (ConfigService.ListHosts) e as chaves de agente já com
// registro de metadata (KeyService.ListKeys, filtrado) — únicas elegíveis
// pra vínculo, já que um vínculo sem Label/registro não teria como ser
// identificado depois (mesma razão de RegisterAgentKey ser pré-requisito).
type hostLinkFormLoadedMsg struct {
	hosts []core.Host
	keys  []core.Key
	err   error
}

// hostLinkFormModel é um assistente de dois passos: escolher o host (dentre
// ConfigService.ListHosts) e, em seguida, a chave de agente (dentre as já
// registradas) — reaproveita hostItem/keyItem (hosts_list.go/keys_list.go)
// pra exibição consistente com o resto da TUI, em vez de reimplementar a
// formatação.
type hostLinkFormModel struct {
	keySvc    core.KeyService
	configSvc core.ConfigService

	step int // 0 = escolher host, 1 = escolher chave de agente

	hosts        list.Model
	keys         list.Model
	selectedHost core.Host

	width, height int
}

func newHostLinkFormModel(keySvc core.KeyService, configSvc core.ConfigService) *hostLinkFormModel {
	hostDelegate := list.NewDefaultDelegate()
	hostsList := list.New(nil, hostDelegate, 0, 0)
	hostsList.Title = "1/2 — Escolha o host"
	hostsList.DisableQuitKeybindings()

	keyDelegate := list.NewDefaultDelegate()
	keysList := list.New(nil, keyDelegate, 0, 0)
	keysList.Title = "2/2 — Escolha a chave de agente"
	keysList.DisableQuitKeybindings()

	return &hostLinkFormModel{keySvc: keySvc, configSvc: configSvc, hosts: hostsList, keys: keysList}
}

func (m *hostLinkFormModel) Init() tea.Cmd {
	keySvc := m.keySvc
	configSvc := m.configSvc
	return startAsync(func() tea.Msg {
		hosts, err := configSvc.ListHosts()
		if err != nil {
			return hostLinkFormLoadedMsg{err: err}
		}
		keys, err := keySvc.ListKeys()
		if err != nil {
			return hostLinkFormLoadedMsg{err: err}
		}
		return hostLinkFormLoadedMsg{hosts: hosts, keys: keys}
	})
}

// eligibleAgentKeys filtra chaves de agente já com registro de metadata
// (Status OK ou AgentOffline — ambos têm Fingerprint/Label persistidos;
// KeyStatusUnregistered é excluída porque ainda não tem rótulo pra
// identificar o vínculo depois).
func eligibleAgentKeys(keys []core.Key) []core.Key {
	var out []core.Key
	for _, k := range keys {
		if k.Source != core.KeySourceAgent {
			continue
		}
		if k.Status == core.KeyStatusOK || k.Status == core.KeyStatusAgentOffline {
			out = append(out, k)
		}
	}
	return out
}

func (m *hostLinkFormModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case hostLinkFormLoadedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		hostItems := make([]list.Item, len(typed.hosts))
		for i, h := range typed.hosts {
			hostItems[i] = hostItem{host: h}
		}
		eligible := eligibleAgentKeys(typed.keys)
		keyItems := make([]list.Item, len(eligible))
		for i, k := range eligible {
			keyItems[i] = keyItem{key: k}
		}
		var cmds []tea.Cmd
		if c := m.hosts.SetItems(hostItems); c != nil {
			cmds = append(cmds, c)
		}
		if c := m.keys.SetItems(keyItems); c != nil {
			cmds = append(cmds, c)
		}
		return m, tea.Batch(cmds...)

	case hostLinkedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg(typed) }
		}
		return m, func() tea.Msg { return popScreenMsg{} }

	case tea.WindowSizeMsg:
		m.width, m.height = typed.Width, typed.Height
		height := max(typed.Height-2, 0)
		m.hosts.SetSize(typed.Width, height)
		m.keys.SetSize(typed.Width, height)
		return m, nil

	case tea.KeyMsg:
		activeList := m.hosts
		if m.step == 1 {
			activeList = m.keys
		}
		if activeList.FilterState() != list.Filtering {
			switch {
			case key.Matches(typed, keyBack):
				if m.step == 1 {
					m.step = 0
					return m, nil
				}
				return m, func() tea.Msg { return popScreenMsg{} }
			case key.Matches(typed, keyEnter):
				if m.step == 0 {
					item, ok := m.hosts.SelectedItem().(hostItem)
					if !ok {
						return m, func() tea.Msg {
							return errMsg{err: errNoHostsAvailable}
						}
					}
					m.selectedHost = item.host
					m.step = 1
					return m, nil
				}
				item, ok := m.keys.SelectedItem().(keyItem)
				if !ok {
					return m, func() tea.Msg {
						return errMsg{err: errNoAgentKeysAvailable}
					}
				}
				return m, m.submit(item.key)
			}
		}
	}

	var cmd tea.Cmd
	if m.step == 0 {
		m.hosts, cmd = m.hosts.Update(msg)
	} else {
		m.keys, cmd = m.keys.Update(msg)
	}
	return m, cmd
}

func (m *hostLinkFormModel) submit(k core.Key) tea.Cmd {
	svc := m.keySvc
	hk := hostKey(m.selectedHost.Patterns)
	fp := k.Metadata.Fingerprint
	return startAsync(func() tea.Msg {
		return hostLinkedMsg{err: svc.LinkHostKey(hk, fp, "")}
	})
}

func (m *hostLinkFormModel) View() string {
	if m.step == 0 {
		return m.hosts.View() + "\n" + StyleHelp.Render("↑/↓ navegar · enter escolher host · esc cancelar")
	}
	return m.keys.View() + "\n" + StyleHelp.Render("↑/↓ navegar · enter vincular · esc voltar")
}
