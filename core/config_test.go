package core

import (
	"os"
	"path/filepath"
	"testing"
)

func hostByFirstPattern(t *testing.T, hosts []Host, pattern string) Host {
	t.Helper()
	for _, h := range hosts {
		if len(h.Patterns) > 0 && h.Patterns[0] == pattern {
			return h
		}
	}
	t.Fatalf("host com pattern %q não encontrado em %+v", pattern, hosts)
	return Host{}
}

func TestListHosts_ParsesBasicFields(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "config"), `
Host bastion
    HostName 203.0.113.10
    User ops
    Port 2200
    IdentityFile ~/.ssh/id_ed25519

Host *
    ServerAliveInterval 60
`)

	svc := NewFileConfigService(filepath.Join(tmp, "config"))
	hosts, err := svc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("esperava 1 host (bastion), obteve %d: %+v", len(hosts), hosts)
	}

	bastion := hostByFirstPattern(t, hosts, "bastion")
	if bastion.HostName != "203.0.113.10" {
		t.Errorf("HostName = %q, esperado 203.0.113.10", bastion.HostName)
	}
	if bastion.User != "ops" {
		t.Errorf("User = %q, esperado ops", bastion.User)
	}
	if bastion.Port != "2200" {
		t.Errorf("Port = %q, esperado 2200", bastion.Port)
	}
	if len(bastion.IdentityFile) != 1 {
		t.Fatalf("esperava 1 IdentityFile, obteve %v", bastion.IdentityFile)
	}
}

func TestListHosts_MultipleIdentityFiles(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "config"), `
Host work
    HostName 10.0.0.5
    IdentityFile ~/.ssh/id_work_a
    IdentityFile ~/.ssh/id_work_b
`)

	svc := NewFileConfigService(filepath.Join(tmp, "config"))
	hosts, err := svc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}

	work := hostByFirstPattern(t, hosts, "work")
	if len(work.IdentityFile) != 2 {
		t.Fatalf("esperava 2 IdentityFile, obteve %v", work.IdentityFile)
	}
}

func TestListHosts_FollowsIncludeRecursively(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	sshDir := filepath.Join(home, ".ssh")
	confDir := filepath.Join(sshDir, "conf.d")
	if err := os.MkdirAll(confDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	writeFile(t, filepath.Join(sshDir, "config"), `
Host bastion
    HostName 203.0.113.10

Host *
    ServerAliveInterval 60

Include conf.d/*.conf
`)
	writeFile(t, filepath.Join(confDir, "work.conf"), `
Host work-vm
    HostName 10.0.0.5
    User felipe
    Port 2222
`)

	svc := NewFileConfigService("")
	hosts, err := svc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("esperava 2 hosts (bastion + work-vm), obteve %d: %+v", len(hosts), hosts)
	}

	bastion := hostByFirstPattern(t, hosts, "bastion")
	if bastion.SourceFile != filepath.Join(sshDir, "config") {
		t.Errorf("SourceFile de bastion = %q", bastion.SourceFile)
	}

	workVM := hostByFirstPattern(t, hosts, "work-vm")
	if workVM.SourceFile != filepath.Join(confDir, "work.conf") {
		t.Errorf("SourceFile de work-vm = %q", workVM.SourceFile)
	}
	if workVM.User != "felipe" || workVM.Port != "2222" {
		t.Errorf("work-vm com campos inesperados: %+v", workVM)
	}
}

func TestListHosts_MissingFileReturnsEmpty(t *testing.T) {
	svc := NewFileConfigService(filepath.Join(t.TempDir(), "does-not-exist"))
	hosts, err := svc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("esperava lista vazia, obteve %+v", hosts)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// setHomeEnv redireciona tanto os.UserHomeDir() quanto os.UserConfigDir()
// para dentro de dir, cobrindo as variáveis de ambiente relevantes em cada
// SO (HOME/USERPROFILE para o home; AppData no Windows e XDG_CONFIG_HOME no
// Linux para o config dir — no Windows, os.UserConfigDir() lê a variável
// AppData diretamente, SEM derivar de USERPROFILE, então sem isso os testes
// vazam para o %AppData% real da máquina em vez de ficar isolados no
// diretório temporário do teste).
func setHomeEnv(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("AppData", filepath.Join(dir, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
}
