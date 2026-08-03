package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// drive executa cmd (e qualquer tea.Cmd que ele produza, recursivamente,
// inclusive desdobrando tea.BatchMsg) através de m.Update, simulando o
// loop do runtime real do Bubble Tea — usado nos testes de integração
// deste arquivo, que exercitam o rootModel de ponta a ponta (navegação +
// async + reload) em vez de uma tela isolada.
func drive(t *testing.T, m rootModel, cmd tea.Cmd) rootModel {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for i := 0; len(queue) > 0 && i < 200; i++ {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		next, newCmd := m.Update(msg)
		var ok2 bool
		m, ok2 = next.(rootModel)
		if !ok2 {
			t.Fatalf("rootModel.Update retornou tipo inesperado %T", next)
		}
		if newCmd != nil {
			queue = append(queue, newCmd)
		}
	}
	return m
}

// TestIntegration_AddHost_ThroughRoot valida a fiação completa
// menu -> hosts_list -> host_form -> AddHost -> pop -> reload automático,
// exercitando o rootModel de verdade (não uma tela isolada) — é o único
// ponto do sistema que os testes por tela não cobrem sozinhos.
func TestIntegration_AddHost_ThroughRoot(t *testing.T) {
	configSvc, keySvc, backupSvc := newTestServices(t)
	m := newRootModel(keySvc, configSvc, backupSvc)
	m = drive(t, m, m.Init())

	if _, ok := m.active.(*menuModel); !ok {
		t.Fatalf("esperava menu principal ativo, obteve %T", m.active)
	}

	// Menu: cursor 0 = "Hosts".
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(rootModel)
	m = drive(t, m, cmd)

	hostsList, ok := m.active.(*hostsListModel)
	if !ok {
		t.Fatalf("esperava *hostsListModel ativo, obteve %T", m.active)
	}
	if len(hostsList.list.Items()) != 0 {
		t.Fatalf("esperava lista de hosts vazia inicialmente, obteve %d itens", len(hostsList.list.Items()))
	}

	// 'n' novo host.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(rootModel)
	m = drive(t, m, cmd)

	form, ok := m.active.(*hostFormModel)
	if !ok {
		t.Fatalf("esperava *hostFormModel ativo, obteve %T", m.active)
	}
	form.patterns.SetValue("bastion")
	form.hostName.SetValue("1.2.3.4")

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(rootModel)
	m = drive(t, m, cmd)

	hostsList, ok = m.active.(*hostsListModel)
	if !ok {
		t.Fatalf("esperava voltar pra *hostsListModel após AddHost, obteve %T", m.active)
	}
	if len(hostsList.list.Items()) != 1 {
		t.Fatalf("esperava 1 host na lista após add + reload automático, obteve %d", len(hostsList.list.Items()))
	}
	item, ok := hostsList.list.Items()[0].(hostItem)
	if !ok || item.host.HostName != "1.2.3.4" {
		t.Fatalf("esperava host com HostName 1.2.3.4, obteve %+v", item.host)
	}
}

// TestIntegration_GenerateKey_ThroughRoot valida a fiação completa
// menu -> keys_list -> key_form -> GenerateKey -> pop -> reload automático.
func TestIntegration_GenerateKey_ThroughRoot(t *testing.T) {
	configSvc, keySvc, backupSvc := newTestServices(t)
	m := newRootModel(keySvc, configSvc, backupSvc)
	m = drive(t, m, m.Init())

	// Menu: cursor 1 = "Chaves".
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(rootModel)
	m = drive(t, m, cmd)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(rootModel)
	m = drive(t, m, cmd)

	if _, ok := m.active.(*keysListModel); !ok {
		t.Fatalf("esperava *keysListModel ativo, obteve %T", m.active)
	}

	// 'g' gerar chave.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = next.(rootModel)
	m = drive(t, m, cmd)

	form, ok := m.active.(*keyFormModel)
	if !ok {
		t.Fatalf("esperava *keyFormModel ativo, obteve %T", m.active)
	}
	form.label.SetValue("chave via root")

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(rootModel)
	m = drive(t, m, cmd)

	keysList, ok := m.active.(*keysListModel)
	if !ok {
		t.Fatalf("esperava voltar pra *keysListModel após GenerateKey, obteve %T", m.active)
	}
	if len(keysList.list.Items()) != 1 {
		t.Fatalf("esperava 1 chave na lista após gerar + reload automático, obteve %d", len(keysList.list.Items()))
	}
	item, ok := keysList.list.Items()[0].(keyItem)
	if !ok || item.key.Metadata.Label != "chave via root" {
		t.Fatalf("esperava chave com label 'chave via root', obteve %+v", item.key.Metadata)
	}
}
