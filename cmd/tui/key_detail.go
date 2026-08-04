package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// keyDetailLoadedMsg é o resultado de KeyService.GetKey.
type keyDetailLoadedMsg struct {
	key core.Key
	err error
}

// keyUnregisteredMsg é o resultado de KeyService.Unregister.
type keyUnregisteredMsg struct{ err error }

// keyDetailModel mostra os dados de uma única chave (KeyService.GetKey) e
// oferece editar metadata ou fazer unregister. Navegação por fingerprint —
// diferente de KeyStatusMissingFile, uma chave KeyStatusUnregistered não
// tem Metadata.Fingerprint preenchido pelo core (limitação conhecida:
// reconcileKeys só popula Key.Metadata quando há registro), então
// keys_list.go não permite abrir o detalhe dessas chaves — só "o" para
// registrar primeiro.
type keyDetailModel struct {
	keySvc core.KeyService

	fingerprint string
	key         core.Key
	loaded      bool
}

func newKeyDetailModel(keySvc core.KeyService, fingerprint string) *keyDetailModel {
	return &keyDetailModel{keySvc: keySvc, fingerprint: fingerprint}
}

func (m *keyDetailModel) Init() tea.Cmd { return m.reload() }

func (m *keyDetailModel) reload() tea.Cmd {
	svc := m.keySvc
	fp := m.fingerprint
	return startAsync(func() tea.Msg {
		k, err := svc.GetKey(fp)
		return keyDetailLoadedMsg{key: k, err: err}
	})
}

func (m *keyDetailModel) requestUnregister() tea.Cmd {
	svc := m.keySvc
	fp := m.fingerprint
	onConfirm := startAsync(func() tea.Msg {
		return keyUnregisteredMsg{err: svc.Unregister(fp)}
	})
	return func() tea.Msg {
		return requestConfirmMsg{
			title: "Remover registro de metadata?",
			body: "O arquivo da chave permanece em disco — só o registro de metadata " +
				"(rótulo, notas, expiração) é removido. A chave volta a aparecer como " +
				"'sem registro de metadata' na lista.",
			danger:    false,
			onConfirm: onConfirm,
		}
	}
}

func (m *keyDetailModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case keyDetailLoadedMsg:
		if typed.err != nil {
			if errors.Is(typed.err, core.ErrKeyNotFound) {
				// Chave sumiu por fora (outra sessão/processo) — volta pra
				// lista em vez de deixar o usuário preso numa tela de
				// detalhe de algo que não existe mais; a lista recarrega
				// sozinha ao ficar ativa de novo.
				return m, tea.Batch(
					func() tea.Msg {
						return errMsg{err: fmt.Errorf("chave não encontrada — pode ter sido removida em outra sessão")}
					},
					func() tea.Msg { return popScreenMsg{} },
				)
			}
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		m.key = typed.key
		m.loaded = true
		return m, nil

	case keyMetadataUpdatedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		m.key = typed.key
		return m, nil

	case keyUnregisteredMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg(typed) }
		}
		return m, func() tea.Msg { return popScreenMsg{} }

	case tea.KeyMsg:
		switch {
		case key.Matches(typed, keyBack):
			return m, func() tea.Msg { return popScreenMsg{} }
		case key.Matches(typed, keyReload):
			return m, m.reload()
		case key.Matches(typed, keyEdit):
			if !m.loaded {
				return m, nil
			}
			form := newKeyMetadataFormModel(m.keySvc, m.key)
			return m, func() tea.Msg { return pushScreenMsg{screen: form} }
		case key.Matches(typed, keyUnregister):
			if !m.loaded {
				return m, nil
			}
			return m, m.requestUnregister()
		}
	}
	return m, nil
}

