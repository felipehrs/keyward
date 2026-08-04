package main

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// keysLoadedMsg é o resultado de KeyService.ListKeys, entregue pelo root
// via asyncResultMsg (ver async.go/model.go). ListKeys já vem ordenada por
// proximidade de expiração e com IsExpired/IsExpiringSoon calculados a
// partir de AppSettings.AlertThresholdDays — a tela só precisa exibir.
type keysLoadedMsg struct {
	keys []core.Key
	err  error
}

// keyItem adapta core.Key para list.DefaultItem, destacando visualmente o
// status de expiração (nunca só por cor — o texto também muda, já que nem
// todo terminal resolve StyleWarn/StyleDanger).
type keyItem struct{ key core.Key }

func (i keyItem) Title() string {
	label := i.key.Metadata.Label
	if label == "" {
		label = "-"
	}
	title := label

	if i.key.Source == core.KeySourceAgent {
		badge := i.key.AgentName
		if badge == "" {
			badge = "Agente SSH"
		}
		title += "  " + StyleAgentBadge.Render("["+badge+"]")
	}

	switch {
	case i.key.IsExpired:
		title += "  " + StyleDanger.Render("EXPIRADA")
	case i.key.IsExpiringSoon:
		title += "  " + StyleWarn.Render("EXPIRANDO")
	}

	switch i.key.Status {
	case core.KeyStatusUnregistered:
		if i.key.Source == core.KeySourceAgent {
			title += "  " + StyleMuted.Render("(identidade não anotada)")
		} else {
			title += "  " + StyleMuted.Render("(sem registro de metadata)")
		}
	case core.KeyStatusMissingFile:
		title += "  " + StyleMuted.Render("(arquivo ausente em disco)")
	case core.KeyStatusAgentOffline:
		// Estado transitório (agente fechado/bloqueado no momento) —
		// destacado, mas distinto de KeyStatusMissingFile (StyleMuted) e de
		// expiração (StyleDanger).
		title += "  " + StyleAgentOffline.Render("AGENTE OFFLINE")
	}
	return title
}

func (i keyItem) Description() string {
	fp := i.key.Metadata.Fingerprint
	if fp == "" {
		fp = "-"
	}
	algo := string(i.key.Algorithm)
	if algo == "" {
		algo = "-"
	}

	if i.key.Source == core.KeySourceAgent {
		comment := i.key.Comment
		if comment == "" {
			comment = "-"
		}
		return StyleMuted.Render(comment) + "  " + StyleMuted.Render(fp) + "  " + algo
	}
	return StyleMuted.Render(keyFileName(i.key)) + "  " + StyleMuted.Render(fp) + "  " + algo
}

// keyFileName retorna o nome do arquivo da chave (ex. "id_ed25519"), sem o
// diretório — útil sobretudo pra KeyStatusUnregistered, cujo
// Metadata.Fingerprint fica vazio (o core só o preenche a partir de um
// registro de metadata existente), então o nome do arquivo é o único
// identificador legível disponível nesse caso.
func keyFileName(k core.Key) string {
	path := k.PrivateKeyPath
	if path == "" {
		path = k.PublicKeyPath
	}
	if path == "" {
		return "-"
	}
	return filepath.Base(path)
}

func (i keyItem) FilterValue() string { return i.Title() + " " + i.Description() }

// sectionHeaderItem é um list.Item não-acionável, usado só como divisor
// visual entre o grupo de chaves de arquivo (ordenado por proximidade de
// expiração) e o grupo de chaves de agente (fora dessa ordenação — spec
// ssh-agent-support seção 5). FilterValue vazio faz o item desaparecer sob
// filtro, em vez de poluir os resultados de busca; handlers de tecla que já
// fazem type assertion pra keyItem simplesmente ignoram-no ao ser
// selecionado (ver keys_list.go Update).
type sectionHeaderItem struct{ label string }

func (i sectionHeaderItem) Title() string       { return StyleTitle.Render(i.label) }
func (i sectionHeaderItem) Description() string { return "" }
func (i sectionHeaderItem) FilterValue() string { return "" }

// buildKeyItems agrupa keys em dois blocos visuais — chaves de arquivo
// (preservando a ordem de ListKeys, já por proximidade de expiração) e
// chaves de agente (preservando a ordem de ListKeys, mas nunca misturadas
// com a ordenação por expiração das chaves de arquivo) — separados por um
// sectionHeaderItem quando há pelo menos uma chave de agente.
func buildKeyItems(keys []core.Key) []list.Item {
	var fileKeys, agentKeys []core.Key
	for _, k := range keys {
		if k.Source == core.KeySourceAgent {
			agentKeys = append(agentKeys, k)
		} else {
			fileKeys = append(fileKeys, k)
		}
	}

	items := make([]list.Item, 0, len(keys)+1)
	for _, k := range fileKeys {
		items = append(items, keyItem{key: k})
	}
	if len(agentKeys) > 0 {
		items = append(items, sectionHeaderItem{label: "— Chaves de agente SSH —"})
		for _, k := range agentKeys {
			items = append(items, keyItem{key: k})
		}
	}
	return items
}

