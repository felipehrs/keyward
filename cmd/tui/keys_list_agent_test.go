package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/felipehrs/keyward/core"
)

// startFakeAgent sobe um ssh-agent em memória (net.Pipe + agent.ServeAgent)
// com as chaves informadas já carregadas, servindo uma conexão nova por
// dial — necessário porque mais de uma operação da tela pode sondar o
// agente na mesma interação (ex. RegisterAgentKey: valida a identidade e
// depois recarrega via GetKey), e um net.Pipe fechado não é reutilizável.
// Espelha core/keys_agent_test.go:startFakeAgentMulti — duplicado aqui
// porque test helpers não exportam entre pacotes.
func startFakeAgent(t *testing.T, keys ...agent.AddedKey) func(timeout time.Duration) (net.Conn, error) {
	t.Helper()
	keyring := agent.NewKeyring()
	for _, k := range keys {
		if err := keyring.Add(k); err != nil {
			t.Fatalf("keyring.Add: %v", err)
		}
	}
	return func(timeout time.Duration) (net.Conn, error) {
		clientConn, serverConn := net.Pipe()
		t.Cleanup(func() { _ = clientConn.Close() })
		go func() { _ = agent.ServeAgent(keyring, serverConn) }()
		return clientConn, nil
	}
}

func genEd25519Identity(t *testing.T) (agent.AddedKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("ssh.NewPublicKey: %v", err)
	}
	return agent.AddedKey{PrivateKey: priv, Comment: "teste"}, ssh.FingerprintSHA256(sshPub)
}

func TestBuildKeyItems_SeparatesAgentGroupWithHeader(t *testing.T) {
	fileKey := core.Key{Source: core.KeySourceFile, Metadata: core.KeyMetadata{Label: "arquivo"}}
	agentKey := core.Key{Source: core.KeySourceAgent, Metadata: core.KeyMetadata{Label: "agente"}}

	items := buildKeyItems([]core.Key{fileKey, agentKey})
	if len(items) != 3 {
		t.Fatalf("esperava 3 items (1 arquivo + header + 1 agente), obteve %d", len(items))
	}
	if _, ok := items[0].(keyItem); !ok {
		t.Fatalf("item 0 deveria ser keyItem de arquivo, obteve %T", items[0])
	}
	if _, ok := items[1].(sectionHeaderItem); !ok {
		t.Fatalf("item 1 deveria ser sectionHeaderItem, obteve %T", items[1])
	}
	if _, ok := items[2].(keyItem); !ok {
		t.Fatalf("item 2 deveria ser keyItem de agente, obteve %T", items[2])
	}
}

func TestBuildKeyItems_NoAgentKeys_NoHeader(t *testing.T) {
	items := buildKeyItems([]core.Key{{Source: core.KeySourceFile}})
	for _, item := range items {
		if _, ok := item.(sectionHeaderItem); ok {
			t.Fatal("não deveria inserir header sem nenhuma chave de agente")
		}
	}
}

func TestKeyItem_Title_AgentBadgeAndOfflineState(t *testing.T) {
	named := keyItem{key: core.Key{Source: core.KeySourceAgent, AgentName: "1password"}}
	if got := named.Title(); got == "-" {
		t.Fatalf("esperava badge com AgentName no título, obteve %q", got)
	}

	generic := keyItem{key: core.Key{Source: core.KeySourceAgent}}
	if got := generic.Title(); got == "-" {
		t.Fatalf("esperava badge genérico 'Agente SSH' no título, obteve %q", got)
	}

	offline := keyItem{key: core.Key{Source: core.KeySourceAgent, Status: core.KeyStatusAgentOffline}}
	if got := offline.Title(); got == "-" {
		t.Fatalf("esperava sufixo de estado offline no título, obteve %q", got)
	}
}

func TestKeysListModel_Init_DetectsAgentAbsence(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	fileSvc := keySvc.(*core.FileKeyService)
	fileSvc.AgentDial = func(timeout time.Duration) (net.Conn, error) {
		return nil, net.ErrClosed
	}

	m := newKeysListModel(keySvc)
	results := resolveMsgs(m.Init())
	var got bool
	for _, msg := range results {
		if detected, ok := msg.(agentDetectedMsg); ok {
			got = true
			if detected.info.Detected {
				t.Fatal("esperava Detected == false sem agente acessível")
			}
		}
	}
	if !got {
		t.Fatal("esperava agentDetectedMsg entre os resultados de Init")
	}

	m.Update(agentDetectedMsg{})
	view := m.View()
	if view == "" {
		t.Fatal("View não deveria ser vazia")
	}
}

func TestKeysListModel_RegisterOrphan_AgentIdentity_PushesAgentRegisterForm(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	fileSvc := keySvc.(*core.FileKeyService)
	added, fingerprint := genEd25519Identity(t)
	fileSvc.AgentDial = startFakeAgent(t, added)

	m := newKeysListModel(keySvc)
	loaded := initKeysListModel(t, m)
	m.Update(loaded)

	// Sem chaves de arquivo, o item 0 da lista é o sectionHeaderItem que
	// antecede o grupo de agente (ver buildKeyItems) — o item selecionável
	// de verdade é o seguinte.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("esperava pushScreenMsg")
	}
	push, ok := cmd().(pushScreenMsg)
	if !ok {
		t.Fatalf("esperava pushScreenMsg, obteve %T", push)
	}
	form, ok := push.screen.(*agentKeyRegisterFormModel)
	if !ok {
		t.Fatalf("esperava *agentKeyRegisterFormModel, obteve %T", push.screen)
	}
	if form.fingerprint != fingerprint {
		t.Fatalf("esperava fingerprint %q, obteve %q", fingerprint, form.fingerprint)
	}
}

func TestAgentKeyRegisterFormModel_Submit_RegistersIdentity(t *testing.T) {
	_, keySvc, _ := newTestServices(t)
	fileSvc := keySvc.(*core.FileKeyService)
	added, fingerprint := genEd25519Identity(t)
	fileSvc.AgentDial = startFakeAgent(t, added)

	m := newAgentKeyRegisterFormModel(keySvc, fingerprint)
	m.label.SetValue("chave anotada")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("esperava tea.Cmd de submissão")
	}
	results := resolveMsgs(startAsyncCmdFor(t, cmd))
	var registered agentKeyRegisteredMsg
	var found bool
	for _, msg := range results {
		if r, ok := msg.(agentKeyRegisteredMsg); ok {
			registered, found = r, true
		}
	}
	if !found {
		t.Fatal("esperava agentKeyRegisteredMsg")
	}
	if registered.err != nil {
		t.Fatalf("RegisterAgentKey: %v", registered.err)
	}
	if registered.key.Metadata.Label != "chave anotada" {
		t.Fatalf("esperava label 'chave anotada', obteve %q", registered.key.Metadata.Label)
	}

	_, popCmd := m.Update(registered)
	if popCmd == nil {
		t.Fatal("esperava popScreenMsg após sucesso")
	}
	if _, ok := popCmd().(popScreenMsg); !ok {
		t.Fatal("esperava popScreenMsg após RegisterAgentKey bem-sucedido")
	}
}