func (m *keyDetailModel) View() string {
	if !m.loaded {
		return StyleTitle.Render("Detalhe da chave") + "\n\ncarregando..."
	}

	k := m.key
	var b strings.Builder
	b.WriteString(StyleTitle.Render("Detalhe da chave") + "\n\n")

	label := k.Metadata.Label
	if label == "" {
		label = "-"
	}
	b.WriteString("Rótulo:      " + label + "\n")
	b.WriteString("Fingerprint: " + k.Metadata.Fingerprint + "\n")
	b.WriteString("Algoritmo:   " + string(k.Algorithm) + "\n")

	status := "OK"
	switch k.Status {
	case core.KeyStatusUnregistered:
		status = StyleMuted.Render("sem registro de metadata")
	case core.KeyStatusMissingFile:
		status = StyleMuted.Render("arquivo ausente em disco")
	}
	switch {
	case k.IsExpired:
		status += "  " + StyleDanger.Render("EXPIRADA")
	case k.IsExpiringSoon:
		status += "  " + StyleWarn.Render("EXPIRANDO")
	}
	b.WriteString("Status:      " + status + "\n")

	expires := "-"
	if k.Metadata.ExpiresAt != nil {
		expires = k.Metadata.ExpiresAt.Format(dateLayout)
	}
	b.WriteString("Expira em:   " + expires + "\n")

	notes := k.Metadata.Notes
	if notes == "" {
		notes = "-"
	}
	b.WriteString("Notas:       " + notes + "\n")

	privPath := k.PrivateKeyPath
	if privPath == "" {
		privPath = "-"
	}
	b.WriteString("Chave priv.: " + StyleMuted.Render(privPath) + "\n")

	b.WriteString("\n" + StyleHelp.Render("e editar · u unregister · r recarregar · esc voltar"))
	return b.String()
}

// --- Edição de metadata (UpdateMetadata) ---

// keyMetadataUpdatedMsg é o resultado de KeyService.UpdateMetadata.
type keyMetadataUpdatedMsg struct {
	key core.Key
	err error
}

const (
	metaFieldLabel = iota
	metaFieldNotes
	metaFieldExpiresAt
	metaFieldCount
)

// keyMetadataFormModel edita label/notes/expiresAt de uma chave já
// registrada. origExpiresAt guarda o valor formatado original do campo —
// usado pra distinguir os três estados que KeyMetadataPatch.ExpiresAt
// (**time.Time) suporta: campo igual ao original -> não altera (patch nil);
// campo vazio -> limpa a expiração (patch aponta pra *time.Time nil);
// campo com data nova -> define (patch aponta pro ponteiro da nova data).
type keyMetadataFormModel struct {
	keySvc      core.KeyService
	fingerprint string

	origExpiresAt string
	focus         int

	label     textinput.Model
	notes     textarea.Model
	expiresAt textinput.Model

	validationErr error
}

func newKeyMetadataFormModel(keySvc core.KeyService, k core.Key) *keyMetadataFormModel {
	m := &keyMetadataFormModel{
		keySvc:      keySvc,
		fingerprint: k.Metadata.Fingerprint,
		label:       textinput.New(),
		notes:       textarea.New(),
		expiresAt:   textinput.New(),
	}
	m.label.SetValue(k.Metadata.Label)
	m.notes.SetValue(k.Metadata.Notes)
	m.notes.SetHeight(3)
	if k.Metadata.ExpiresAt != nil {
		m.origExpiresAt = k.Metadata.ExpiresAt.Format(dateLayout)
	}
	m.expiresAt.SetValue(m.origExpiresAt)
	m.expiresAt.Placeholder = dateLayout + " (vazio = sem expiração)"
	m.label.Focus()
	return m
}

func (m *keyMetadataFormModel) Init() tea.Cmd { return nil }

func (m *keyMetadataFormModel) inputs() map[int]*textinput.Model {
	return map[int]*textinput.Model{metaFieldLabel: &m.label, metaFieldExpiresAt: &m.expiresAt}
}

