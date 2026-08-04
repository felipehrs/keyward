package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// startFakeAgent sobe um ssh-agent em memória (via net.Pipe, servido por
// agent.ServeAgent) com as chaves informadas já carregadas, e devolve um
// AgentDial que conecta a ele. O agente é encerrado quando o teste termina.
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

	go func() {
		_ = agent.ServeAgent(keyring, serverConn)
	}()

	return func(timeout time.Duration) (net.Conn, error) {
		return clientConn, nil
	}
}

// startFakeAgentMulti é como startFakeAgent, mas serve uma conexão nova
// (net.Pipe) por dial, em vez de sempre a mesma — necessário para testes que
// disparam mais de uma operação contra o agente na mesma chamada (ex.
// RegisterAgentKey, que sonda o agente e depois recarrega via GetKey), já
// que um net.Pipe fechado não pode ser reaproveitado por um segundo dial.
func startFakeAgentMulti(t *testing.T, keys ...agent.AddedKey) func(timeout time.Duration) (net.Conn, error) {
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
		go func() {
			_ = agent.ServeAgent(keyring, serverConn)
		}()
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
	fingerprint := ssh.FingerprintSHA256(sshPub)
	return agent.AddedKey{PrivateKey: priv, Comment: "teste"}, fingerprint
}

func TestFileKeyService_ListKeys_AgentPresentWithKey(t *testing.T) {
	svc := newTestKeyService(t)
	added, fingerprint := genEd25519Identity(t)
	svc.AgentDial = startFakeAgent(t, added)

	keys, err := svc.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("esperava 1 chave, obteve %d: %+v", len(keys), keys)
	}
	k := keys[0]
	if k.Source != KeySourceAgent {
		t.Errorf("Source = %v, esperado KeySourceAgent", k.Source)
	}
	if k.Status != KeyStatusUnregistered {
		t.Errorf("Status = %v, esperado KeyStatusUnregistered (sem metadata ainda)", k.Status)
	}
	// Fingerprint é o único campo populado para uma identidade ainda sem
	// registro — serve de handle estável pra RegisterAgentKey (equivalente
	// ao PrivateKeyPath do caso file-based).
	if k.Metadata.Fingerprint != fingerprint {
		t.Errorf("Metadata.Fingerprint = %q, esperado %q", k.Metadata.Fingerprint, fingerprint)
	}
	if k.Metadata.Label != "" || k.Metadata.ID != "" || !k.Metadata.CreatedAt.IsZero() {
		t.Errorf("esperava demais campos de Metadata zerados para identidade não registrada, obteve %+v", k.Metadata)
	}
	if k.PrivateKeyPath != "" || k.PublicKeyPath != "" {
		t.Errorf("chave de agente não deveria ter PrivateKeyPath/PublicKeyPath, obteve %q/%q", k.PrivateKeyPath, k.PublicKeyPath)
	}
	_ = fingerprint
}

func TestFileKeyService_RegisterAgentKey_AdoptsOfferedIdentity(t *testing.T) {
	svc := newTestKeyService(t)
	added, fingerprint := genEd25519Identity(t)
	svc.AgentDial = startFakeAgentMulti(t, added)

	label := "minha chave 1password"
	registered, err := svc.RegisterAgentKey(fingerprint, KeyMetadataPatch{Label: &label})
	if err != nil {
		t.Fatalf("RegisterAgentKey: %v", err)
	}
	if registered.Status != KeyStatusOK {
		t.Errorf("Status = %v, esperado KeyStatusOK após registrar", registered.Status)
	}
	if registered.Metadata.Label != label {
		t.Errorf("Label = %q, esperado %q", registered.Metadata.Label, label)
	}
	if registered.Source != KeySourceAgent {
		t.Errorf("Source = %v, esperado KeySourceAgent", registered.Source)
	}

	keys, err := svc.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].Status != KeyStatusOK {
		t.Fatalf("esperava 1 chave KeyStatusOK após registro, obteve %+v", keys)
	}
}

func TestFileKeyService_RegisterAgentKey_UnknownFingerprintReturnsError(t *testing.T) {
	svc := newTestKeyService(t)
	svc.AgentDial = startFakeAgentMulti(t)

	if _, err := svc.RegisterAgentKey("SHA256:nao-oferecida", KeyMetadataPatch{}); err == nil {
		t.Fatal("esperava erro ao registrar fingerprint não oferecido pelo agente")
	}
}

func TestFileKeyService_RegisterAgentKey_AlreadyRegisteredReturnsError(t *testing.T) {
	svc := newTestKeyService(t)
	added, fingerprint := genEd25519Identity(t)
	svc.AgentDial = startFakeAgentMulti(t, added)

	if _, err := svc.RegisterAgentKey(fingerprint, KeyMetadataPatch{}); err != nil {
		t.Fatalf("RegisterAgentKey (1a vez): %v", err)
	}
	if _, err := svc.RegisterAgentKey(fingerprint, KeyMetadataPatch{}); err == nil {
		t.Fatal("esperava erro ao registrar fingerprint já registrado")
	}
}

