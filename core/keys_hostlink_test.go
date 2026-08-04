package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestKeyService_LinkHostKey_ThenListHostLinks(t *testing.T) {
	svc := newTestKeyService(t)

	if err := svc.LinkHostKey("prod-server", "SHA256:abc", "vinculo inicial"); err != nil {
		t.Fatalf("LinkHostKey: %v", err)
	}

	links, err := svc.ListHostLinks()
	if err != nil {
		t.Fatalf("ListHostLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("esperava 1 vínculo, obteve %d", len(links))
	}
	if links[0].HostKey != "prod-server" || links[0].AgentKeyFingerprint != "SHA256:abc" || links[0].Notes != "vinculo inicial" {
		t.Errorf("vínculo inesperado: %+v", links[0])
	}
}

func TestKeyService_LinkHostKey_SamePairIsIdempotentAndUpdatesNotes(t *testing.T) {
	svc := newTestKeyService(t)

	if err := svc.LinkHostKey("prod-server", "SHA256:abc", "primeira nota"); err != nil {
		t.Fatalf("LinkHostKey: %v", err)
	}
	if err := svc.LinkHostKey("prod-server", "SHA256:abc", "nota atualizada"); err != nil {
		t.Fatalf("LinkHostKey (segunda chamada): %v", err)
	}

	links, err := svc.ListHostLinks()
	if err != nil {
		t.Fatalf("ListHostLinks: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("esperava 1 vínculo (idempotente), obteve %d", len(links))
	}
	if links[0].Notes != "nota atualizada" {
		t.Errorf("Notes = %q, esperado %q", links[0].Notes, "nota atualizada")
	}
}

func TestKeyService_UnlinkHostKey_RemovesLink(t *testing.T) {
	svc := newTestKeyService(t)

	if err := svc.LinkHostKey("prod-server", "SHA256:abc", ""); err != nil {
		t.Fatalf("LinkHostKey: %v", err)
	}
	if err := svc.UnlinkHostKey("prod-server", "SHA256:abc"); err != nil {
		t.Fatalf("UnlinkHostKey: %v", err)
	}

	links, err := svc.ListHostLinks()
	if err != nil {
		t.Fatalf("ListHostLinks: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("esperava 0 vínculos após unlink, obteve %d", len(links))
	}
}

func TestKeyService_UnlinkHostKey_UnknownPairReturnsErrHostLinkNotFound(t *testing.T) {
	svc := newTestKeyService(t)

	err := svc.UnlinkHostKey("nao-existe", "SHA256:zzz")
	if err != ErrHostLinkNotFound {
		t.Fatalf("erro = %v, esperado ErrHostLinkNotFound", err)
	}
}

func TestMetadataFile_DecodesWithoutHostsField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.json")

	// Simula metadata.json gravado antes deste campo existir: sem "hosts".
	legacy := `{
		"version": 1,
		"keys": [],
		"settings": {"alertThresholdDays": 30, "defaultAlgorithm": "ed25519"}
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st := newMetadataStore(path)
	mf, err := st.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(mf.Hosts) != 0 {
		t.Errorf("Hosts = %v, esperado vazio", mf.Hosts)
	}

	svc := NewFileKeyService(filepath.Join(dir, "ssh"), path)
	links, err := svc.ListHostLinks()
	if err != nil {
		t.Fatalf("ListHostLinks: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("ListHostLinks = %v, esperado vazio", links)
	}
}

// sanity check: confirma que o JSON gravado por LinkHostKey usa as chaves
// documentadas (hostKey/agentKeyFingerprint/notes), não os nomes dos campos Go.
func TestKeyService_LinkHostKey_PersistsExpectedJSONShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.json")
	svc := NewFileKeyService(filepath.Join(dir, "ssh"), path)

	if err := svc.LinkHostKey("h", "SHA256:x", "n"); err != nil {
		t.Fatalf("LinkHostKey: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var decoded struct {
		Hosts []map[string]any `json:"hosts"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(decoded.Hosts) != 1 {
		t.Fatalf("esperava 1 host no JSON, obteve %d", len(decoded.Hosts))
	}
	if decoded.Hosts[0]["hostKey"] != "h" || decoded.Hosts[0]["agentKeyFingerprint"] != "SHA256:x" || decoded.Hosts[0]["notes"] != "n" {
		t.Errorf("shape inesperado: %+v", decoded.Hosts[0])
	}
}