func (m *keyMetadataFormModel) setFocus(field int) tea.Cmd {
	m.label.Blur()
	m.notes.Blur()
	m.expiresAt.Blur()
	m.focus = field
	switch field {
	case metaFieldLabel:
		return m.label.Focus()
	case metaFieldNotes:
		return m.notes.Focus()
	case metaFieldExpiresAt:
		return m.expiresAt.Focus()
	}
	return nil
}

func (m *keyMetadataFormModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case keyMetadataUpdatedMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		return m, func() tea.Msg { return popScreenMsg{} }

	case tea.KeyMsg:
		switch typed.String() {
		case "esc":
			return m, func() tea.Msg { return popScreenMsg{} }
		case "tab", "down":
			return m, m.setFocus((m.focus + 1) % metaFieldCount)
		case "shift+tab", "up":
			return m, m.setFocus((m.focus - 1 + metaFieldCount) % metaFieldCount)
		case "enter":
			if m.focus != metaFieldNotes {
				return m, m.submit()
			}
		}
	}

	if m.focus == metaFieldNotes {
		var cmd tea.Cmd
		m.notes, cmd = m.notes.Update(msg)
		return m, cmd
	}
	in := m.inputs()[m.focus]
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	return m, cmd
}

func (m *keyMetadataFormModel) submit() tea.Cmd {
	m.validationErr = nil

	label := m.label.Value()
	notes := m.notes.Value()
	patch := core.KeyMetadataPatch{Label: &label, Notes: &notes}

	raw := m.expiresAt.Value()
	switch raw {
	case m.origExpiresAt:
		// campo intocado — não altera ExpiresAt.
	case "":
		var cleared *time.Time
		patch.ExpiresAt = &cleared
	default:
		t, err := time.Parse(dateLayout, raw)
		if err != nil {
			m.validationErr = fmt.Errorf("data inválida %q (esperado formato %s)", raw, dateLayout)
			return nil
		}
		t = t.UTC()
		newVal := &t
		patch.ExpiresAt = &newVal
	}

	svc := m.keySvc
	fp := m.fingerprint
	return startAsync(func() tea.Msg {
		k, err := svc.UpdateMetadata(fp, patch)
		return keyMetadataUpdatedMsg{key: k, err: err}
	})
}

func (m *keyMetadataFormModel) View() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("Editar metadata") + "\n\n")
	b.WriteString("Rótulo:    " + m.label.View() + "\n")
	b.WriteString("Notas:\n" + m.notes.View() + "\n")
	b.WriteString("Expira em: " + m.expiresAt.View() + "\n")

	if m.validationErr != nil {
		b.WriteString("\n" + StyleDanger.Render("erro: "+m.validationErr.Error()) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("tab/shift+tab navegar · enter salvar · esc cancelar"))
	return b.String()
}

// --- Registro de chave órfã (RegisterKey) ---

// keyRegisteredMsg é o resultado de KeyService.RegisterKey.
type keyRegisteredMsg struct {
	key core.Key
	err error
}

const (
	registerFieldLabel = iota
	registerFieldNotes
	registerFieldExpiresAt
	registerFieldCount
)

// keyRegisterFormModel registra metadata para uma chave KeyStatusUnregistered
// já encontrada em disco (privateKeyPath vem da seleção em keys_list —
// não é editável aqui, a chave já foi escolhida).
type keyRegisterFormModel struct {
	keySvc         core.KeyService
	privateKeyPath string

	focus     int
	label     textinput.Model
	notes     textarea.Model
	expiresAt textinput.Model

	validationErr error
}

func newKeyRegisterFormModel(keySvc core.KeyService, privateKeyPath string) *keyRegisterFormModel {
	m := &keyRegisterFormModel{
		keySvc:         keySvc,
		privateKeyPath: privateKeyPath,
		label:          textinput.New(),
		notes:          textarea.New(),
		expiresAt:      textinput.New(),
	}
	m.notes.SetHeight(3)
	m.expiresAt.Placeholder = dateLayout + " (opcional)"
	m.label.Focus()
	return m
}

