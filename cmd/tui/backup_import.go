package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// backupImportPreviewMsg é o resultado de BackupService.PreviewImport.
type backupImportPreviewMsg struct {
	preview core.ImportPreview
	err     error
}

// backupImportDoneMsg é o resultado de BackupService.Import. result é
// sempre renderizado mesmo quando err != nil — Import retorna os dois
// juntos (falhas isoladas por item não abortam os demais).
type backupImportDoneMsg struct {
	result core.ImportResult
	err    error
}

type importRowKind int

const (
	rowHostToAdd importRowKind = iota
	rowHostUnchanged
	rowHostConflict
	rowKeyToAdd
	rowKeyUnchanged
	rowKeyConflict
	rowSettings
)

// importRow é uma linha da tela de preview — uma por host/chave em
// ToAdd/Unchanged/Conflicts, mais uma linha informativa se o pacote
// incluir settings. hostSkip/keySkip cobrem cherry-pick (excluir um item
// sem conflito); hostRes/keyRes cobrem a resolução escolhida pra um
// conflito — só um dos dois pares é relevante, dependendo de kind.
type importRow struct {
	kind importRowKind

	host         core.Host
	hostConflict core.HostConflict
	keyMeta      core.KeyMetadata
	keyConflict  core.KeyConflict

	hostSkip bool
	keySkip  bool
	hostRes  core.HostResolution
	keyRes   core.KeyResolution
}

func buildImportRows(p core.ImportPreview) []importRow {
	var rows []importRow
	for _, h := range p.HostsToAdd {
		rows = append(rows, importRow{kind: rowHostToAdd, host: h})
	}
	for _, h := range p.HostsUnchanged {
		rows = append(rows, importRow{kind: rowHostUnchanged, host: h})
	}
	for _, c := range p.HostConflicts {
		rows = append(rows, importRow{kind: rowHostConflict, hostConflict: c, hostRes: core.HostResolutionSkip})
	}
	for _, k := range p.KeysToAdd {
		rows = append(rows, importRow{kind: rowKeyToAdd, keyMeta: k})
	}
	for _, k := range p.KeysUnchanged {
		rows = append(rows, importRow{kind: rowKeyUnchanged, keyMeta: k})
	}
	for _, c := range p.KeyConflicts {
		rows = append(rows, importRow{kind: rowKeyConflict, keyConflict: c, keyRes: core.KeyResolutionSkip})
	}
	if p.Settings != nil {
		rows = append(rows, importRow{kind: rowSettings})
	}
	return rows
}

const (
	importPhasePath = iota
	importPhasePreview
	importPhaseResult
)

// backupImportModel cobre o fluxo completo de import em três fases
// (caminho de origem → preview com resolução item-a-item → resultado),
// num único model — mais simples que empilhar telas via pushScreenMsg pra
// um fluxo linear que sempre volta ao ponto anterior em vez de navegar
// livremente.
type backupImportModel struct {
	backupSvc core.BackupService

	phase   int
	srcPath textinput.Model

	preview core.ImportPreview
	rows    []importRow
	cursor  int

	result    core.ImportResult
	resultErr error

	validationErr error
}

func newBackupImportModel(backupSvc core.BackupService) *backupImportModel {
	m := &backupImportModel{backupSvc: backupSvc, srcPath: textinput.New(), phase: importPhasePath}
	m.srcPath.Placeholder = "ex: ~/backup-keyward.tar.gz"
	m.srcPath.Focus()
	return m
}

func (m *backupImportModel) Init() tea.Cmd { return nil }

func (m *backupImportModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case backupImportPreviewMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		m.preview = typed.preview
		m.rows = buildImportRows(typed.preview)
		m.cursor = 0
		m.phase = importPhasePreview
		return m, nil

	case backupImportDoneMsg:
		m.result = typed.result
		m.resultErr = typed.err
		m.phase = importPhaseResult
		return m, nil

	case tea.KeyMsg:
		switch m.phase {
		case importPhasePath:
			return m.updatePath(typed)
		case importPhasePreview:
			return m.updatePreview(typed)
		default: // importPhaseResult
			if typed.String() == "esc" || typed.String() == "enter" {
				return m, func() tea.Msg { return popScreenMsg{} }
			}
		}
	}
	return m, nil
}

