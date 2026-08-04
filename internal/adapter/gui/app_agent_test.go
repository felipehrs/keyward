package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh/agent"

	"github.com/felipehrs/keyward/core"
)

func genEd25519Key(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	return priv
}

// startFakeAgent sobe um ssh-agent em memória (net.Pipe + agent.ServeAgent)
// com as chaves informadas já carregadas — mesma técnica de
// core/keys_agent_test.go, reaplicada aqui pra testar a tradução App/DTO
// sem depender de um agente real no ambiente de CI.
func startFakeAgent(t *testing.T, keys ...agent.AddedKey) func(timeout time.Duration) (net.Conn, error) {
	t.Helper()

	keyring := agent.NewKeyring()
	for _, k := range keys {
		if err := keyring.Add(k); err != nil {
			t.Fatalf("keyring.Add: %v", err)
		}
	}

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close() })

	go func() { _ = agent.ServeAgent(keyring, serverConn) }()

	return func(timeout time.Duration) (net.Conn, error) {
		return clientConn, nil
	}
}

func TestApp_DetectAgent_NotDetected(t *testing.T) {
	a := newTestApp(t)
	svc := a.keySvc.(*core.FileKeyService)
	svc.AgentDial = func(timeout time.Duration) (net.Conn, error) {
		return nil, errors.New("sem agente")
	}

	info, err := a.DetectAgent()
	if err != nil {
		t.Fatalf("DetectAgent: %v", err)
	}
	if info.Detected {
		t.Error("Detected = true, esperado false")
	}
}

func TestApp_DetectAgent_Detected(t *testing.T) {
	a := newTestApp(t)
	svc := a.keySvc.(*core.FileKeyService)
	svc.AgentDial = startFakeAgent(t)

	info, err := a.DetectAgent()
	if err != nil {
		t.Fatalf("DetectAgent: %v", err)
	}
	if !info.Detected {
		t.Error("Detected = false, esperado true mesmo sem identidades")
	}
}

func TestApp_HostKey_JoinsPatternsWithNUL(t *testing.T) {
	a := newTestApp(t)
	got := a.HostKey([]string{"prod", "*.example.com"})
	want := "prod\x00*.example.com"
	if got != want {
		t.Errorf("HostKey = %q, esperado %q", got, want)
	}
}

func TestApp_LinkHostKey_ThenListHostLinks(t *testing.T) {
	a := newTestApp(t)
	if err := a.LinkHostKey(LinkHostKeyInput{HostKey: "prod-server", AgentKeyFingerprint: "SHA256:abc", Notes: "nota"}); err != nil {
		t.Fatalf("LinkHostKey: %v", err)
	}

	links, err := a.ListHostLinks()
	if err != nil {
		t.Fatalf("ListHostLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("esperava 1 vínculo, obteve %d", len(links))
	}
	if links[0].HostKey != "prod-server" || links[0].AgentKeyFingerprint != "SHA256:abc" || links[0].Notes != "nota" {
		t.Errorf("vínculo inesperado: %+v", links[0])
	}
	// Sem host algum cadastrado em ConfigService — o vínculo é órfão.
	if !links[0].Orphan {
		t.Error("esperava Orphan = true (nenhum host cadastrado corresponde a HostKey)")
	}
}

func TestApp_ListHostLinks_NotOrphanWhenHostExists(t *testing.T) {
	a := newTestApp(t)
	if err := a.configSvc.AddHost("", core.HostSpec{Patterns: []string{"prod-server"}}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	hostKey := a.HostKey([]string{"prod-server"})
	if err := a.LinkHostKey(LinkHostKeyInput{HostKey: hostKey, AgentKeyFingerprint: "SHA256:abc"}); err != nil {
		t.Fatalf("LinkHostKey: %v", err)
	}

	links, err := a.ListHostLinks()
	if err != nil {
		t.Fatalf("ListHostLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("esperava 1 vínculo, obteve %d", len(links))
	}
	if links[0].Orphan {
		t.Error("esperava Orphan = false (host existe em ConfigService.ListHosts())")
	}
}

func TestApp_UnlinkHostKey_RemovesLink(t *testing.T) {
	a := newTestApp(t)
	if err := a.LinkHostKey(LinkHostKeyInput{HostKey: "prod-server", AgentKeyFingerprint: "SHA256:abc"}); err != nil {
		t.Fatalf("LinkHostKey: %v", err)
	}
	if err := a.UnlinkHostKey(UnlinkHostKeyInput{HostKey: "prod-server", AgentKeyFingerprint: "SHA256:abc"}); err != nil {
		t.Fatalf("UnlinkHostKey: %v", err)
	}

	links, err := a.ListHostLinks()
	if err != nil {
		t.Fatalf("ListHostLinks: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("esperava 0 vínculos após unlink, obteve %d", len(links))
	}
}

func TestApp_UnlinkHostKey_NotFound_ReturnsError(t *testing.T) {
	a := newTestApp(t)
	err := a.UnlinkHostKey(UnlinkHostKeyInput{HostKey: "nao-existe", AgentKeyFingerprint: "SHA256:zzz"})
	if err == nil {
		t.Fatal("esperava erro")
	}
}

func TestApp_ListKeys_AgentSourcedKey_MapsSourceAndAgentName(t *testing.T) {
	a := newTestApp(t)
	svc := a.keySvc.(*core.FileKeyService)
	svc.AgentDial = startFakeAgent(t, agent.AddedKey{PrivateKey: genEd25519Key(t), Comment: "chave-agente"})

	keys, err := a.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("esperava 1 chave, obteve %d: %+v", len(keys), keys)
	}
	k := keys[0]
	if k.Source != KeySourceAgent {
		t.Errorf("Source = %q, esperado %q", k.Source, KeySourceAgent)
	}
	if k.PrivateKeyPath != "" || k.PublicKeyPath != "" {
		t.Errorf("chave de agente não deveria ter paths de arquivo, obteve %q/%q", k.PrivateKeyPath, k.PublicKeyPath)
	}
}

func TestApp_ListKeys_FileSourcedKey_MapsSourceFile(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.keySvc.GenerateKey(core.GenerateKeyOptions{}); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	keys, err := a.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("esperava 1 chave, obteve %d", len(keys))
	}
	if keys[0].Source != KeySourceFile {
		t.Errorf("Source = %q, esperado %q", keys[0].Source, KeySourceFile)
	}
	if keys[0].AgentName != "" {
		t.Errorf("AgentName = %q, esperado vazio pra chave de arquivo", keys[0].AgentName)
	}
}
