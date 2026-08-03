package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// fakeScreen é um screen mínimo para testar o roteamento do root sem
// depender de telas reais — registra toda mensagem recebida, na ordem.
type fakeScreen struct {
	updates []tea.Msg
}

func (f *fakeScreen) Init() tea.Cmd { return nil }

func (f *fakeScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	f.updates = append(f.updates, msg)
	return f, nil
}

func (f *fakeScreen) View() string { return "" }

func newTestRoot() (rootModel, *fakeScreen) {
	fs := &fakeScreen{}
	return rootModel{active: fs, spin: spinner.New()}, fs
}

func TestRoot_KeyMsg_RoutesToConfirmWhenActive(t *testing.T) {
	m, fs := newTestRoot()
	confirmed := false
	m.confirm = newConfirmModel("t", "b", false, func() tea.Msg {
		confirmed = true
		return nil
	})

	// tab alterna o foco do modal (0 -> 1), sem tocar na tela ativa.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(rootModel)
	if len(fs.updates) != 0 {
		t.Fatalf("tea.KeyMsg vazou pro screen ativo com modal aberto: %v", fs.updates)
	}
	if m.confirm == nil {
		t.Fatal("modal deveria continuar aberto após tab")
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(rootModel)
	if m.confirm != nil {
		t.Fatal("modal deveria fechar após confirmar")
	}
	if cmd == nil {
		t.Fatal("esperava o onConfirm Cmd de volta")
	}
	cmd()
	if !confirmed {
		t.Fatal("onConfirm não foi disparado")
	}
}

func TestRoot_KeyMsg_RoutesToActiveWhenNoConfirm(t *testing.T) {
	m, fs := newTestRoot()
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if len(fs.updates) != 1 {
		t.Fatalf("esperava 1 mensagem repassada à tela ativa, obteve %d", len(fs.updates))
	}
}

func TestRoot_WindowSizeMsg_BroadcastsToActiveAndConfirm(t *testing.T) {
	m, fs := newTestRoot()
	m.confirm = newConfirmModel("t", "b", false, nil)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(rootModel)

	if len(fs.updates) != 1 {
		t.Fatalf("esperava WindowSizeMsg repassado pra tela ativa, obteve %d msgs", len(fs.updates))
	}
	if m.confirm.width != 80 || m.confirm.height != 24 {
		t.Fatalf("esperava confirm com width/height atualizados, obteve %dx%d", m.confirm.width, m.confirm.height)
	}
}

func TestRoot_AsyncResult_DiscardsStaleReqID(t *testing.T) {
	m, fs := newTestRoot()

	// Dispara duas operações em sequência, simulando o usuário navegando
	// antes da primeira resposta voltar.
	next, cmd1 := m.Update(startAsyncMsg{run: func() tea.Msg { return "primeira" }})
	m = next.(rootModel)
	if cmd1 == nil {
		t.Fatal("esperava tea.Cmd da primeira operação")
	}
	if m.inFlight != 1 {
		t.Fatalf("esperava inFlight==1, obteve %d", m.inFlight)
	}

	next, cmd2 := m.Update(startAsyncMsg{run: func() tea.Msg { return "segunda" }})
	m = next.(rootModel)
	if m.inFlight != 2 {
		t.Fatalf("esperava inFlight==2, obteve %d", m.inFlight)
	}

	// A resposta da primeira chega por último (fora de ordem) — deve ser
	// descartada, já que seu reqID não é mais o mais recente.
	result1, ok := findAsyncResult(resolveMsgs(cmd1))
	if !ok {
		t.Fatal("esperava uma asyncResultMsg entre as mensagens de cmd1")
	}
	next, _ = m.Update(result1)
	m = next.(rootModel)
	if len(fs.updates) != 0 {
		t.Fatalf("resposta obsoleta não deveria ter sido repassada à tela ativa: %v", fs.updates)
	}
	if m.inFlight != 1 {
		t.Fatalf("esperava inFlight==1 após descartar a primeira resposta, obteve %d", m.inFlight)
	}

	result2, ok := findAsyncResult(resolveMsgs(cmd2))
	if !ok {
		t.Fatal("esperava uma asyncResultMsg entre as mensagens de cmd2")
	}
	next, _ = m.Update(result2)
	m = next.(rootModel)
	if len(fs.updates) != 1 {
		t.Fatalf("resposta mais recente deveria ter sido repassada à tela ativa, obteve %d msgs", len(fs.updates))
	}
	if m.inFlight != 0 {
		t.Fatalf("esperava inFlight==0, obteve %d", m.inFlight)
	}
}

func TestRoot_PushPopScreen(t *testing.T) {
	m, fs := newTestRoot()
	fs2 := &fakeScreen{}

	next, _ := m.Update(pushScreenMsg{screen: fs2})
	m = next.(rootModel)
	if m.active != screen(fs2) {
		t.Fatal("esperava fs2 como tela ativa após push")
	}
	if len(m.stack) != 1 || m.stack[0] != screen(fs) {
		t.Fatal("esperava fs empilhado")
	}

	next, _ = m.Update(popScreenMsg{})
	m = next.(rootModel)
	if m.active != screen(fs) {
		t.Fatal("esperava fs de volta como tela ativa após pop")
	}
	if len(m.stack) != 0 {
		t.Fatal("esperava pilha vazia após pop")
	}
}

// TestRoot_PushScreen_ForwardsKnownWindowSize é um teste de regressão:
// tea.WindowSizeMsg só chega do runtime uma vez no início (e a cada resize
// real do terminal) — sem repassar o tamanho já conhecido pra toda tela
// recém-empilhada, ela nasce com width/height zerados e list.Model
// renderiza sem título nem itens (bug relatado pelo usuário).
func TestRoot_PushScreen_ForwardsKnownWindowSize(t *testing.T) {
	m, _ := newTestRoot()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(rootModel)

	fs2 := &fakeScreen{}
	m.Update(pushScreenMsg{screen: fs2})

	if len(fs2.updates) != 1 {
		t.Fatalf("esperava 1 mensagem entregue à tela recém-empilhada, obteve %d", len(fs2.updates))
	}
	sizeMsg, ok := fs2.updates[0].(tea.WindowSizeMsg)
	if !ok || sizeMsg.Width != 80 || sizeMsg.Height != 24 {
		t.Fatalf("esperava WindowSizeMsg{80,24} repassado à tela nova, obteve %+v", fs2.updates[0])
	}
}

// TestRoot_PopScreen_ForwardsKnownWindowSize cobre o mesmo bug ao voltar
// (pop) — a tela que reassume o foco também precisa do tamanho atual, não
// só a que acabou de ser empilhada.
func TestRoot_PopScreen_ForwardsKnownWindowSize(t *testing.T) {
	m, fs := newTestRoot()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(rootModel)

	fs2 := &fakeScreen{}
	next, _ = m.Update(pushScreenMsg{screen: fs2})
	m = next.(rootModel)
	fs.updates = nil // limpa a entrega inicial pra focar no que importa: o pop

	next, _ = m.Update(popScreenMsg{})
	m = next.(rootModel)
	if m.active != screen(fs) {
		t.Fatal("esperava fs de volta ativo após pop")
	}

	found := false
	for _, msg := range fs.updates {
		if sz, ok := msg.(tea.WindowSizeMsg); ok && sz.Width == 100 && sz.Height == 30 {
			found = true
		}
	}
	if !found {
		t.Fatalf("esperava WindowSizeMsg{100,30} repassado à tela ao voltar (pop), updates: %v", fs.updates)
	}
}

func TestRoot_SpinnerTick_StopsWhenNoLongerInFlight(t *testing.T) {
	m, _ := newTestRoot()
	_, cmd := m.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Fatal("sem operação em voo, spinner.TickMsg não deveria reagendar outro tick")
	}
}