func (m *backupImportModel) updatePath(msg tea.KeyMsg) (screen, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return popScreenMsg{} }
	case "enter":
		path := strings.TrimSpace(m.srcPath.Value())
		if path == "" {
			m.validationErr = fmt.Errorf("informe o caminho do arquivo .tar.gz a importar")
			return m, nil
		}
		m.validationErr = nil
		svc := m.backupSvc
		return m, startAsync(func() tea.Msg {
			preview, err := svc.PreviewImport(path)
			return backupImportPreviewMsg{preview: preview, err: err}
		})
	}
	var cmd tea.Cmd
	m.srcPath, cmd = m.srcPath.Update(msg)
	return m, cmd
}

func (m *backupImportModel) updatePreview(msg tea.KeyMsg) (screen, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.phase = importPhasePath
		return m, nil
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case " ":
		m.cycleCurrent()
	case "enter", "c":
		return m, m.requestApply()
	}
	return m, nil
}

func (m *backupImportModel) cycleCurrent() {
	if len(m.rows) == 0 {
		return
	}
	r := &m.rows[m.cursor]
	switch r.kind {
	case rowHostToAdd:
		r.hostSkip = !r.hostSkip
	case rowHostConflict:
		if r.hostRes == core.HostResolutionSkip {
			r.hostRes = core.HostResolutionApply
		} else {
			r.hostRes = core.HostResolutionSkip
		}
	case rowKeyToAdd:
		r.keySkip = !r.keySkip
	case rowKeyConflict:
		switch r.keyRes {
		case core.KeyResolutionSkip:
			r.keyRes = core.KeyResolutionOverwrite
		case core.KeyResolutionOverwrite:
			if r.keyConflict.Kind == core.KeyConflictFileNameCollision {
				r.keyRes = core.KeyResolutionImportRenamed
			} else {
				r.keyRes = core.KeyResolutionSkip
			}
		case core.KeyResolutionImportRenamed:
			r.keyRes = core.KeyResolutionSkip
		}
	}
}

// resolutions monta core.ImportResolutions a partir do estado atual das
// linhas. Chave de hosts sem conflito calculada via core.HostImportKey —
// usar qualquer outra chave faria um cherry-pick de Skip virar no-op
// silencioso (ver core/backup.go).
func (m *backupImportModel) resolutions() core.ImportResolutions {
	hosts := map[string]core.HostResolution{}
	keys := map[string]core.KeyResolution{}
	for _, r := range m.rows {
		switch r.kind {
		case rowHostToAdd:
			if r.hostSkip {
				hosts[core.HostImportKey(r.host.SourceFile, r.host.Patterns)] = core.HostResolutionSkip
			}
		case rowHostConflict:
			hosts[r.hostConflict.Key] = r.hostRes
		case rowKeyToAdd:
			if r.keySkip {
				keys[r.keyMeta.Fingerprint] = core.KeyResolutionSkip
			}
		case rowKeyConflict:
			keys[r.keyConflict.Key] = r.keyRes
		}
	}
	return core.ImportResolutions{Hosts: hosts, Keys: keys}
}

// summaryIsDanger indica se o resumo do import tem algo irreversível o
// bastante pra pedir confirmação de alto contraste: substituir host
// existente, sobrescrever arquivo de chave, ou (sempre, incondicionalmente
// — ver nota no plano) o pacote incluir settings.
func (m *backupImportModel) summaryIsDanger() bool {
	if m.preview.Settings != nil {
		return true
	}
	for _, r := range m.rows {
		if r.kind == rowHostConflict && r.hostRes == core.HostResolutionApply {
			return true
		}
		if r.kind == rowKeyConflict && r.keyRes == core.KeyResolutionOverwrite {
			return true
		}
	}
	return false
}

func (m *backupImportModel) requestApply() tea.Cmd {
	resolutions := m.resolutions()
	danger := m.summaryIsDanger()

	body := "Aplicar o import conforme as resoluções escolhidas em cada item?"
	if m.preview.Settings != nil {
		body += "\n\nATENÇÃO: o pacote inclui configurações (AppSettings), que serão aplicadas " +
			"automaticamente — o core não permite excluir só esse item do import."
	}

	svc := m.backupSvc
	src := strings.TrimSpace(m.srcPath.Value())
	onConfirm := startAsync(func() tea.Msg {
		result, err := svc.Import(src, resolutions)
		return backupImportDoneMsg{result: result, err: err}
	})

	return func() tea.Msg {
		return requestConfirmMsg{title: "Confirmar import?", body: body, danger: danger, onConfirm: onConfirm}
	}
}

func (m *backupImportModel) View() string {
	switch m.phase {
	case importPhasePath:
		return m.viewPath()
	case importPhasePreview:
		return m.viewPreview()
	default:
		return m.viewResult()
	}
}