func (m *keyRegisterFormModel) Init() tea.Cmd { return nil }

func (m *keyRegisterFormModel) setFocus(field int) tea.Cmd {
	m.label.Blur()
	m.notes.Blur()
	m.expiresAt.Blur()
	m.focus = field
	switch field {
	case registerFieldLabel:
		return m.label.Focus()
	case registerFieldNotes:
		return m.notes.Focus()
	case registerFieldExpiresAt:
		return m.expiresAt.Focus()
	}
	return nil
}

func (m *keyRegisterFormModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case keyRegisteredMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		return m, func() tea.Msg { return popScreenMsg{} }

	case tea.KeyMsg:
		switch typed.String() {
		case "esc":
			return m, func() tea.Msg { return popScreenMsg{} }
		case "tab", "down":
			return m, m.setFocus((m.focus + 1) % registerFieldCount)
		case "shift+tab", "up":
			return m, m.setFocus((m.focus - 1 + registerFieldCount) % registerFieldCount)
		case "enter":
			if m.focus != registerFieldNotes {
				return m, m.submit()
			}
		}
	}

	switch m.focus {
	case registerFieldNotes:
		var cmd tea.Cmd
		m.notes, cmd = m.notes.Update(msg)
		return m, cmd
	case registerFieldExpiresAt:
		var cmd tea.Cmd
		m.expiresAt, cmd = m.expiresAt.Update(msg)
		return m, cmd
	default:
		var cmd tea.Cmd
		m.label, cmd = m.label.Update(msg)
		return m, cmd
	}
}

func (m *keyRegisterFormModel) submit() tea.Cmd {
	m.validationErr = nil

	patch := core.KeyMetadataPatch{}
	label := m.label.Value()
	patch.Label = &label
	notes := m.notes.Value()
	patch.Notes = &notes

	if raw := m.expiresAt.Value(); raw != "" {
		t, err := time.Parse(dateLayout, raw)
		if err != nil {
			m.validationErr = fmt.Errorf("data inválida %q (esperado formato %s)", raw, dateLayout)
			return nil
		}
		t = t.UTC()
		tp := &t
		patch.ExpiresAt = &tp
	}

	svc := m.keySvc
	path := m.privateKeyPath
	return startAsync(func() tea.Msg {
		k, err := svc.RegisterKey(path, patch)
		return keyRegisteredMsg{key: k, err: err}
	})
}

func (m *keyRegisterFormModel) View() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("Registrar chave") + "\n\n")
	b.WriteString("Arquivo:   " + StyleMuted.Render(m.privateKeyPath) + "\n")
	b.WriteString("Rótulo:    " + m.label.View() + "\n")
	b.WriteString("Notas:\n" + m.notes.View() + "\n")
	b.WriteString("Expira em: " + m.expiresAt.View() + "\n")

	if m.validationErr != nil {
		b.WriteString("\n" + StyleDanger.Render("erro: "+m.validationErr.Error()) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("tab/shift+tab navegar · enter registrar · esc cancelar"))
	return b.String()
}

// --- Anotação de identidade de agente sem registro (RegisterAgentKey) ---

// agentKeyRegisteredMsg é o resultado de KeyService.RegisterAgentKey.
type agentKeyRegisteredMsg struct {
	key core.Key
	err error
}

// agentKeyRegisterFormModel anota (Label/Notes/ExpiresAt) uma identidade já
// oferecida por um ssh-agent, mas ainda sem registro de metadata
// (KeyStatusUnregistered, Source == KeySourceAgent). Espelha
// keyRegisterFormModel, mas identifica a chave por fingerprint (já exposto
// por ListKeys pra esse caso — ver core/keys_agent.go) em vez de um caminho
// de arquivo, já que não há arquivo local envolvido.
type agentKeyRegisterFormModel struct {
	keySvc      core.KeyService
	fingerprint string

	focus     int
	label     textinput.Model
	notes     textarea.Model
	expiresAt textinput.Model

	validationErr error
}

