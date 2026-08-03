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
	switch {
	case i.key.IsExpired:
		title += "  " + StyleDanger.Render("EXPIRADA")
	case i.key.IsExpiringSoon:
		title += "  " + StyleWarn.Render("EXPIRANDO")
	}
	switch i.key.Status {
	case core.KeyStatusUnregistered:
		title += "  " + StyleMuted.Render("(sem registro de metadata)")
	case core.KeyStatusMissingFile:
		title += "  " + StyleMuted.Render("(arquivo ausente em disco)")
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

// keysListModel é a tela de listagem de chaves (KeyService.ListKeys).
// Recarrega sempre que fica ativa (Init) e sob pedido explícito (tecla r).
type keysListModel struct {
	keySvc core.KeyService
	list   list.Model
}

func newKeysListModel(keySvc core.KeyService) *keysListModel {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.Title = "Chaves"
	l.DisableQuitKeybindings()
	return &keysListModel{keySvc: keySvc, list: l}
}

func (m *keysListModel) Init() tea.Cmd {
	return m.reload()
}

func (m *keysListModel) reload() tea.Cmd {
	svc := m.keySvc
	return startAsync(func() tea.Msg {
		keys, err := svc.ListKeys()
		return keysLoadedMsg{keys: keys, err: err}
	})
}

func (m *keysListModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case keysLoadedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		items := make([]list.Item, len(typed.keys))
		for i, k := range typed.keys {
			items[i] = keyItem{key: k}
		}
		return m, m.list.SetItems(items)

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
	return m.list.View() + "\n" + StyleHelp.Render("↑/↓ navegar · enter detalhe · g gerar · o registrar órfã · s configurações · r recarregar · esc voltar")
}
