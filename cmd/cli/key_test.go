package main

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/felipehrs/keyward/core"
)

// newTestKeySvc cria um FileKeyService apontando para um diretório de chaves
// e um metadata.json isolados em t.TempDir(), sem AgentDial configurado (o
// dial default falha por falta de SSH_AUTH_SOCK, deixando qualquer registro
// de origem agente em KeyStatusAgentOffline — cenário exercitado pelos
// testes abaixo sem precisar subir um ssh-agent fake).
func newTestKeySvc(t *testing.T) *core.FileKeyService {
	t.Helper()
	dir := t.TempDir()
	svc := core.NewFileKeyService(filepath.Join(dir, "keys"), filepath.Join(dir, "metadata.json"))
	if err := os.MkdirAll(dir+"/keys", 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Garante que o dial default (Unix socket via SSH_AUTH_SOCK) falhe de
	// forma determinística nos testes, independente do ambiente onde rodam.
	svc.AgentDial = func(timeout time.Duration) (net.Conn, error) {
		return nil, os.ErrNotExist
	}
	return svc
}

// seedAgentKeyMetadata grava diretamente um registro de metadata com Source
// == KeySourceAgent, sem depender de um ssh-agent real — o schema de
// metadata.json é estável o bastante (core/metadata.go) para popular via
// JSON aqui, mantendo o teste no pacote cmd/cli em vez de importar
// utilitários internos de core.
func seedAgentKeyMetadata(t *testing.T, svc *core.FileKeyService, fingerprint, label string) {
	t.Helper()
	doc := map[string]any{
		"version": 1,
		"keys": []map[string]any{
			{
				"id":          "test-agent-key-id",
				"fingerprint": fingerprint,
				"label":       label,
				"algorithm":   "ed25519",
				"createdAt":   time.Now().UTC().Format(time.RFC3339),
				"source":      1, // KeySourceAgent
				"agentName":   "1password",
			},
		},
		"settings": map[string]any{
			"alertThresholdDays": 30,
			"defaultAlgorithm":   "ed25519",
		},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(svc.MetadataPath, data, 0o600); err != nil {
		t.Fatalf("WriteFile metadata: %v", err)
	}
}

func TestKeyList_SourceFilter(t *testing.T) {
	svc := newTestKeySvc(t)
	if _, err := svc.GenerateKey(core.GenerateKeyOptions{Algorithm: core.AlgorithmEd25519, FileName: "id_ed25519"}); err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	seedAgentKeyMetadata(t, svc, "SHA256:agentfingerprint", "chave 1password")

	run := func(args ...string) string {
		cmd := newRootCmd(svc, core.NewFileConfigService(""), core.NewFileBackupService("", "", ""))
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
		return out.String()
	}

	t.Run("default mostra arquivo e agente", func(t *testing.T) {
		out := run("key", "list")
		if !strings.Contains(out, "id_ed25519") && !strings.Contains(out, "ed25519") {
			t.Errorf("saída não parece conter a chave de arquivo: %s", out)
		}
		if !strings.Contains(out, "agent-offline") {
			t.Errorf("saída não contém status agent-offline para a chave de agente: %s", out)
		}
		if !strings.Contains(out, "agent (1password)") {
			t.Errorf("saída não contém a coluna SOURCE com o nome do agente: %s", out)
		}
	})

	t.Run("--source=agent mostra só chave de agente", func(t *testing.T) {
		out := run("key", "list", "--source=agent")
		if strings.Contains(out, "chave 1password") == false {
			t.Errorf("esperava a chave de agente na saída: %s", out)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for _, l := range lines[1:] {
			if l == "" {
				continue
			}
			if !strings.Contains(l, "agent") {
				t.Errorf("linha inesperada com --source=agent (deveria ser só agente): %q", l)
			}
		}
	})

	t.Run("--source=file mostra só chave de arquivo", func(t *testing.T) {
		out := run("key", "list", "--source=file")
		if strings.Contains(out, "chave 1password") {
			t.Errorf("--source=file não deveria mostrar a chave de agente: %s", out)
		}
	})

	t.Run("--source inválido retorna erro", func(t *testing.T) {
		cmd := newRootCmd(svc, core.NewFileConfigService(""), core.NewFileBackupService("", "", ""))
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"key", "list", "--source=bogus"})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("esperava erro para --source=bogus")
		}
	})
}

func TestKeyStatusString_AgentOffline(t *testing.T) {
	if got := keyStatusString(core.KeyStatusAgentOffline); got != "agent-offline" {
		t.Errorf("keyStatusString(KeyStatusAgentOffline) = %q, esperado %q", got, "agent-offline")
	}
}

func TestKeySourceString(t *testing.T) {
	cases := []struct {
		key  core.Key
		want string
	}{
		{core.Key{Source: core.KeySourceFile}, "file"},
		{core.Key{Source: core.KeySourceAgent}, "agent"},
		{core.Key{Source: core.KeySourceAgent, AgentName: "1password"}, "agent (1password)"},
	}
	for _, c := range cases {
		if got := keySourceString(c.key); got != c.want {
			t.Errorf("keySourceString(%+v) = %q, esperado %q", c.key, got, c.want)
		}
	}
}
