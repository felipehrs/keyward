package main

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

func TestHostsListModel_Init_LoadsHosts(t *testing.T) {
	configSvc, _, _ := newTestServices(t)
	if err := configSvc.AddHost("", core.HostSpec{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	m := newHostsListModel(configSvc)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("esperava um tea.Cmd de Init")
	}

	results := resolveMsgs(startAsyncCmdFor(t, cmd))
	loaded, ok := findHostsLoaded(results)
	if !ok {
		t.Fatal("esperava hostsLoadedMsg")
	}
	if loaded.err != nil {
		t.Fatalf("hostsLoadedMsg.err: %v", loaded.err)
	}
	if len(loaded.hosts) != 1 || loaded.hosts[0].Patterns[0] != "bastion" {
		t.Fatalf("esperava host 'bastion', obteve %+v", loaded.hosts)
	}

	m.Update(loaded) // m.list.SetItems pode retornar cmd nil; só o efeito colateral importa aqui
	if len(m.list.Items()) != 1 {
		t.Fatalf("esperava 1 item na lista, obteve %d", len(m.list.Items()))
	}
}

func TestHostsListModel_LoadError_EmitsErrMsg(t *testing.T) {
	m := newHostsListModel(&brokenConfigService{})
	results := resolveMsgs(startAsyncCmdFor(t, m.Init()))
	loaded, ok := findHostsLoaded(results)
	if !ok {
		t.Fatal("esperava hostsLoadedMsg")
	}
	if loaded.err == nil {
		t.Fatal("esperava erro")
	}

	_, cmd := m.Update(loaded)
	if cmd == nil {
		t.Fatal("esperava um tea.Cmd de erro")
	}
	msg := cmd()
	if _, ok := msg.(errMsg); !ok {
		t.Fatalf("esperava errMsg, obteve %T", msg)
	}
}

func TestHostsListModel_Esc_PopsScreen(t *testing.T) {
	configSvc, _, _ := newTestServices(t)
	m := newHostsListModel(configSvc)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esperava popScreenMsg")
	}
	if _, ok := cmd().(popScreenMsg); !ok {
		t.Fatal("esperava popScreenMsg")
	}
}

func TestHostsListModel_New_PushesAddForm(t *testing.T) {
	configSvc, _, _ := newTestServices(t)
	m := newHostsListModel(configSvc)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil {
		t.Fatal("esperava pushScreenMsg")
	}
	push, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", push)
	}
	form, ok := push.screen.(*hostFormModel)
	if !ok {
		t.Fatalf("esperava *hostFormModel, obteve %T", push.screen)
	}
	if form.editing {
		t.Fatal("formulário aberto por 'n' não deveria estar em modo de edição")
	}
}

func TestHostsListModel_Enter_PushesEditFormPrefilled(t *testing.T) {
	configSvc, _, _ := newTestServices(t)
	if err := configSvc.AddHost("", core.HostSpec{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	m := newHostsListModel(configSvc)
	loaded, _ := findHostsLoaded(resolveMsgs(startAsyncCmdFor(t, m.Init())))
	m.Update(loaded)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava pushScreenMsg")
	}
	push, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", push)
	}
	form, ok := push.screen.(*hostFormModel)
	if !ok {
		t.Fatalf("esperava *hostFormModel, obteve %T", push.screen)
	}
	if !form.editing || form.patterns.Value() != "bastion" {
		t.Fatalf("esperava formulário pré-preenchido em modo de edição, obteve %+v", form)
	}
}

func TestHostsListModel_Delete_RequestsConfirmation(t *testing.T) {
	configSvc, _, _ := newTestServices(t)
	if err := configSvc.AddHost("", core.HostSpec{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	m := newHostsListModel(configSvc)
	loaded, _ := findHostsLoaded(resolveMsgs(startAsyncCmdFor(t, m.Init())))
	m.Update(loaded)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("esperava requestConfirmMsg")
	}
	confirmMsg, ok := cmd().(requestConfirmMsg)
	if !ok {
		t.Fatalf("remover host deveria exigir confirmação, obteve %T", confirmMsg)
	}
	if !confirmMsg.danger {
		t.Fatal("remover host deveria ser danger=true")
	}

	result := resolveMsgs(startAsyncCmdFor(t, confirmMsg.onConfirm))
	mutated, ok := findHostMutated(result)
	if !ok {
		t.Fatal("esperava hostMutatedMsg após confirmar remoção")
	}
	if mutated.err != nil {
		t.Fatalf("RemoveHost: %v", mutated.err)
	}

	hosts, err := configSvc.ListHosts()
	if err != nil || len(hosts) != 0 {
		t.Fatalf("esperava lista vazia após remoção, obteve %v %+v", err, hosts)
	}
}

// startAsyncCmdFor extrai o tea.Cmd real de dentro de um startAsyncMsg,
// simulando o que o root faz ao receber essa mensagem (ver model.go). Usado
// em testes de tela isolada, sem precisar instanciar o rootModel inteiro.
func startAsyncCmdFor(t *testing.T, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	msg := cmd()
	start, ok := msg.(startAsyncMsg)
	if !ok {
		t.Fatalf("esperava startAsyncMsg, obteve %T", msg)
	}
	return func() tea.Msg { return start.run() }
}

func findHostsLoaded(msgs []tea.Msg) (hostsLoadedMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(hostsLoadedMsg); ok {
			return m, true
		}
	}
	return hostsLoadedMsg{}, false
}

// brokenConfigService implementa core.ConfigService retornando erro em
// ListHosts, para testar o caminho de erro sem depender de um path de
// disco inválido específico de SO.
type brokenConfigService struct{}

func (brokenConfigService) ListHosts() ([]core.Host, error) {
	return nil, errBroken
}
func (brokenConfigService) AddHost(string, core.HostSpec) error               { return errBroken }
func (brokenConfigService) ReplaceHost(string, []string, core.HostSpec) error { return errBroken }
func (brokenConfigService) RemoveHost(string, []string) error                 { return errBroken }

var errBroken = errors.New("erro simulado")
