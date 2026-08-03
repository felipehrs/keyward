package core

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func newTestBackupService(t *testing.T) (*FileBackupService, *FileKeyService, *FileConfigService, string) {
	t.Helper()
	dir := t.TempDir()
	keyDir := filepath.Join(dir, "ssh")
	metadataPath := filepath.Join(dir, "config", "metadata.json")
	configPath := filepath.Join(keyDir, "config")

	keySvc := NewFileKeyService(keyDir, metadataPath)
	configSvc := NewFileConfigService(configPath)
	backupSvc := NewFileBackupService(configPath, keyDir, metadataPath)
	return backupSvc, keySvc, configSvc, dir
}

func TestExport_IncludesSelectedHostsAndKeys(t *testing.T) {
	backupSvc, keySvc, configSvc, dir := newTestBackupService(t)

	if err := configSvc.AddHost("", HostSpec{Patterns: []string{"bastion"}, HostName: "203.0.113.10"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	generated, err := keySvc.GenerateKey(GenerateKeyOptions{Label: "teste"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	hosts, err := configSvc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}

	destPath := filepath.Join(dir, "backup.tar.gz")
	err = backupSvc.Export(destPath, ExportOptions{
		Hosts: hosts,
		Keys:  []KeySelection{{Fingerprint: generated.Metadata.Fingerprint, IncludePrivate: true}},
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	files, err := extractArchive(destPath)
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	manifestData, ok := files["manifest.json"]
	if !ok {
		t.Fatal("manifest.json ausente")
	}

	var manifest backupManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("Unmarshal manifest: %v", err)
	}
	if len(manifest.Hosts) != 1 || manifest.Hosts[0].Patterns[0] != "bastion" {
		t.Errorf("Hosts = %+v", manifest.Hosts)
	}
	if len(manifest.Keys) != 1 || !manifest.Keys[0].HasPrivateKey {
		t.Errorf("Keys = %+v", manifest.Keys)
	}

	fileName := manifest.Keys[0].FileName
	if _, ok := files["keys/"+fileName]; !ok {
		t.Error("chave privada ausente no pacote")
	}
	if _, ok := files["keys/"+fileName+".pub"]; !ok {
		t.Error(".pub ausente no pacote")
	}
}

func TestExport_IncludePrivateFalse_OmitsKeyFiles(t *testing.T) {
	backupSvc, keySvc, _, dir := newTestBackupService(t)

	generated, err := keySvc.GenerateKey(GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	destPath := filepath.Join(dir, "backup.tar.gz")
	err = backupSvc.Export(destPath, ExportOptions{
		Keys: []KeySelection{{Fingerprint: generated.Metadata.Fingerprint, IncludePrivate: false}},
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	files, err := extractArchive(destPath)
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	for name := range files {
		if strings.HasPrefix(name, "keys/") {
			t.Errorf("não deveria haver arquivo de chave no pacote: %s", name)
		}
	}
}

func TestExport_HostSourceFileCompactedRelativeToHome(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	outsideConfig := filepath.Join(t.TempDir(), "extra.conf")
	writeFile(t, outsideConfig, "Host external\n    HostName 9.9.9.9\n")

	configSvc := NewFileConfigService("")
	if err := configSvc.AddHost("", HostSpec{Patterns: []string{"local"}, HostName: "1.1.1.1"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	hosts, err := configSvc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	externalHosts, err := listHosts(outsideConfig, 0)
	if err != nil {
		t.Fatalf("listHosts: %v", err)
	}
	hosts = append(hosts, externalHosts...)

	backupSvc := NewFileBackupService("", "", "")
	destPath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := backupSvc.Export(destPath, ExportOptions{Hosts: hosts}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	files, err := extractArchive(destPath)
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	var manifest backupManifest
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	var foundLocal, foundExternal bool
	for _, h := range manifest.Hosts {
		switch h.Patterns[0] {
		case "local":
			foundLocal = true
			if !strings.HasPrefix(h.SourceFile, "~/") {
				t.Errorf("host sob o home deveria exportar SourceFile compactado, obteve %q", h.SourceFile)
			}
		case "external":
			foundExternal = true
			if h.SourceFile != outsideConfig {
				t.Errorf("SourceFile = %q, esperado path absoluto intacto %q", h.SourceFile, outsideConfig)
			}
		}
	}
	if !foundLocal || !foundExternal {
		t.Fatalf("hosts esperados não encontrados no manifest: %+v", manifest.Hosts)
	}
}