// agentDetectedMsg é o resultado de KeyService.DetectAgent, usado só para o
// aviso discreto "nenhum agente SSH detectado" — nunca bloqueia a tela
// (por isso disparado fora do mecanismo startAsync/spinner de model.go: não
// compete pelo reqID "última operação vence" com o carregamento de
// keysLoadedMsg, que é o que realmente importa pra tela).
type agentDetectedMsg struct{ info core.AgentInfo }

// keysListModel é a tela de listagem de chaves (KeyService.ListKeys).
// Recarrega sempre que fica ativa (Init) e sob pedido explícito (tecla r).
type keysListModel struct {
	keySvc core.KeyService
	list   list.Model

	agentChecked bool // já recebeu uma resposta de DetectAgent nesta sessão da tela
	agentInfo    core.AgentInfo
}

func newKeysListModel(keySvc core.KeyService) *keysListModel {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.Title = "Chaves"
	l.DisableQuitKeybindings()
	return &keysListModel{keySvc: keySvc, list: l}
}

func (m *keysListModel) Init() tea.Cmd {
	return tea.Batch(m.reload(), m.detectAgent())
}

func (m *keysListModel) reload() tea.Cmd {
	svc := m.keySvc
	return startAsync(func() tea.Msg {
		keys, err := svc.ListKeys()
		return keysLoadedMsg{keys: keys, err: err}
	})
}

// detectAgent sonda DetectAgent fora do mecanismo startAsync (ver
// agentDetectedMsg) — o aviso resultante é puramente informativo e não deve
// nunca bloquear a tela nem competir pelo reqID de "última operação vence"
// com o carregamento da lista de chaves.
func (m *keysListModel) detectAgent() tea.Cmd {
	svc := m.keySvc
	return func() tea.Msg {
		info, _ := svc.DetectAgent() // DetectAgent nunca retorna err != nil hoje; Detected==false já cobre "inacessível"
		return agentDetectedMsg{info: info}
	}
}

func (m *keysListModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case keysLoadedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		return m, m.list.SetItems(buildKeyItems(typed.keys))

	case agentDetectedMsg:
		m.agentChecked = true
		m.agentInfo = typed.info
		return m, nil

	case tea.WindowSizeMsg:
		height := max(typed.Height-2, 0) // reserva espaço pro rodapé de atalhos próprio
		m.list.SetSize(typed.Width, height)
		return m, nil

	case tea.KeyMsg:
		// Enquanto o usuário digita um filtro (list.Filtering), toda tecla
		// vai pro campo de filtro — nunca interceptada aqui.
		if m.list.FilterState() != list.Filtering {
			switch {
			case key.Matches(typed, keyBack):
				return m, func() tea.Msg { return popScreenMsg{} }
			case key.Matches(typed, keyReload):
				return m, m.reload()
			case key.Matches(typed, keyGenerate):
				form := newKeyFormModel(m.keySvc)
				return m, func() tea.Msg { return pushScreenMsg{screen: form} }
			case key.Matches(typed, keyEnter):
				item, ok := m.list.SelectedItem().(keyItem)
				if !ok {
					return m, nil
				}
				if item.key.Status == core.KeyStatusUnregistered {
					return m, func() tea.Msg {
						return errMsg{err: fmt.Errorf("chave sem registro de metadata — pressione 'o' pra registrar antes de ver detalhes")}
					}
				}
				detail := newKeyDetailModel(m.keySvc, item.key.Metadata.Fingerprint)
				return m, func() tea.Msg { return pushScreenMsg{screen: detail} }
			case key.Matches(typed, keyRegisterOrphan):
				item, ok := m.list.SelectedItem().(keyItem)
				if !ok || item.key.Status != core.KeyStatusUnregistered {
					return m, func() tea.Msg {
						return errMsg{err: fmt.Errorf("selecione uma chave sem registro de metadata pra registrar")}
					}
				}
				// Mesmo atalho pras duas origens — "registrar" é a mesma
				// ação conceitual (dar um primeiro registro de metadata a
				// algo já detectado), só o formulário e o método do core
				// mudam.
				if item.key.Source == core.KeySourceAgent {
					form := newAgentKeyRegisterFormModel(m.keySvc, item.key.Metadata.Fingerprint)
					return m, func() tea.Msg { return pushScreenMsg{screen: form} }
				}
				form := newKeyRegisterFormModel(m.keySvc, item.key.PrivateKeyPath)
				return m, func() tea.Msg { return pushScreenMsg{screen: form} }
			case key.Matches(typed, keySettings):
				settings := newSettingsModel(m.keySvc)
				return m, func() tea.Msg { return pushScreenMsg{screen: settings} }
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *keysListModel) View() string {
	view := m.list.View() + "\n" + StyleHelp.Render("↑/↓ navegar · enter detalhe · g gerar · o registrar órfã · s configurações · r recarregar · esc voltar")
	if m.agentChecked && !m.agentInfo.Detected {
		// Aviso discreto — nunca bloqueia a tela nem impede nenhuma ação
		// (spec ssh-agent-support, fluxo "nenhum agente acessível").
		view += "\n" + StyleMuted.Render("nenhum agente SSH detectado")
	}
	return view
}
