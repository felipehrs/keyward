package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// backupSelectionLoadedMsg carrega hosts e chaves numa única mensagem —
// carregados juntos (não via dois startAsync separados) porque o root só
// rastreia o reqID da ÚLTIMA operação disparada por tela (ver model.go);
// duas chamadas assíncronas concorrentes fariam a primeira resposta ser
// descartada como obsoleta.
type backupSelectionLoadedMsg struct {
	hosts []core.Host
	keys  []core.Key
	err   error
}

// backupExportDoneMsg é o resultado de BackupService.Export.
type backupExportDoneMsg struct{ err error }

type exportHostRow struct {
	host     core.Host
	included bool
}

type exportKeyRow struct {
	key            core.Key
	included       bool
	includePrivate bool
}

// backupExportModel é a tela de seleção de hosts/chaves pra
// BackupService.Export — seleção multi-item com toggle independente de
// "incluir chave privada" por chave (spec 6: aviso de alto contraste
// enquanto qualquer uma estiver marcada) e toggle de incluir settings.
type backupExportModel struct {
	configSvc core.ConfigService
	keySvc    core.KeyService
	backupSvc core.BackupService

	loaded          bool
	hosts           []exportHostRow
	keys            []exportKeyRow
	includeSettings bool
	destPath        textinput.Model

	cursor        int
	validationErr error
}

func newBackupExportModel(configSvc core.ConfigService, keySvc core.KeyService, backupSvc core.BackupService) *backupExportModel {
	m := &backupExportModel{
		configSvc: configSvc,
		keySvc:    keySvc,
		backupSvc: backupSvc,
		destPath:  textinput.New(),
	}
	m.destPath.Placeholder = "ex: ~/backup-keyward.tar.gz"
	return m
}

func (m *backupExportModel) Init() tea.Cmd { return m.reload() }

func (m *backupExportModel) reload() tea.Cmd {
	configSvc := m.configSvc
	keySvc := m.keySvc
	return startAsync(func() tea.Msg {
		hosts, err := configSvc.ListHosts()
		if err != nil {
			return backupSelectionLoadedMsg{err: err}
		}
		keys, err := keySvc.ListKeys()
		if err != nil {
			return backupSelectionLoadedMsg{err: err}
		}
		return backupSelectionLoadedMsg{hosts: hosts, keys: keys}
	})
}

func (m *backupExportModel) totalRows() int   { return len(m.hosts) + len(m.keys) + 2 }
func (m *backupExportModel) settingsRow() int { return len(m.hosts) + len(m.keys) }
func (m *backupExportModel) destPathRow() int { return m.settingsRow() + 1 }

func (m *backupExportModel) toggleCurrent() {
	switch {
	case m.cursor < len(m.hosts):
		m.hosts[m.cursor].included = !m.hosts[m.cursor].included
	case m.cursor < len(m.hosts)+len(m.keys):
		idx := m.cursor - len(m.hosts)
		m.keys[idx].included = !m.keys[idx].included
		if !m.keys[idx].included {
			m.keys[idx].includePrivate = false
		}
	case m.cursor == m.settingsRow():
		m.includeSettings = !m.includeSettings
	}
}

func (m *backupExportModel) togglePrivateCurrent() {
	if m.cursor < len(m.hosts) || m.cursor >= len(m.hosts)+len(m.keys) {
		return
	}
	idx := m.cursor - len(m.hosts)
	if !m.keys[idx].included {
		return
	}
	m.keys[idx].includePrivate = !m.keys[idx].includePrivate
}

func (m *backupExportModel) hasPrivateSelected() bool {
	for _, r := range m.keys {
		if r.included && r.includePrivate {
			return true
		}
	}
	return false
}

