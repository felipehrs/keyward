//go:build !windows

package sshagent

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh/agent"
)

func TestDial_NoSSHAuthSock(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	_, err := Dial(500 * time.Millisecond)
	if !os.IsNotExist(err) {
		t.Fatalf("erro = %v, esperado os.ErrNotExist", err)
	}
}

func TestDial_NonexistentSocket_FailsWithinTimeout(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(dir, "nao-existe.sock"))

	start := time.Now()
	_, err := Dial(200 * time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("esperava erro ao conectar a socket inexistente")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Dial demorou %v, esperado retorno rápido para socket inexistente", elapsed)
	}
}

func TestDial_SuccessAgainstFakeAgent(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "agent.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	keyring := agent.NewKeyring()

	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func() { _ = agent.ServeAgent(keyring, conn) }()
		}
	}()

	t.Setenv("SSH_AUTH_SOCK", sockPath)

	conn, err := Dial(2 * time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := agent.NewClient(conn)
	identities, err := client.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(identities) != 0 {
		t.Fatalf("esperava 0 identidades, obteve %d", len(identities))
	}
}