func (m *backupImportModel) viewPath() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("Importar backup") + "\n\n")
	b.WriteString("Origem: " + m.srcPath.View() + "\n")
	if m.validationErr != nil {
		b.WriteString("\n" + StyleDanger.Render("erro: "+m.validationErr.Error()) + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("enter pré-visualizar · esc voltar"))
	return b.String()
}

func (m *backupImportModel) viewPreview() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("Preview do import") + "\n\n")
	if len(m.rows) == 0 {
		b.WriteString(StyleMuted.Render("pacote vazio — nada a importar") + "\n")
	}
	for i, r := range m.rows {
		b.WriteString(m.renderImportRow(i, r) + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render(
		"↑/↓ navegar · space alternar resolução · enter/c confirmar · esc escolher outro arquivo"))
	return b.String()
}

func (m *backupImportModel) renderImportRow(i int, r importRow) string {
	cursor := "  "
	if m.cursor == i {
		cursor = "> "
	}

	var content string
	switch r.kind {
	case rowHostToAdd:
		state := "adicionar"
		if r.hostSkip {
			state = StyleMuted.Render("pulado")
		}
		content = "[+] " + strings.Join(r.host.Patterns, ", ") + "  (" + state + ")"
	case rowHostUnchanged:
		content = StyleMuted.Render("[=] " + strings.Join(r.host.Patterns, ", ") + "  (sem mudança)")
	case rowHostConflict:
		state := "pular"
		if r.hostRes == core.HostResolutionApply {
			state = StyleDanger.Render("substituir")
		}
		content = "[!] " + strings.Join(r.hostConflict.Host.Patterns, ", ") + " em " +
			r.hostConflict.DestinationFile + "  (" + state + ")"
	case rowKeyToAdd:
		label := keyMetaLabel(r.keyMeta)
		state := "adicionar"
		if r.keySkip {
			state = StyleMuted.Render("pulado")
		}
		content = "[+] " + label + "  (" + state + ")"
	case rowKeyUnchanged:
		content = StyleMuted.Render("[=] " + keyMetaLabel(r.keyMeta) + "  (sem mudança)")
	case rowKeyConflict:
		label := keyMetaLabel(r.keyConflict.Metadata)
		kind := "já registrada"
		if r.keyConflict.Kind == core.KeyConflictFileNameCollision {
			kind = "colisão de nome de arquivo"
		}
		state := "pular"
		switch r.keyRes {
		case core.KeyResolutionOverwrite:
			state = StyleDanger.Render("sobrescrever")
		case core.KeyResolutionImportRenamed:
			state = StyleWarn.Render("importar renomeada")
		}
		content = "[!] " + label + " (" + kind + ")  (" + state + ")"
	case rowSettings:
		content = StyleMuted.Render("[i] configurações do pacote serão aplicadas automaticamente ao confirmar")
	}

	interactive := r.kind != rowHostUnchanged && r.kind != rowKeyUnchanged && r.kind != rowSettings
	if m.cursor == i && interactive {
		content = StyleSelected.Render(content)
	}
	return cursor + content
}

func keyMetaLabel(k core.KeyMetadata) string {
	if k.Label != "" {
		return k.Label
	}
	return k.Fingerprint
}

func (m *backupImportModel) viewResult() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("Resultado do import") + "\n\n")
	fmt.Fprintf(&b, "Hosts: %d adicionados, %d substituídos, %d sem mudança, %d pulados\n",
		len(m.result.HostsAdded), len(m.result.HostsReplaced), len(m.result.HostsUnchanged), len(m.result.HostsSkipped))
	fmt.Fprintf(&b, "Chaves: %d adicionadas, %d sem mudança, %d metadata atualizada, %d arquivo sobrescrito, %d renomeadas, %d puladas\n",
		len(m.result.KeysAdded), len(m.result.KeysUnchanged), len(m.result.KeysMetadataUpdated),
		len(m.result.KeysFileOverwritten), len(m.result.KeysImportedRenamed), len(m.result.KeysSkipped))
	if m.result.SettingsApplied {
		b.WriteString("Configurações aplicadas.\n")
	}

	if len(m.result.Errors) > 0 {
		b.WriteString("\n" + StyleDanger.Render(fmt.Sprintf("%d erro(s):", len(m.result.Errors))) + "\n")
		for _, e := range m.result.Errors {
			b.WriteString(StyleDanger.Render("  - "+e.Error()) + "\n")
		}
	} else if m.resultErr != nil {
		b.WriteString("\n" + StyleDanger.Render("erro: "+m.resultErr.Error()) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("enter/esc voltar"))
	return b.String()
}
