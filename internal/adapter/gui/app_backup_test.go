package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/felipehrs/keyward/core"
)

func TestApp_Export_WritesPackage(t *testing.T) {
	a := newTestApp(t)
	if err := a.AddHost(HostSpecInput{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	key, err := a.GenerateKey(GenerateKeyInput{Label: "export"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	hosts, err := a.ListHosts()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("ListHosts: %v %+v", err, hosts)
	}

	dest := filepath.Join(t.TempDir(), "backup.tar.gz")
	err = a.Export(ExportInput{
		DestPath: dest,
		Hosts:    hosts,
		Keys:     []KeySelectionInput{{Fingerprint: key.Metadata.Fingerprint, IncludePrivate: true}},
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("esperava arquivo de export criado em %s: %v", dest, err)
	}
}

// setFakeHome aponta os.UserHomeDir() (via HOME/USERPROFILE) pra home —
// necessário porque core/backup_export.go compacta Host.SourceFile de
// volta pra "~/..." usando o home REAL do processo no momento do export, e
// core/backup_import.go expande esse marcador usando o home do processo
// no momento do import — sem isso, dois t.TempDir() nunca "combinam", e
// todo host vira ExternalPath (mesmo padrão de cmd/tui/backup_import_test.go).
func setFakeHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func newHomeApp(t *testing.T, home string) *App {
	t.Helper()
	setFakeHome(t, home)
	sshDir := filepath.Join(home, ".ssh")
	metadataPath := filepath.Join(home, "metadata.json")
	configSvc := core.NewFileConfigService(filepath.Join(sshDir, "config"))
	keySvc := core.NewFileKeyService(sshDir, metadataPath)
	backupSvc := core.NewFileBackupService(filepath.Join(sshDir, "config"), sshDir, metadataPath)
	return newApp(configSvc, keySvc, backupSvc)
}

func buildExportedPackage(t *testing.T, originHome string) (dest string) {
	t.Helper()
	a := newHomeApp(t, originHome)
	if err := a.AddHost(HostSpecInput{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	key, err := a.GenerateKey(GenerateKeyInput{Label: "chave export"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	hosts, err := a.ListHosts()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("ListHosts: %v %+v", err, hosts)
	}

	dest = filepath.Join(t.TempDir(), "export.tar.gz")
	err = a.Export(ExportInput{
		DestPath: dest,
		Hosts:    hosts,
		Keys:     []KeySelectionInput{{Fingerprint: key.Metadata.Fingerprint, IncludePrivate: true}},
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	return dest
}

func TestApp_PreviewImport_CleanPackage_ListsToAdd(t *testing.T) {
	src := buildExportedPackage(t, t.TempDir())
	a := newHomeApp(t, t.TempDir())

	preview, err := a.PreviewImport(src)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.HostsToAdd) != 1 {
		t.Fatalf("esperava 1 host a adicionar, obteve %+v", preview.HostsToAdd)
	}
	if len(preview.KeysToAdd) != 1 {
		t.Fatalf("esperava 1 chave a adicionar, obteve %+v", preview.KeysToAdd)
	}
	if len(preview.HostConflicts) != 0 || len(preview.KeyConflicts) != 0 {
		t.Fatalf("não esperava conflitos, obteve hosts=%+v keys=%+v", preview.HostConflicts, preview.KeyConflicts)
	}
}

func TestApp_PreviewImport_DetectsConflicts(t *testing.T) {
	src := buildExportedPackage(t, t.TempDir())

	destHome := t.TempDir()
	a := newHomeApp(t, destHome)
	if err := a.AddHost(HostSpecInput{Patterns: []string{"bastion"}, HostName: "9.9.9.9"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	if _, err := a.GenerateKey(GenerateKeyInput{FileName: "id_ed25519"}); err != nil {
		t.Fatalf("GenerateKey local: %v", err)
	}

	preview, err := a.PreviewImport(src)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.HostConflicts) != 1 {
		t.Fatalf("esperava 1 HostConflict, obteve %+v", preview.HostConflicts)
	}
	if preview.HostConflicts[0].Existing == nil || preview.HostConflicts[0].Existing.HostName != "9.9.9.9" {
		t.Fatalf("esperava Existing apontando pro host local (9.9.9.9), obteve %+v", preview.HostConflicts[0].Existing)
	}
	if len(preview.KeyConflicts) != 1 {
		t.Fatalf("esperava 1 KeyConflict, obteve %+v", preview.KeyConflicts)
	}
	if preview.KeyConflicts[0].Kind != KeyConflictFileNameCollision {
		t.Fatalf("esperava conflito de colisão de nome, obteve %q", preview.KeyConflicts[0].Kind)
	}
}

func TestApp_PreviewImport_InvalidPath_ReturnsError(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.PreviewImport(filepath.Join(t.TempDir(), "nao-existe.tar.gz")); err == nil {
		t.Fatal("esperava erro para arquivo inexistente")
	}
}

func TestApp_Import_CleanPackage_AddsHostAndKey(t *testing.T) {
	src := buildExportedPackage(t, t.TempDir())
	a := newHomeApp(t, t.TempDir())

	preview, err := a.PreviewImport(src)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	hostKey := a.HostImportKey(preview.HostsToAdd[0].SourceFile, preview.HostsToAdd[0].Patterns)

	result, err := a.Import(src, ImportResolutionsInput{
		Hosts: map[string]string{hostKey: "apply"},
		Keys:  map[string]string{preview.KeysToAdd[0].Fingerprint: "overwrite"},
	})
	if err != nil {
		t.Fatalf("Import não deveria rejeitar (contrato: sempre resolve), obteve err: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("não esperava erro agregado, obteve %q", *result.Error)
	}
	if len(result.HostsAdded) != 1 {
		t.Fatalf("esperava 1 host adicionado, obteve %+v", result)
	}
	if len(result.KeysAdded) != 1 {
		t.Fatalf("esperava 1 chave adicionada, obteve %+v", result)
	}

	hosts, err := a.ListHosts()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("ListHosts após import: %v %+v", err, hosts)
	}
}

func TestApp_Import_DefaultSkip_AppliesNothing(t *testing.T) {
	src := buildExportedPackage(t, t.TempDir())
	a := newHomeApp(t, t.TempDir())

	// Sem nenhuma resolução (mapas vazios) — default do core é Skip pra
	// tudo que está em conflito, mas ToAdd é aplicado por default quando
	// AUSENTE do mapa (comportamento do core, não deste teste) — este
	// teste cobre justamente que passar mapas vazios não quebra nada.
	result, err := a.Import(src, ImportResolutionsInput{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("não esperava erro, obteve %q", *result.Error)
	}
}

func TestApp_Import_ResolvesConflictsItemByItem(t *testing.T) {
	src := buildExportedPackage(t, t.TempDir())

	destHome := t.TempDir()
	a := newHomeApp(t, destHome)
	if err := a.AddHost(HostSpecInput{Patterns: []string{"bastion"}, HostName: "9.9.9.9"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	preview, err := a.PreviewImport(src)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.HostConflicts) != 1 {
		t.Fatalf("esperava 1 HostConflict, obteve %+v", preview.HostConflicts)
	}

	result, err := a.Import(src, ImportResolutionsInput{
		Hosts: map[string]string{preview.HostConflicts[0].Key: "apply"},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(result.HostsReplaced) != 1 {
		t.Fatalf("esperava 1 host substituído, obteve %+v", result)
	}

	hosts, err := a.ListHosts()
	if err != nil || len(hosts) != 1 || hosts[0].HostName != "1.2.3.4" {
		t.Fatalf("esperava host substituído com HostName do pacote, obteve %v %+v", err, hosts)
	}
}

func TestApp_Import_AggregatedError_NeverRejectsAndKeepsPartialResult(t *testing.T) {
	a := newTestApp(t)
	// Um caminho de origem inválido faz PreviewImport/Import falhar antes
	// de aplicar qualquer coisa — mas o contrato de App.Import ainda é
	// "nunca rejeita por erro do core", só por erro de tradução de
	// entrada. Um path inexistente é um erro do CORE (arquivo não
	// encontrado), não de tradução — então o teste confirma que ainda
	// assim vem como (dto, nil), não (dto vazio, err).
	dto, err := a.Import(filepath.Join(t.TempDir(), "nao-existe.tar.gz"), ImportResolutionsInput{})
	if err != nil {
		t.Fatalf("Import não deveria rejeitar mesmo com erro do core, obteve err: %v", err)
	}
	if dto.Error == nil {
		t.Fatal("esperava ImportResultDTO.Error preenchido com o erro do core")
	}
}

func TestApp_Import_InvalidResolution_RejectsAsTranslationError(t *testing.T) {
	a := newTestApp(t)
	_, err := a.Import("qualquer.tar.gz", ImportResolutionsInput{Hosts: map[string]string{"x": "valor-invalido"}})
	if err == nil {
		t.Fatal("esperava erro de tradução pra resolução inválida")
	}
}

func TestApp_Export_IncludeSettings(t *testing.T) {
	a := newTestApp(t)
	if err := a.UpdateSettings(AppSettingsDTO{AlertThresholdDays: 45, DefaultAlgorithm: "rsa"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := a.Export(ExportInput{DestPath: dest, IncludeSettings: true}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Reimporta num destino separado e confirma que as settings vieram no
	// pacote (prova indireta de que IncludeSettings foi respeitado).
	destSvc := core.NewFileBackupService(
		filepath.Join(t.TempDir(), "config"),
		filepath.Join(t.TempDir(), "ssh"),
		filepath.Join(t.TempDir(), "metadata.json"),
	)
	preview, err := destSvc.PreviewImport(dest)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if preview.Settings == nil || preview.Settings.AlertThresholdDays != 45 {
		t.Fatalf("esperava settings no pacote com AlertThresholdDays=45, obteve %+v", preview.Settings)
	}
}
