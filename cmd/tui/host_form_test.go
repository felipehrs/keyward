package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

func TestHostFormModel_Add_NoConfirmation_CallsAddHostDirectly(t *testing.T) {
	configSvc, _, _ := newTestServices(t)
	m := newHostFormModel(configSvc)
	m.patterns.SetValue("bastion, bastion.example.com")
	m.hostName.SetValue("1.2.3.4")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava tea.Cmd de submissão")
	}
	msg := cmd()
	// AddHost não é destrutivo: não deve passar por requestConfirmMsg.
	if _, ok := msg.(requestConfirmMsg); ok {
		t.Fatal("AddHost não deveria exigir confirmação")
	}
	start, ok := msg.(startAsyncMsg)
	if !ok {
		t.Fatalf("esperava startAsyncMsg, obteve %T", msg)
	}
	result := start.run().(hostMutatedMsg)
	if result.err != nil {
		t.Fatalf("AddHost: %v", result.err)
	}

	hosts, err := configSvc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 1 || hosts[0].HostName != "1.2.3.4" {
		t.Fatalf("esperava host com HostName 1.2.3.4, obteve %+v", hosts)
	}
}

func TestHostFormModel_Add_EmptyPatterns_BlocksSubmit(t *testing.T) {
	configSvc, _, _ := newTestServices(t)
	m := newHostFormModel(configSvc)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("não deveria submeter sem Patterns")
	}
	if m.validationErr == nil {
		t.Fatal("esperava validationErr definido")
	}
}

func TestHostFormModel_Edit_RequiresConfirmation(t *testing.T) {
	configSvc, _, _ := newTestServices(t)
	if err := configSvc.AddHost("", core.HostSpec{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	hosts, err := configSvc.ListHosts()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("ListHosts: %v %+v", err, hosts)
	}

	m := newHostEditFormModel(configSvc, hosts[0])
	m.hostName.SetValue("9.9.9.9")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava tea.Cmd de submissão")
	}
	msg := cmd()
	confirmMsg, ok := msg.(requestConfirmMsg)
	if !ok {
		t.Fatalf("ReplaceHost deveria exigir confirmação, obteve %T", msg)
	}
	if !confirmMsg.danger {
		t.Fatal("substituir host deveria ser danger=true")
	}

	// Confirma e verifica que a mutação realmente aconteceu.
	result := resolveMsgs(startAsyncCmdFor(t, confirmMsg.onConfirm))
	mutated, ok := findHostMutated(result)
	if !ok {
		t.Fatal("esperava hostMutatedMsg após confirmar")
	}
	if mutated.err != nil {
		t.Fatalf("ReplaceHost: %v", mutated.err)
	}

	hostsAfter, err := configSvc.ListHosts()
	if err != nil || len(hostsAfter) != 1 || hostsAfter[0].HostName != "9.9.9.9" {
		t.Fatalf("esperava HostName atualizado, obteve %v %+v", err, hostsAfter)
	}
}

func TestHostFormModel_Esc_PopsScreen(t *testing.T) {
	configSvc, _, _ := newTestServices(t)
	m := newHostFormModel(configSvc)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esperava popScreenMsg")
	}
	if _, ok := cmd().(popScreenMsg); !ok {
		t.Fatal("esperava popScreenMsg")
	}
}

func findHostMutated(msgs []tea.Msg) (hostMutatedMsg, bool) {
	for _, msg := range msgs {
		if m, ok := msg.(hostMutatedMsg); ok {
			return m, true
		}
	}
	return hostMutatedMsg{}, false
}