func TestFileKeyService_ListKeys_NoAgentConfigured(t *testing.T) {
	svc := newTestKeyService(t)
	svc.AgentDial = func(timeout time.Duration) (net.Conn, error) {
		return nil, errors.New("SSH_AUTH_SOCK não definido")
	}

	if _, err := svc.GenerateKey(GenerateKeyOptions{}); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	keys, err := svc.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("esperava 1 chave (só de arquivo), obteve %d: %+v", len(keys), keys)
	}
	if keys[0].Source != KeySourceFile {
		t.Errorf("Source = %v, esperado KeySourceFile", keys[0].Source)
	}
}

func TestFileKeyService_ListKeys_AgentUnreachable_ExistingAgentMetadataGoesOffline(t *testing.T) {
	svc := newTestKeyService(t)
	svc.AgentDial = func(timeout time.Duration) (net.Conn, error) {
		return nil, errors.New("timeout")
	}

	st, err := svc.store()
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	mf, err := st.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	mf.Keys = append(mf.Keys, KeyMetadata{
		ID:          "agent-key-1",
		Fingerprint: "SHA256:fake-agent-fingerprint",
		Source:      KeySourceAgent,
		AgentName:   "1password",
	})
	if err := st.save(mf); err != nil {
		t.Fatalf("save: %v", err)
	}

	keys, err := svc.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("esperava 1 chave, obteve %d: %+v", len(keys), keys)
	}
	if keys[0].Status != KeyStatusAgentOffline {
		t.Errorf("Status = %v, esperado KeyStatusAgentOffline", keys[0].Status)
	}
	if keys[0].AgentName != "1password" {
		t.Errorf("AgentName = %q, esperado %q", keys[0].AgentName, "1password")
	}
}

func TestFileKeyService_DetectAgent_TrueWithZeroIdentities(t *testing.T) {
	svc := newTestKeyService(t)
	svc.AgentDial = startFakeAgent(t)

	info, err := svc.DetectAgent()
	if err != nil {
		t.Fatalf("DetectAgent: %v", err)
	}
	if !info.Detected {
		t.Error("Detected = false, esperado true mesmo sem identidades")
	}
}

func TestFileKeyService_DetectAgent_FalseWhenUnreachable(t *testing.T) {
	svc := newTestKeyService(t)
	svc.AgentDial = func(timeout time.Duration) (net.Conn, error) {
		return nil, errors.New("sem agente")
	}

	info, err := svc.DetectAgent()
	if err != nil {
		t.Fatalf("DetectAgent: %v", err)
	}
	if info.Detected {
		t.Error("Detected = true, esperado false")
	}
	if info.SocketPath != "" || info.Name != "" {
		t.Errorf("esperava SocketPath/Name vazios, obteve %+v", info)
	}
}

func TestFileKeyService_ListKeys_SlowAgentDialDoesNotHangPastTimeout(t *testing.T) {
	svc := newTestKeyService(t)
	svc.AgentDial = func(timeout time.Duration) (net.Conn, error) {
		<-make(chan struct{}) // nunca retorna
		return nil, nil
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.ListKeys()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ListKeys: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ListKeys não retornou dentro do prazo — AgentDial que nunca retorna travou a chamada")
	}
}

func TestFileKeyService_DetectAgent_SlowDialDoesNotHangPastTimeout(t *testing.T) {
	svc := newTestKeyService(t)
	svc.AgentDial = func(timeout time.Duration) (net.Conn, error) {
		<-make(chan struct{}) // nunca retorna
		return nil, nil
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.DetectAgent()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("DetectAgent: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("DetectAgent não retornou dentro do prazo — AgentDial que nunca retorna travou a chamada")
	}
}

func TestMetadataStore_LoadsPreExistingRecordWithoutSourceAsKeySourceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.json")
	legacyJSON := `{
		"version": 1,
		"keys": [
			{
				"id": "legacy-1",
				"fingerprint": "SHA256:legacy",
				"keyPath": "/home/user/.ssh/id_ed25519",
				"algorithm": "ed25519",
				"createdAt": "2024-01-01T00:00:00Z"
			}
		],
		"settings": {"alertThresholdDays": 30, "defaultAlgorithm": "ed25519"}
	}`
	writeFile(t, path, legacyJSON)

	st := newMetadataStore(path)
	mf, err := st.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(mf.Keys) != 1 {
		t.Fatalf("esperava 1 registro, obteve %d", len(mf.Keys))
	}
	if mf.Keys[0].Source != KeySourceFile {
		t.Errorf("Source = %v, esperado KeySourceFile (zero value) para registro sem campo source", mf.Keys[0].Source)
	}
}
