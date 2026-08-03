package main

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// screen é a interface que toda tela (sub-model) da TUI implementa. Existe
// em vez de usar tea.Model puro para evitar type assertion a cada
// navegação na pilha do root.
type screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (screen, tea.Cmd)
	View() string
}

// rootModel é o model raiz da TUI: dono da navegação (pilha de screen), do
// modal de confirmação, e do único spinner global usado por toda operação
// assíncrona (ver regra de roteamento em Update).
type rootModel struct {
	keySvc    core.KeyService
	configSvc core.ConfigService
	backupSvc core.BackupService

	active screen
	stack  []screen

	confirm *confirmModel

	spin      spinner.Model
	inFlight  int // operações assíncronas em voo — contador, não bool (ver Update)
	reqSeq    int // gerador monotônico de reqID
	lastReqID int // reqID da última operação disparada; respostas com reqID diferente são obsoletas

	lastErr error

	width, height int
}

func newRootModel(keySvc core.KeyService, configSvc core.ConfigService, backupSvc core.BackupService) rootModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return rootModel{
		keySvc:    keySvc,
		configSvc: configSvc,
		backupSvc: backupSvc,
		active:    newMenuModel(configSvc, keySvc, backupSvc),
		spin:      sp,
	}
}

func (m rootModel) Init() tea.Cmd {
	return m.active.Init()
}

// enterActive inicializa a tela recém-ativada (Init) e, se o root já
// conhece as dimensões do terminal (WindowSizeMsg só chega do runtime uma
// vez no início e a cada resize real — nunca de novo só porque a tela
// ativa mudou), replica esse tamanho conhecido pra ela imediatamente.
// Sem isso, toda tela empilhada depois da primeira WindowSizeMsg nasce com
// width/height zerados (list.Model sem SetSize nunca chamado) e renderiza
// sem título nem itens.
func (m *rootModel) enterActive() tea.Cmd {
	initCmd := m.active.Init()
	if m.width <= 0 || m.height <= 0 {
		return initCmd
	}
	var resizeCmd tea.Cmd
	m.active, resizeCmd = m.active.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return tea.Batch(initCmd, resizeCmd)
}

// Update implementa a regra única de roteamento de mensagens (ver plano,
// seção "Arquitetura Bubble Tea"):
//
//  1. tea.WindowSizeMsg e spinner.TickMsg sempre em broadcast: vão para a
//     tela ativa E para o modal de confirmação, se houver — evita que a
//     tela sob o modal perca SetSize e volte com layout errado.
//  2. tea.KeyMsg: se há modal ativo, vai só pra ele (nunca vaza pro
//     formulário atrás). Caso contrário, vai pra tela ativa.
//  3. Mensagens de resultado assíncrono (asyncResultMsg) carregam reqID;
//     respostas com reqID diferente do último disparado são descartadas
//     silenciosamente (usuário já navegou pra outro lugar).
//  4. Navegação e confirm/erro são housekeeping tratado só aqui, nunca
//     repassado à tela ativa.
func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = typed.Width, typed.Height
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.active, cmd = m.active.Update(msg)
		cmds = append(cmds, cmd)
		if m.confirm != nil {
			m.confirm, cmd = m.confirm.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		if m.inFlight == 0 {
			// Nenhuma operação em voo: para de realimentar o tick, o que
			// interrompe a animação (spinner.Update sempre re-agenda outro
			// tick enquanto for alimentado).
			return m, nil
		}
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		cmds = append(cmds, cmd)
		m.active, cmd = m.active.Update(msg)
		cmds = append(cmds, cmd)
		if m.confirm != nil {
			m.confirm, cmd = m.confirm.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		if m.confirm != nil {
			var cmd tea.Cmd
			m.confirm, cmd = m.confirm.Update(msg)
			return m, cmd
		}
		// Sem modal ativo: cai no repasse genérico ao final da função.

	case pushScreenMsg:
		m.stack = append(m.stack, m.active)
		m.active = typed.screen
		return m, m.enterActive()

	case popScreenMsg:
		if len(m.stack) == 0 {
			return m, nil
		}
		m.active = m.stack[len(m.stack)-1]
		m.stack = m.stack[:len(m.stack)-1]
		return m, m.enterActive()

	case gotoMenuMsg:
		m.stack = nil
		m.active = newMenuModel(m.configSvc, m.keySvc, m.backupSvc)
		return m, m.enterActive()

	case requestConfirmMsg:
		m.confirm = newConfirmModel(typed.title, typed.body, typed.danger, typed.onConfirm)
		return m, nil

	case errMsg:
		m.lastErr = typed.err
		return m, nil

	case startAsyncMsg:
		m.reqSeq++
		id := m.reqSeq
		m.lastReqID = id
		m.inFlight++
		run := typed.run
		resultCmd := func() tea.Msg {
			return asyncResultMsg{reqID: id, msg: run()}
		}
		var spinCmd tea.Cmd
		if m.inFlight == 1 {
			spinCmd = m.spin.Tick
		}
		return m, tea.Batch(resultCmd, spinCmd)

	case asyncResultMsg:
		if m.inFlight > 0 {
			m.inFlight--
		}
		if typed.reqID != m.lastReqID {
			return m, nil // resposta obsoleta — descartada
		}
		var cmd tea.Cmd
		m.active, cmd = m.active.Update(typed.msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.active, cmd = m.active.Update(msg)
	return m, cmd
}

func (m rootModel) View() string {
	if m.confirm != nil {
		return m.confirm.View()
	}

	view := m.active.View()
	if m.inFlight > 0 {
		view += "\n\n" + m.spin.View() + " carregando..."
	}
	if m.lastErr != nil {
		view += "\n\n" + StyleDanger.Render("erro: "+m.lastErr.Error())
	}
	return view
}
