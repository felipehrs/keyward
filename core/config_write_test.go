package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAddHost_PreservesRestOfFileByteForByte(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	original := "# comentário de topo\n\n" +
		"Host bastion\n" +
		"    HostName 203.0.113.10\n" +
		"    User ops\n" +
		"\n" +
		"# comentário do segundo host\n" +
		"Host work\n" +
		"    HostName 10.0.0.5\n"
	writeFile(t, path, original)

	svc := NewFileConfigService(path)
	if err := svc.AddHost("", HostSpec{Patterns: []string{"newhost"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(got), original) {
		t.Fatalf("conteúdo original não preservado verbatim.\noriginal:\n%s\nresultado:\n%s", original, got)
	}
}

func TestAddHost_InsertsBeforeTrailingCatchAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	writeFile(t, path, "Host bastion\n    HostName 203.0.113.10\n\nHost *\n    ServerAliveInterval 60\n")

	svc := NewFileConfigService(path)
	if err := svc.AddHost("", HostSpec{Patterns: []string{"newhost"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(got)

	idxNew := strings.Index(content, "Host newhost")
	idxCatchAll := strings.Index(content, "Host *")
	if idxNew == -1 || idxCatchAll == -1 {
		t.Fatalf("blocos esperados não encontrados:\n%s", content)
	}
	if idxNew > idxCatchAll {
		t.Fatalf("Host newhost deveria vir antes de Host *, resultado:\n%s", content)
	}
}

func TestAddHost_NoCatchAll_AppendsAtEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	writeFile(t, path, "Host bastion\n    HostName 203.0.113.10\n")

	svc := NewFileConfigService(path)
	if err := svc.AddHost("", HostSpec{Patterns: []string{"newhost"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	hosts, err := svc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 2 || hosts[len(hosts)-1].Patterns[0] != "newhost" {
		t.Fatalf("esperava newhost por último, obteve %+v", hosts)
	}
}

func TestAddHost_DuplicatePatterns_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	writeFile(t, path, "Host bastion\n    HostName 203.0.113.10\n")

	svc := NewFileConfigService(path)
	if err := svc.AddHost("", HostSpec{Patterns: []string{"bastion"}, HostName: "9.9.9.9"}); err == nil {
		t.Fatal("esperava erro ao adicionar host com Patterns já existente")
	}
}

func TestAddHost_InvalidInput_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")

	svc := NewFileConfigService(path)
	if err := svc.AddHost("", HostSpec{}); err == nil {
		t.Fatal("esperava erro para HostSpec sem Patterns")
	}
}

func TestReplaceHost_ReplacesExistingBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	writeFile(t, path, "# comentário do bastion\nHost bastion\n    HostName 203.0.113.10\n    User old\n")

	svc := NewFileConfigService(path)
	err := svc.ReplaceHost("", []string{"bastion"}, HostSpec{
		Patterns: []string{"bastion"},
		HostName: "198.51.100.1",
		User:     "new",
	})
	if err != nil {
		t.Fatalf("ReplaceHost: %v", err)
	}

	hosts, err := svc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("esperava 1 host, obteve %d: %+v", len(hosts), hosts)
	}
	if hosts[0].HostName != "198.51.100.1" || hosts[0].User != "new" {
		t.Errorf("host não refletiu a substituição: %+v", hosts[0])
	}
}

func TestReplaceHost_MissingHost_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	writeFile(t, path, "Host bastion\n    HostName 203.0.113.10\n")

	svc := NewFileConfigService(path)
	err := svc.ReplaceHost("", []string{"nao-existe"}, HostSpec{Patterns: []string{"nao-existe"}, HostName: "1.2.3.4"})
	if err == nil {
		t.Fatal("esperava erro ao substituir host inexistente")
	}
}

func TestRemoveHost_RemovesExistingBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	writeFile(t, path, "Host bastion\n    HostName 203.0.113.10\n\nHost work\n    HostName 10.0.0.5\n")

	svc := NewFileConfigService(path)
	if err := svc.RemoveHost("", []string{"bastion"}); err != nil {
		t.Fatalf("RemoveHost: %v", err)
	}

	hosts, err := svc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Patterns[0] != "work" {
		t.Fatalf("esperava só 'work' restante, obteve %+v", hosts)
	}
}

func TestRemoveHost_PreservesRestOfFileByteForByte(t *testing.T) {
	// Nota: comentários imediatamente ANTES de um bloco Host são atribuídos
	// pela lib kevinburke/ssh_config ao texto bruto do bloco ANTERIOR, não
	// ao bloco seguinte — por isso o marcador de conteúdo preservado aqui
	// fica dentro do próprio bloco "work" (não numa linha de comentário
	// separada acima dele), para não depender dessa atribuição ambígua.
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	kept := "Host work\n" +
		"    HostName 10.0.0.5 # comentário do segundo host\n"
	original := "# comentário de topo\n\n" +
		"Host bastion\n" +
		"    HostName 203.0.113.10\n" +
		"    User ops\n" +
		"\n" +
		kept
	writeFile(t, path, original)

	svc := NewFileConfigService(path)
	if err := svc.RemoveHost("", []string{"bastion"}); err != nil {
		t.Fatalf("RemoveHost: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(got), kept) {
		t.Fatalf("bloco não removido não preservado verbatim.\nesperado conter:\n%s\nresultado:\n%s", kept, got)
	}
	if strings.Contains(string(got), "Host bastion") {
		t.Fatalf("bloco removido ainda presente:\n%s", got)
	}
}

func TestRemoveHost_MissingHost_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	writeFile(t, path, "Host bastion\n    HostName 203.0.113.10\n")

	svc := NewFileConfigService(path)
	if err := svc.RemoveHost("", []string{"nao-existe"}); err == nil {
		t.Fatal("esperava erro ao remover host inexistente")
	}
}

