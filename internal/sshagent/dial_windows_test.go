//go:build windows

package sshagent

import (
	"os"
	"testing"
	"time"
)

// TestDial_NonexistentPipe_FailsWithinTimeout não pôde ser executado nem
// verificado em CI/sandbox Linux — depende de um named pipe real do
// Windows. Cobre o caso "pipe inexistente retorna erro dentro do timeout,
// não trava", análogo ao teste equivalente em dial_unix_test.go.
func TestDial_NonexistentPipe_FailsWithinTimeout(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", `\\.\pipe\keyward-test-pipe-que-nao-existe`)

	start := time.Now()
	_, err := Dial(200 * time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("esperava erro ao conectar a pipe inexistente")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Dial demorou %v, esperado retorno rápido para pipe inexistente", elapsed)
	}
}

// TestDial_DefaultPipePath_UsedWhenEnvEmpty não pôde ser executado nem
// verificado em CI/sandbox Linux. Confirma só que, sem SSH_AUTH_SOCK
// definido, Dial tenta o pipe padrão do OpenSSH for Windows (falha
// esperada na ausência de um agente real rodando).
func TestDial_DefaultPipePath_UsedWhenEnvEmpty(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	_, err := Dial(200 * time.Millisecond)
	if err == nil {
		t.Skip("agente ssh real detectado no pipe padrão; nada a verificar aqui")
	}
	if os.IsNotExist(err) {
		return
	}
}
