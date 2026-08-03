package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// dateLayout é o mesmo formato usado por cmd/cli (não compartilhado entre
// os dois comandos — cada um é independente por regra de arquitetura — mas
// mantido idêntico por consistência de UX).
const dateLayout = "2006-01-02"

// Índices dos campos do formulário de geração de chave, na ordem de
// navegação (tab/shift+tab).
const (
	fieldAlgorithm = iota
	fieldRSABits
	fieldLabel
	fieldNotes
	fieldFileName
	fieldDirectory
	fieldPassphrase
	fieldPassphraseConfirm
	fieldExpiresAt
	fieldCount
)

// keyGeneratedMsg é o resultado de KeyService.GenerateKey.
type keyGeneratedMsg struct {
	key core.Key
	err error
}

// keyFormModel é o formulário de geração de chave (KeyService.GenerateKey).
// Enter em qualquer campo de uma linha submete o formulário; em Notes
// (textarea), Enter insere quebra de linha — é a única exceção. Esc
// cancela e volta pra lista.
type keyFormModel struct {
	keySvc core.KeyService

	focus     int
	algorithm core.Algorithm

	rsaBits           textinput.Model
	label             textinput.Model
	fileName          textinput.Model
	directory         textinput.Model
	passphrase        textinput.Model
	passphraseConfirm textinput.Model
	expiresAt         textinput.Model
	notes             textarea.Model

	validationErr error
	width         int
}

func newKeyFormModel(keySvc core.KeyService) *keyFormModel {
	m := &keyFormModel{
		keySvc:            keySvc,
		algorithm:         core.AlgorithmEd25519,
		rsaBits:           textinput.New(),
		label:             textinput.New(),
		fileName:          textinput.New(),
		directory:         textinput.New(),
		passphrase:        textinput.New(),
		passphraseConfirm: textinput.New(),
		expiresAt:         textinput.New(),
		notes:             textarea.New(),
	}

	m.rsaBits.Placeholder = "4096"
	m.label.Placeholder = "rótulo (opcional)"
	m.fileName.Placeholder = "id_ed25519 (default por algoritmo)"
	m.directory.Placeholder = "~/.ssh (default)"
	m.passphrase.Placeholder = "sem passphrase"
	m.passphrase.EchoMode = textinput.EchoPassword
	m.passphraseConfirm.Placeholder = "confirme a passphrase"
	m.passphraseConfirm.EchoMode = textinput.EchoPassword
	m.expiresAt.Placeholder = dateLayout + " (opcional)"
	m.notes.Placeholder = "notas (opcional)"
	m.notes.SetHeight(3)

	m.setFocus(fieldAlgorithm)
	return m
}

func (m *keyFormModel) Init() tea.Cmd { return nil }

// singleLineInputs retorna, na ordem dos índices de campo, um ponteiro para
// cada textinput.Model correspondente — usado para navegação/blur/focus
// genéricos sem repetir um switch gigante em cada operação.
func (m *keyFormModel) singleLineInputs() map[int]*textinput.Model {
	return map[int]*textinput.Model{
		fieldRSABits:           &m.rsaBits,
		fieldLabel:             &m.label,
		fieldFileName:          &m.fileName,
		fieldDirectory:         &m.directory,
		fieldPassphrase:        &m.passphrase,
		fieldPassphraseConfirm: &m.passphraseConfirm,
		fieldExpiresAt:         &m.expiresAt,
	}
}

func (m *keyFormModel) setFocus(field int) tea.Cmd {
	for i, in := range m.singleLineInputs() {
		if i == field {
			continue
		}
		in.Blur()
	}
	m.notes.Blur()

	m.focus = field
	if in, ok := m.singleLineInputs()[field]; ok {
		return in.Focus()
	}
	if field == fieldNotes {
		return m.notes.Focus()
	}
	return nil
}

func (m *keyFormModel) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch typed := msg.(type) {
	case keyGeneratedMsg:
		return m.handleGenerated(typed)

	case tea.WindowSizeMsg:
		m.width = typed.Width
		return m, nil

	case tea.KeyMsg:
		switch typed.String() {
		case "esc":
			return m, func() tea.Msg { return popScreenMsg{} }
		case "tab":
			return m, m.setFocus((m.focus + 1) % fieldCount)
		case "shift+tab":
			return m, m.setFocus((m.focus - 1 + fieldCount) % fieldCount)
		case "left", "right":
			if m.focus == fieldAlgorithm {
				if m.algorithm == core.AlgorithmEd25519 {
					m.algorithm = core.AlgorithmRSA
				} else {
					m.algorithm = core.AlgorithmEd25519
				}
				return m, nil
			}
		case "enter":
			if m.focus != fieldNotes {
				return m, m.submit()
			}
		}
	}

	return m, m.updateFocusedField(msg)
}

