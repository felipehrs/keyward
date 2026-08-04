package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/felipehrs/keyward/core"
)

func TestHostLinkKeyCmd_RoundTrip(t *testing.T) {
	svc := newTestKeySvc(t)

	run := func(args ...string) (string, error) {
		cmd := newRootCmd(svc, core.NewFileConfigService(""), core.NewFileBackupService("", "", ""))
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), err
	}

	if _, err := run("host", "link-key", "myhost", "--fingerprint=SHA256:abc", "--notes=nota"); err != nil {
		t.Fatalf("link-key: %v", err)
	}

	out, err := run("host", "list-key-links")
	if err != nil {
		t.Fatalf("list-key-links: %v", err)
	}
	if !strings.Contains(out, "myhost") || !strings.Contains(out, "SHA256:abc") || !strings.Contains(out, "nota") {
		t.Errorf("saída inesperada de list-key-links: %s", out)
	}

	if _, err := run("host", "unlink-key", "myhost", "--fingerprint=SHA256:abc"); err != nil {
		t.Fatalf("unlink-key: %v", err)
	}

	out, err = run("host", "list-key-links")
	if err != nil {
		t.Fatalf("list-key-links after unlink: %v", err)
	}
	if strings.Contains(out, "SHA256:abc") {
		t.Errorf("esperava vínculo removido, ainda presente: %s", out)
	}

	if _, err := run("host", "unlink-key", "myhost", "--fingerprint=SHA256:abc"); err == nil {
		t.Errorf("esperava erro ao desvincular vínculo inexistente")
	}
}

func TestHostLinkKeyCmd_RequiresFingerprint(t *testing.T) {
	svc := newTestKeySvc(t)
	cmd := newRootCmd(svc, core.NewFileConfigService(""), core.NewFileBackupService("", "", ""))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"host", "link-key", "myhost"})
	if err := cmd.Execute(); err == nil {
		t.Errorf("esperava erro sem --fingerprint")
	}
}