func (m *backupExportModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case backupSelectionLoadedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		m.hosts = make([]exportHostRow, len(typed.hosts))
		for i, h := range typed.hosts {
			m.hosts[i] = exportHostRow{host: h}
		}
		m.keys = make([]exportKeyRow, len(typed.keys))
		for i, k := range typed.keys {
			m.keys[i] = exportKeyRow{key: k}
		}
		m.loaded = true
		if m.cursor >= m.totalRows() {
			m.cursor = 0
		}
		return m, nil

	case backupExportDoneMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg(typed) }
		}
		return m, func() tea.Msg { return popScreenMsg{} }

	case tea.KeyMsg:
		switch typed.String() {
		case "esc":
			return m, func() tea.Msg { return popScreenMsg{} }
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down":
			if m.loaded && m.cursor < m.totalRows()-1 {
				m.cursor++
			}
			return m, nil
		case " ":
			if m.loaded {
				m.toggleCurrent()
			}
			return m, nil
		case "p":
			if m.loaded {
				m.togglePrivateCurrent()
			}
			return m, nil
		case "enter":
			if m.loaded {
				return m, m.submit()
			}
			return m, nil
		}
		if m.loaded && m.cursor == m.destPathRow() {
			var cmd tea.Cmd
			m.destPath, cmd = m.destPath.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *backupExportModel) submit() tea.Cmd {
	m.validationErr = nil

	dest := strings.TrimSpace(m.destPath.Value())
	if dest == "" {
		m.validationErr = fmt.Errorf("informe o caminho de destino do arquivo .tar.gz")
		return nil
	}

	var selectedHosts []core.Host
	for _, r := range m.hosts {
		if r.included {
			selectedHosts = append(selectedHosts, r.host)
		}
	}
	var selections []core.KeySelection
	for _, r := range m.keys {
		if !r.included {
			continue
		}
		selections = append(selections, core.KeySelection{
			Fingerprint:    r.key.Metadata.Fingerprint,
			IncludePrivate: r.includePrivate,
		})
	}

	opts := core.ExportOptions{Hosts: selectedHosts, Keys: selections, IncludeSettings: m.includeSettings}
	svc := m.backupSvc
	onConfirm := startAsync(func() tea.Msg {
		return backupExportDoneMsg{err: svc.Export(dest, opts)}
	})

	danger := m.hasPrivateSelected()
	body := fmt.Sprintf("%d host(s) e %d chave(s) serão exportados para %s.\nSe já existir um arquivo nesse caminho, será sobrescrito.",
		len(selectedHosts), len(selections), dest)
	if danger {
		body += "\n\nATENÇÃO: o pacote incluirá chave(s) PRIVADA(S) em texto claro — armazene o arquivo resultante com segurança."
	}

	return func() tea.Msg {
		return requestConfirmMsg{
			title:     "Exportar backup?",
			body:      body,
			danger:    danger,
			onConfirm: onConfirm,
		}
	}
}

func (m *backupExportModel) View() string {
	if !m.loaded {
		return StyleTitle.Render("Exportar backup") + "\n\ncarregando..."
	}

	var b strings.Builder
	b.WriteString(StyleTitle.Render("Exportar backup") + "\n\n")

	if m.hasPrivateSelected() {
		b.WriteString(StyleHighContrastWarn.Render("ATENÇÃO: inclui chave(s) privada(s) em texto claro") + "\n\n")
	}

	b.WriteString(StyleTitle.Render("Hosts") + "\n")
	if len(m.hosts) == 0 {
		b.WriteString(StyleMuted.Render("nenhum host encontrado") + "\n")
	}
	for i, r := range m.hosts {
		b.WriteString(m.renderRow(i, checkbox(r.included)+" "+strings.Join(r.host.Patterns, ", ")) + "\n")
	}

	b.WriteString("\n" + StyleTitle.Render("Chaves") + "\n")
	if len(m.keys) == 0 {
		b.WriteString(StyleMuted.Render("nenhuma chave encontrada") + "\n")
	}
	for i, r := range m.keys {
		idx := len(m.hosts) + i
		label := r.key.Metadata.Label
		if label == "" {
			label = "-"
		}
		priv := ""
		if r.included {
			priv = "  [privada: não]"
			if r.includePrivate {
				priv = "  " + StyleHighContrastWarn.Render("[privada: SIM]")
			}
		}
		b.WriteString(m.renderRow(idx, checkbox(r.included)+" "+label+priv) + "\n")
	}

	b.WriteString("\n" + m.renderRow(m.settingsRow(), checkbox(m.includeSettings)+" Incluir configurações (AppSettings)") + "\n")

	destLine := "Destino: " + m.destPath.View()
	if m.cursor == m.destPathRow() {
		destLine = "> " + destLine
	} else {
		destLine = "  " + destLine
	}
	b.WriteString("\n" + destLine + "\n")

	if m.validationErr != nil {
		b.WriteString("\n" + StyleDanger.Render("erro: "+m.validationErr.Error()) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("↑/↓ navegar · space incluir/excluir · p incluir privada (chaves) · enter exportar · esc voltar"))
	return b.String()
}

func (m *backupExportModel) renderRow(row int, content string) string {
	cursor := "  "
	if m.cursor == row {
		cursor = "> "
		content = StyleSelected.Render(content)
	}
	return cursor + content
}

func checkbox(on bool) string {
	if on {
		return "[x]"
	}
	return "[ ]"
}