// updateFocusedField repassa msg só pro widget com foco — os demais nunca
// veem teclas que não são deles (evita, ex., dígitos vazando pro campo de
// passphrase enquanto o usuário edita Label).
func (m *keyFormModel) updateFocusedField(msg tea.Msg) tea.Cmd {
	if m.focus == fieldNotes {
		var cmd tea.Cmd
		m.notes, cmd = m.notes.Update(msg)
		return cmd
	}
	in, ok := m.singleLineInputs()[m.focus]
	if !ok {
		return nil
	}
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	return cmd
}

// submit valida os campos localmente (mesma filosofia "falhar rápido" da
// CLI) e, se válidos, dispara GenerateKey em background.
func (m *keyFormModel) submit() tea.Cmd {
	m.validationErr = nil

	opts := core.GenerateKeyOptions{
		Algorithm: m.algorithm,
		Label:     m.label.Value(),
		Notes:     m.notes.Value(),
		FileName:  m.fileName.Value(),
		Directory: m.directory.Value(),
	}

	if m.algorithm == core.AlgorithmRSA && m.rsaBits.Value() != "" {
		bits, err := strconv.Atoi(m.rsaBits.Value())
		if err != nil {
			m.validationErr = fmt.Errorf("RSABits inválido: %q", m.rsaBits.Value())
			return nil
		}
		if bits < 4096 {
			m.validationErr = fmt.Errorf("RSABits deve ser >= 4096, obteve %d", bits)
			return nil
		}
		opts.RSABits = bits
	}

	if m.passphrase.Value() != m.passphraseConfirm.Value() {
		m.validationErr = fmt.Errorf("passphrase e confirmação não coincidem")
		return nil
	}
	if m.passphrase.Value() != "" {
		opts.Passphrase = []byte(m.passphrase.Value())
	}

	if raw := m.expiresAt.Value(); raw != "" {
		t, err := time.Parse(dateLayout, raw)
		if err != nil {
			m.validationErr = fmt.Errorf("data inválida %q (esperado formato %s)", raw, dateLayout)
			return nil
		}
		t = t.UTC()
		opts.ExpiresAt = &t
	}

	return m.generate(opts)
}

func (m *keyFormModel) generate(opts core.GenerateKeyOptions) tea.Cmd {
	svc := m.keySvc
	return startAsync(func() tea.Msg {
		k, err := svc.GenerateKey(opts)
		return keyGeneratedMsg{key: k, err: err}
	})
}

// overwriteConflict detecta o erro de arquivo já existente retornado por
// core/keygen.go — o core não expõe uma sentinela pra isso (limitação
// documentada no plano), então o texto é comparado por substring.
func overwriteConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "já existe (use Overwrite")
}

func (m *keyFormModel) handleGenerated(msg keyGeneratedMsg) (screen, tea.Cmd) {
	if msg.err == nil {
		return m, func() tea.Msg { return popScreenMsg{} }
	}

	if overwriteConflict(msg.err) {
		opts := core.GenerateKeyOptions{
			Algorithm: m.algorithm,
			Label:     m.label.Value(),
			Notes:     m.notes.Value(),
			FileName:  m.fileName.Value(),
			Directory: m.directory.Value(),
			Overwrite: true,
		}
		if m.passphrase.Value() != "" {
			opts.Passphrase = []byte(m.passphrase.Value())
		}
		return m, func() tea.Msg {
			return requestConfirmMsg{
				title:     "Sobrescrever chave existente?",
				body:      msg.err.Error(),
				danger:    true,
				onConfirm: m.generate(opts),
			}
		}
	}

	return m, func() tea.Msg { return errMsg{err: msg.err} }
}

func (m *keyFormModel) View() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("Gerar chave") + "\n\n")

	algo := string(m.algorithm)
	if m.focus == fieldAlgorithm {
		algo = StyleSelected.Render(algo)
	}
	b.WriteString("Algoritmo (←/→): " + algo + "\n")
	if m.algorithm == core.AlgorithmRSA {
		b.WriteString("RSA bits:        " + m.rsaBits.View() + "\n")
	}
	b.WriteString("Rótulo:          " + m.label.View() + "\n")
	b.WriteString("Notas:\n" + m.notes.View() + "\n")
	b.WriteString("Nome do arquivo: " + m.fileName.View() + "\n")
	b.WriteString("Diretório:       " + m.directory.View() + "\n")
	b.WriteString("Passphrase:      " + m.passphrase.View() + "\n")
	b.WriteString("Confirmar:       " + m.passphraseConfirm.View() + "\n")
	b.WriteString("Expira em:       " + m.expiresAt.View() + "\n")

	if m.validationErr != nil {
		b.WriteString("\n" + StyleDanger.Render("erro: "+m.validationErr.Error()) + "\n")
	}

	b.WriteString("\n" + StyleHelp.Render("tab/shift+tab navegar · enter submeter (exceto em notas) · esc cancelar"))
	return b.String()
}