func TestAddHost_SpecificIncludeFile_Existing(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	sshDir := filepath.Join(home, ".ssh")
	confDir := filepath.Join(sshDir, "conf.d")
	if err := os.MkdirAll(confDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(sshDir, "config"), "Host bastion\n    HostName 203.0.113.10\n\nInclude conf.d/*.conf\n")
	writeFile(t, filepath.Join(confDir, "work.conf"), "Host work-vm\n    HostName 10.0.0.5\n")

	svc := NewFileConfigService("")
	target := filepath.Join(confDir, "work.conf")
	if err := svc.AddHost(target, HostSpec{Patterns: []string{"work-vm-2"}, HostName: "10.0.0.6"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	hosts, err := svc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	found := false
	for _, h := range hosts {
		if len(h.Patterns) > 0 && h.Patterns[0] == "work-vm-2" {
			found = true
			if h.SourceFile != target {
				t.Errorf("SourceFile = %q, esperado %q", h.SourceFile, target)
			}
		}
	}
	if !found {
		t.Fatalf("host work-vm-2 não encontrado via ListHosts: %+v", hosts)
	}

	mainContent, err := os.ReadFile(filepath.Join(sshDir, "config"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(mainContent), "work-vm-2") {
		t.Error("arquivo principal não deveria ter sido tocado")
	}
}

func TestAddHost_SpecificFile_DoesNotExist_CreatesOrphan(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(sshDir, "config"), "Host bastion\n    HostName 203.0.113.10\n")

	orphanPath := filepath.Join(sshDir, "conf.d", "orphan.conf")
	svc := NewFileConfigService("")
	if err := svc.AddHost(orphanPath, HostSpec{Patterns: []string{"orphan-host"}, HostName: "9.9.9.9"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	if _, err := os.Stat(orphanPath); err != nil {
		t.Fatalf("arquivo órfão deveria ter sido criado: %v", err)
	}

	hosts, err := svc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	for _, h := range hosts {
		if len(h.Patterns) > 0 && h.Patterns[0] == "orphan-host" {
			t.Fatalf("host órfão não deveria ser alcançável via ListHosts do arquivo principal: %+v", hosts)
		}
	}
}

func TestAddHost_MainFileDoesNotExist_CreatesDirAndFile(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	svc := NewFileConfigService("")
	if err := svc.AddHost("", HostSpec{Patterns: []string{"newhost"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	hosts, err := svc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Patterns[0] != "newhost" {
		t.Fatalf("esperava 1 host newhost, obteve %+v", hosts)
	}
}

func TestWriteHost_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissões POSIX não se aplicam da mesma forma no Windows")
	}

	t.Run("new file gets default mode", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config")
		svc := NewFileConfigService(path)
		if err := svc.AddHost("", HostSpec{Patterns: []string{"newhost"}, HostName: "1.2.3.4"}); err != nil {
			t.Fatalf("AddHost: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("permissão de arquivo novo = %o, esperado 600", perm)
		}
	})

	t.Run("existing file keeps its mode", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config")
		writeFile(t, path, "Host bastion\n    HostName 203.0.113.10\n")
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("Chmod: %v", err)
		}

		svc := NewFileConfigService(path)
		if err := svc.ReplaceHost("", []string{"bastion"}, HostSpec{Patterns: []string{"bastion"}, HostName: "9.9.9.9"}); err != nil {
			t.Fatalf("ReplaceHost: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o644 {
			t.Errorf("permissão preservada = %o, esperado 644", perm)
		}
	})
}

func TestAddHost_ScopedToSingleFile(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	sshDir := filepath.Join(home, ".ssh")
	confDir := filepath.Join(sshDir, "conf.d")
	if err := os.MkdirAll(confDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(sshDir, "config"), "Include conf.d/*.conf\n")
	writeFile(t, filepath.Join(confDir, "work.conf"), "Host shared\n    HostName 10.0.0.5\n")

	svc := NewFileConfigService("")
	err := svc.AddHost(filepath.Join(sshDir, "config"), HostSpec{Patterns: []string{"shared"}, HostName: "9.9.9.9"})
	if err != nil {
		t.Fatalf("AddHost não deveria falhar por causa de um host homônimo em outro arquivo: %v", err)
	}
}

func TestCompactHome_RoundTripsWithExpandHome(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	underHome := filepath.Join(home, ".ssh", "id_ed25519")
	want := "~/.ssh/id_ed25519"
	if got := compactHome(underHome); got != want {
		t.Errorf("compactHome(%q) = %q, esperado %q", underHome, got, want)
	}
	if got := expandHome(compactHome(underHome)); got != underHome {
		t.Errorf("roundtrip falhou: %q != %q", got, underHome)
	}

	outsideHome := filepath.Join(t.TempDir(), "somewhere", "id_rsa")
	if got := compactHome(outsideHome); got != outsideHome {
		t.Errorf("compactHome fora do home deveria retornar path intacto, obteve %q", got)
	}
}