func newAgentKeyRegisterFormModel(keySvc core.KeyService, fingerprint string) *agentKeyRegisterFormModel {
	m := &agentKeyRegisterFormModel{
		keySvc:      keySvc,
		fingerprint: fingerprint,
		label:       textinput.New(),
		notes:       textarea.New(),
		expiresAt:   textinput.New(),
	}
	m.notes.SetHeight(3)
	m.expiresAt.Placeholder = dateLayout + " (opcional)"
	m.label.Focus()
	return m
}

func (m *agentKeyRegisterFormModel) Init() tea.Cmd { return nil }

func (m *agentKeyRegisterFormModel) setFocus(field int) tea.Cmd {
	m.label.Blur()
	m.notes.Blur()
	m.expiresAt.Blur()
	m.focus = field
	switch field {
	case registerFieldLabel:
		return m.label.Focus()
	case registerFieldNotes:
		return m.notes.Focus()
	case registerFieldExpiresAt:
		return m.expiresAt.Focus()
	}
	return nil
}

func (m *agentKeyRegisterFormModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case agentKeyRegisteredMsg:
		if typed.err != nil {
			return m, func() tea.Msg { return errMsg{err: typed.err} }
		}
		return m, func() tea.Msg { return popScreenMsg{} }

	case tea.KeyMsg:
		switch typed.String() {
		case "esc":
			return m, func() tea.Msg { return popScreenMsg{} }
		case "tab", "down":
			return m, m.setFocus((m.focus + 1) % registerFieldCount)
		case "shift+tab", "up":
			return m, m.setFocus((m.focus - 1 + registerFieldCount) % registerFieldCount)
		case "enter":
			if m.focus != registerFieldNotes {
				return m, m.submit()
			}
		}
	}

	switch m.focus {
	case registerFieldNotes:
		var cmd tea.Cmd
		m.notes, cmd = m.notes.Update(msg)
		return m, cmd
	case registerFieldExpiresAt:
		var cmd tea.Cmd
		m.expiresAt, cmd = m.expiresAt.Update(msg)
		return m, cmd
	default:
		var cmd tea.Cmd
		m.label, cmd = m.label.Update(msg)
		return m, cmd
	}
}

func (m *agentKeyRegisterFormModel) submit() tea.Cmd {
	m.validationErr = nil

	patch := core.KeyMetadataPatch{}
	label := m.label.Value()
	patch.Label = &label
	notes := m.notes.Value()
	patch.Notes = &notes

	if raw := m.expiresAt.Value(); raw != "" {
		t, err := time.Parse(dateLayout, raw)
		if err != nil {
			m.validationErr = fmt.Errorf("data inválida %q (esperado formato %s)", raw, dateLayout)
			return nil
		}
		t = t.UTC()
		tp := &t
		patch.ExpiresAt = &tp
	}

	svc := m.keySvc
	fp := m.fingerprint
	return startAsync(func() tea.Msg {
		k, err := svc.RegisterAgentKey(fp, patch)
		return agentKeyRegisteredMsg{key: k, err: err}
	})
}

func (m *agentKeyRegisterFormModel) View() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("Anotar chave de agente") + "\n\n")
	b.WriteString("Fingerprint: " + StyleMuted.Render(m.fingerprint) + "\n")
	b.WriteString("Rótulo:      " + m.label.View() + "\n")
	b.WriteString("Notas:\n" + m.notes.View() + "\n")
	b.WriteString("Expira em:   " + m.expiresAt.View() + "\n")

	if m.validationErr != nil {
		b.WriteString("\n" + StyleDanger.Render("erro: "+m.validationErr.Error()) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("tab/shift+tab navegar · enter registrar · esc cancelar"))
	return b.String()
}
