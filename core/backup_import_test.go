package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// buildManifestOnlyArchive monta um pacote só com manifest.json (sem
// arquivos de chave) — usado para testar cenários de host isoladamente.
func buildManifestOnlyArchive(t *testing.T, manifest backupManifest) string {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal manifest: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := createArchive(dest, []archiveFile{{Name: "manifest.json", Mode: 0o600, Data: data}}); err != nil {
		t.Fatalf("createArchive: %v", err)
	}
	return dest
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", dst, err)
	}
}

func TestImport_CrossMachineRoundTrip(t *testing.T) {
	// "máquina de origem"
	originHome := t.TempDir()
	setHomeEnv(t, originHome)

	originConfigSvc := NewFileConfigService("")
	if err := originConfigSvc.AddHost("", HostSpec{Patterns: []string{"bastion"}, HostName: "203.0.113.10", User: "ops"}); err != nil {
		t.Fatalf("AddHost (origem): %v", err)
	}

	originKeySvc := NewFileKeyService("", "")
	generated, err := originKeySvc.GenerateKey(GenerateKeyOptions{Label: "bastion key"})
	if err != nil {
		t.Fatalf("GenerateKey (origem): %v", err)
	}
	if err := originConfigSvc.ReplaceHost("", []string{"bastion"}, HostSpec{
		Patterns:     []string{"bastion"},
		HostName:     "203.0.113.10",
		User:         "ops",
		IdentityFile: []string{generated.PrivateKeyPath},
	}); err != nil {
		t.Fatalf("ReplaceHost (origem): %v", err)
	}

	hosts, err := originConfigSvc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts (origem): %v", err)
	}

	originBackupSvc := NewFileBackupService("", "", "")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	err = originBackupSvc.Export(archivePath, ExportOptions{
		Hosts: hosts,
		Keys:  []KeySelection{{Fingerprint: generated.Metadata.Fingerprint, IncludePrivate: true}},
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// "máquina de destino"
	destHome := t.TempDir()
	setHomeEnv(t, destHome)

	destBackupSvc := NewFileBackupService("", "", "")
	preview, err := destBackupSvc.PreviewImport(archivePath)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.HostsToAdd) != 1 || len(preview.KeysToAdd) != 1 {
		t.Fatalf("preview inesperado: %+v", preview)
	}

	result, err := destBackupSvc.Import(archivePath, ImportResolutions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(result.HostsAdded) != 1 || len(result.KeysAdded) != 1 {
		t.Fatalf("result inesperado: %+v", result)
	}

	destConfigSvc := NewFileConfigService("")
	destHosts, err := destConfigSvc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts (destino): %v", err)
	}
	if len(destHosts) != 1 {
		t.Fatalf("esperava 1 host no destino, obteve %d", len(destHosts))
	}

	wantIdentityFile := filepath.Join(destHome, ".ssh", filepath.Base(generated.PrivateKeyPath))
	if len(destHosts[0].IdentityFile) != 1 || destHosts[0].IdentityFile[0] != wantIdentityFile {
		t.Errorf("IdentityFile no destino = %v, esperado [%s] (deve apontar pro home do destino, não da origem)",
			destHosts[0].IdentityFile, wantIdentityFile)
	}
	if _, err := os.Stat(wantIdentityFile); err != nil {
		t.Errorf("chave privada não foi gravada no destino: %v", err)
	}
}

func TestImport_HostConflict_SkipAndApply(t *testing.T) {
	originHome := t.TempDir()
	setHomeEnv(t, originHome)
	originConfigSvc := NewFileConfigService("")
	if err := originConfigSvc.AddHost("", HostSpec{Patterns: []string{"bastion"}, HostName: "1.1.1.1"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	hosts, err := originConfigSvc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	originBackupSvc := NewFileBackupService("", "", "")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := originBackupSvc.Export(archivePath, ExportOptions{Hosts: hosts}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	destHome := t.TempDir()
	setHomeEnv(t, destHome)
	destConfigSvc := NewFileConfigService("")
	if err := destConfigSvc.AddHost("", HostSpec{Patterns: []string{"bastion"}, HostName: "2.2.2.2"}); err != nil {
		t.Fatalf("AddHost (destino): %v", err)
	}
	destBackupSvc := NewFileBackupService("", "", "")

	preview, err := destBackupSvc.PreviewImport(archivePath)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.HostConflicts) != 1 {
		t.Fatalf("esperava 1 conflito de host, obteve %d: %+v", len(preview.HostConflicts), preview.HostConflicts)
	}
	conflictKey := preview.HostConflicts[0].Key

	t.Run("skip mantém local", func(t *testing.T) {
		result, err := destBackupSvc.Import(archivePath, ImportResolutions{})
		if err != nil {
			t.Fatalf("Import: %v", err)
		}
		if len(result.HostsSkipped) != 1 {
			t.Errorf("esperava host pulado, result=%+v", result)
		}
		got, err := destConfigSvc.ListHosts()
		if err != nil {
			t.Fatalf("ListHosts: %v", err)
		}
		if got[0].HostName != "2.2.2.2" {
			t.Errorf("host local foi alterado sem confirmação: %+v", got)
		}
	})

	t.Run("apply substitui", func(t *testing.T) {
		result, err := destBackupSvc.Import(archivePath, ImportResolutions{
			Hosts: map[string]HostResolution{conflictKey: HostResolutionApply},
		})
		if err != nil {
			t.Fatalf("Import: %v", err)
		}
		if len(result.HostsReplaced) != 1 {
			t.Errorf("esperava host substituído, result=%+v", result)
		}
		got, err := destConfigSvc.ListHosts()
		if err != nil {
			t.Fatalf("ListHosts: %v", err)
		}
		if got[0].HostName != "1.1.1.1" {
			t.Errorf("host local não foi substituído: %+v", got)
		}
	})
}

func TestImport_HostUnchanged_NoOp(t *testing.T) {
	originHome := t.TempDir()
	setHomeEnv(t, originHome)
	originConfigSvc := NewFileConfigService("")
	if err := originConfigSvc.AddHost("", HostSpec{Patterns: []string{"bastion"}, HostName: "1.1.1.1", User: "ops"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	hosts, err := originConfigSvc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	originBackupSvc := NewFileBackupService("", "", "")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := originBackupSvc.Export(archivePath, ExportOptions{Hosts: hosts}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	destHome := t.TempDir()
	setHomeEnv(t, destHome)
	destConfigSvc := NewFileConfigService("")
	if err := destConfigSvc.AddHost("", HostSpec{Patterns: []string{"bastion"}, HostName: "1.1.1.1", User: "ops"}); err != nil {
		t.Fatalf("AddHost (destino): %v", err)
	}
	destBackupSvc := NewFileBackupService("", "", "")

	preview, err := destBackupSvc.PreviewImport(archivePath)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.HostsUnchanged) != 1 || len(preview.HostConflicts) != 0 {
		t.Fatalf("esperava host unchanged sem conflito, preview=%+v", preview)
	}
}

func TestImport_HostExternalPath_RequiresExplicitApply(t *testing.T) {
	destHome := t.TempDir()
	setHomeEnv(t, destHome)

	externalPath := filepath.Join(t.TempDir(), "extra.conf")
	manifest := backupManifest{
		Version: backupManifestVersion,
		Hosts: []backupHost{
			{Patterns: []string{"external"}, SourceFile: externalPath, HostName: "9.9.9.9"},
		},
	}
	archivePath := buildManifestOnlyArchive(t, manifest)
	backupSvc := NewFileBackupService("", "", "")

	preview, err := backupSvc.PreviewImport(archivePath)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.HostConflicts) != 1 || !preview.HostConflicts[0].ExternalPath {
		t.Fatalf("esperava 1 conflito ExternalPath, preview=%+v", preview)
	}
	key := preview.HostConflicts[0].Key

	result, err := backupSvc.Import(archivePath, ImportResolutions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(result.HostsSkipped) != 1 {
		t.Fatalf("default deveria pular host externo, result=%+v", result)
	}
	if _, err := os.Stat(externalPath); err == nil {
		t.Fatal("não deveria ter criado arquivo fora do home sem confirmação explícita")
	}

	result2, err := backupSvc.Import(archivePath, ImportResolutions{
		Hosts: map[string]HostResolution{key: HostResolutionApply},
	})
	if err != nil {
		t.Fatalf("Import (apply): %v", err)
	}
	if len(result2.HostsAdded) != 1 {
		t.Fatalf("esperava host adicionado, result=%+v", result2)
	}
	if _, err := os.Stat(externalPath); err != nil {
		t.Errorf("esperava arquivo externo criado após apply explícito: %v", err)
	}
}

func TestImport_TamperedPortablePath_RejectsWholePackage(t *testing.T) {
	destHome := t.TempDir()
	setHomeEnv(t, destHome)

	manifest := backupManifest{
		Version: backupManifestVersion,
		Hosts: []backupHost{
			{Patterns: []string{"evil"}, SourceFile: "~/../../outside", HostName: "9.9.9.9"},
		},
	}
	archivePath := buildManifestOnlyArchive(t, manifest)
	backupSvc := NewFileBackupService("", "", "")

	if _, err := backupSvc.PreviewImport(archivePath); err == nil {
		t.Fatal("esperava erro para sourceFile adulterado")
	}
	if _, err := backupSvc.Import(archivePath, ImportResolutions{}); err == nil {
		t.Fatal("Import deveria falhar também")
	}
}

func TestImport_KeyAlreadyRegistered_SkipAndOverwrite(t *testing.T) {
	originHome := t.TempDir()
	setHomeEnv(t, originHome)
	originKeySvc := NewFileKeyService("", "")
	generated, err := originKeySvc.GenerateKey(GenerateKeyOptions{Label: "origem", Notes: "nota da origem"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	originBackupSvc := NewFileBackupService("", "", "")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := originBackupSvc.Export(archivePath, ExportOptions{
		Keys: []KeySelection{{Fingerprint: generated.Metadata.Fingerprint, IncludePrivate: true}},
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	destHome := t.TempDir()
	setHomeEnv(t, destHome)
	destSSHDir := filepath.Join(destHome, ".ssh")
	if err := os.MkdirAll(destSSHDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	destPrivPath := filepath.Join(destSSHDir, filepath.Base(generated.PrivateKeyPath))
	destPubPath := filepath.Join(destSSHDir, filepath.Base(generated.PublicKeyPath))
	copyFile(t, generated.PrivateKeyPath, destPrivPath)
	copyFile(t, generated.PublicKeyPath, destPubPath)

	destKeySvc := NewFileKeyService("", "")
	localLabel := "local"
	if _, err := destKeySvc.RegisterKey(destPrivPath, KeyMetadataPatch{Label: &localLabel}); err != nil {
		t.Fatalf("RegisterKey (destino): %v", err)
	}
	localKeyBefore, err := destKeySvc.GetKey(generated.Metadata.Fingerprint)
	if err != nil {
		t.Fatalf("GetKey (destino, antes): %v", err)
	}

	destBackupSvc := NewFileBackupService("", "", "")
	preview, err := destBackupSvc.PreviewImport(archivePath)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.KeyConflicts) != 1 || preview.KeyConflicts[0].Kind != KeyConflictAlreadyRegistered {
		t.Fatalf("esperava 1 KeyConflictAlreadyRegistered, preview=%+v", preview)
	}

	resultSkip, err := destBackupSvc.Import(archivePath, ImportResolutions{})
	if err != nil {
		t.Fatalf("Import (skip): %v", err)
	}
	if len(resultSkip.KeysSkipped) != 1 {
		t.Errorf("esperava chave pulada, result=%+v", resultSkip)
	}
	afterSkip, err := destKeySvc.GetKey(generated.Metadata.Fingerprint)
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if afterSkip.Metadata.Label != "local" {
		t.Errorf("skip não deveria alterar metadata local, obteve %+v", afterSkip.Metadata)
	}

	resultOverwrite, err := destBackupSvc.Import(archivePath, ImportResolutions{
		Keys: map[string]KeyResolution{generated.Metadata.Fingerprint: KeyResolutionOverwrite},
	})
	if err != nil {
		t.Fatalf("Import (overwrite): %v", err)
	}
	if len(resultOverwrite.KeysMetadataUpdated) != 1 {
		t.Fatalf("esperava metadata atualizada, result=%+v", resultOverwrite)
	}
	afterOverwrite, err := destKeySvc.GetKey(generated.Metadata.Fingerprint)
	if err != nil {
		t.Fatalf("GetKey (destino, depois): %v", err)
	}
	if afterOverwrite.Metadata.Label != "origem" {
		t.Errorf("Label = %q, esperado 'origem' (do pacote)", afterOverwrite.Metadata.Label)
	}
	if afterOverwrite.Metadata.ID != localKeyBefore.Metadata.ID {
		t.Errorf("ID deveria permanecer o local: got %q want %q", afterOverwrite.Metadata.ID, localKeyBefore.Metadata.ID)
	}
	if !afterOverwrite.Metadata.CreatedAt.Equal(localKeyBefore.Metadata.CreatedAt) {
		t.Errorf("CreatedAt deveria permanecer o local: got %v want %v", afterOverwrite.Metadata.CreatedAt, localKeyBefore.Metadata.CreatedAt)
	}
}

func TestImport_KeyFileNameCollision_SkipAndRename(t *testing.T) {
	originHome := t.TempDir()
	setHomeEnv(t, originHome)
	originKeySvc := NewFileKeyService("", "")
	generated, err := originKeySvc.GenerateKey(GenerateKeyOptions{FileName: "id_ed25519", Label: "origem"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	originBackupSvc := NewFileBackupService("", "", "")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := originBackupSvc.Export(archivePath, ExportOptions{
		Keys: []KeySelection{{Fingerprint: generated.Metadata.Fingerprint, IncludePrivate: true}},
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	destHome := t.TempDir()
	setHomeEnv(t, destHome)
	destKeySvc := NewFileKeyService("", "")
	otherKey, err := destKeySvc.GenerateKey(GenerateKeyOptions{FileName: "id_ed25519"})
	if err != nil {
		t.Fatalf("GenerateKey (destino): %v", err)
	}

	destBackupSvc := NewFileBackupService("", "", "")
	preview, err := destBackupSvc.PreviewImport(archivePath)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.KeyConflicts) != 1 || preview.KeyConflicts[0].Kind != KeyConflictFileNameCollision {
		t.Fatalf("esperava 1 KeyConflictFileNameCollision, preview=%+v", preview)
	}
	if preview.KeyConflicts[0].ExistingFingerprint != otherKey.Metadata.Fingerprint {
		t.Errorf("ExistingFingerprint = %q, esperado %q", preview.KeyConflicts[0].ExistingFingerprint, otherKey.Metadata.Fingerprint)
	}

	t.Run("skip nada muda", func(t *testing.T) {
		result, err := destBackupSvc.Import(archivePath, ImportResolutions{})
		if err != nil {
			t.Fatalf("Import: %v", err)
		}
		if len(result.KeysSkipped) != 1 {
			t.Errorf("esperava chave pulada, result=%+v", result)
		}
		stillThere, err := destKeySvc.GetKey(otherKey.Metadata.Fingerprint)
		if err != nil || stillThere.Status != KeyStatusOK {
			t.Errorf("chave local original deveria continuar intacta: %+v, err=%v", stillThere, err)
		}
	})

	t.Run("importrenamed preserva as duas", func(t *testing.T) {
		result, err := destBackupSvc.Import(archivePath, ImportResolutions{
			Keys: map[string]KeyResolution{generated.Metadata.Fingerprint: KeyResolutionImportRenamed},
		})
		if err != nil {
			t.Fatalf("Import: %v", err)
		}
		if len(result.KeysImportedRenamed) != 1 {
			t.Fatalf("esperava chave importada renomeada, result=%+v", result)
		}

		keys, err := destKeySvc.ListKeys()
		if err != nil {
			t.Fatalf("ListKeys: %v", err)
		}
		if len(keys) != 2 {
			t.Fatalf("esperava 2 chaves no destino, obteve %d: %+v", len(keys), keys)
		}

		orig, err := destKeySvc.GetKey(otherKey.Metadata.Fingerprint)
		if err != nil || orig.Status != KeyStatusOK {
			t.Errorf("chave original do destino deveria continuar intacta: %+v", orig)
		}
		imported, err := destKeySvc.GetKey(generated.Metadata.Fingerprint)
		if err != nil || imported.Status != KeyStatusOK {
			t.Errorf("chave importada deveria estar registrada: %+v, err=%v", imported, err)
		}
		if imported.PrivateKeyPath == orig.PrivateKeyPath {
			t.Error("chave importada deveria ter path diferente da original")
		}
	})
}

func TestImport_KeyFileNameCollision_Overwrite(t *testing.T) {
	originHome := t.TempDir()
	setHomeEnv(t, originHome)
	originKeySvc := NewFileKeyService("", "")
	generated, err := originKeySvc.GenerateKey(GenerateKeyOptions{FileName: "id_ed25519"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	originBackupSvc := NewFileBackupService("", "", "")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := originBackupSvc.Export(archivePath, ExportOptions{
		Keys: []KeySelection{{Fingerprint: generated.Metadata.Fingerprint, IncludePrivate: true}},
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	destHome := t.TempDir()
	setHomeEnv(t, destHome)
	destKeySvc := NewFileKeyService("", "")
	otherKey, err := destKeySvc.GenerateKey(GenerateKeyOptions{FileName: "id_ed25519"})
	if err != nil {
		t.Fatalf("GenerateKey (destino): %v", err)
	}

	destBackupSvc := NewFileBackupService("", "", "")
	result, err := destBackupSvc.Import(archivePath, ImportResolutions{
		Keys: map[string]KeyResolution{generated.Metadata.Fingerprint: KeyResolutionOverwrite},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(result.KeysFileOverwritten) != 1 {
		t.Fatalf("esperava chave sobrescrita, result=%+v", result)
	}

	newKey, err := destKeySvc.GetKey(generated.Metadata.Fingerprint)
	if err != nil || newKey.Status != KeyStatusOK {
		t.Fatalf("chave importada deveria estar KeyStatusOK: %+v, err=%v", newKey, err)
	}

	oldKey, err := destKeySvc.GetKey(otherKey.Metadata.Fingerprint)
	if err != nil {
		t.Fatalf("GetKey (chave antiga): %v", err)
	}
	if oldKey.Status != KeyStatusMissingFile {
		t.Errorf("chave antiga deveria virar KeyStatusMissingFile após o arquivo ser sobrescrito, obteve %v", oldKey.Status)
	}
}

func TestImport_FingerprintMismatch_Rejected(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	keySvc := NewFileKeyService("", "")
	genA, err := keySvc.GenerateKey(GenerateKeyOptions{FileName: "id_a"})
	if err != nil {
		t.Fatalf("GenerateKey A: %v", err)
	}
	genB, err := keySvc.GenerateKey(GenerateKeyOptions{FileName: "id_b"})
	if err != nil {
		t.Fatalf("GenerateKey B: %v", err)
	}

	privB, err := os.ReadFile(genB.PrivateKeyPath)
	if err != nil {
		t.Fatalf("ReadFile privB: %v", err)
	}
	pubB, err := os.ReadFile(genB.PublicKeyPath)
	if err != nil {
		t.Fatalf("ReadFile pubB: %v", err)
	}

	// manifest declara o fingerprint de A, mas o .pub incluído é o de B —
	// simula um pacote adulterado.
	manifest := backupManifest{
		Version: backupManifestVersion,
		Keys: []backupKey{
			{Metadata: genA.Metadata, FileName: "id_mismatch", HasPrivateKey: true},
		},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	err = createArchive(archivePath, []archiveFile{
		{Name: "manifest.json", Mode: 0o600, Data: manifestData},
		{Name: "keys/id_mismatch", Mode: 0o600, Data: privB},
		{Name: "keys/id_mismatch.pub", Mode: 0o644, Data: pubB},
	})
	if err != nil {
		t.Fatalf("createArchive: %v", err)
	}

	backupSvc := NewFileBackupService("", "", "")
	if _, err := backupSvc.PreviewImport(archivePath); err == nil {
		t.Fatal("esperava erro por fingerprint divergente do .pub declarado")
	}
}

func TestImport_MetadataOnly_KeyBecomesMissingFile(t *testing.T) {
	originHome := t.TempDir()
	setHomeEnv(t, originHome)
	originKeySvc := NewFileKeyService("", "")
	generated, err := originKeySvc.GenerateKey(GenerateKeyOptions{Label: "só metadata"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	originBackupSvc := NewFileBackupService("", "", "")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := originBackupSvc.Export(archivePath, ExportOptions{
		Keys: []KeySelection{{Fingerprint: generated.Metadata.Fingerprint, IncludePrivate: false}},
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	destHome := t.TempDir()
	setHomeEnv(t, destHome)
	destBackupSvc := NewFileBackupService("", "", "")
	result, err := destBackupSvc.Import(archivePath, ImportResolutions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(result.KeysAdded) != 1 {
		t.Fatalf("esperava metadata adicionada, result=%+v", result)
	}

	destKeySvc := NewFileKeyService("", "")
	key, err := destKeySvc.GetKey(generated.Metadata.Fingerprint)
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if key.Status != KeyStatusMissingFile {
		t.Errorf("Status = %v, esperado KeyStatusMissingFile (só metadata foi importada)", key.Status)
	}
	if key.Metadata.Label != "só metadata" {
		t.Errorf("Label = %q", key.Metadata.Label)
	}
}

func TestImport_CherryPick_SkipsNonConflictingItem(t *testing.T) {
	originHome := t.TempDir()
	setHomeEnv(t, originHome)
	originConfigSvc := NewFileConfigService("")
	if err := originConfigSvc.AddHost("", HostSpec{Patterns: []string{"a"}, HostName: "1.1.1.1"}); err != nil {
		t.Fatalf("AddHost a: %v", err)
	}
	if err := originConfigSvc.AddHost("", HostSpec{Patterns: []string{"b"}, HostName: "2.2.2.2"}); err != nil {
		t.Fatalf("AddHost b: %v", err)
	}
	hosts, err := originConfigSvc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}

	originBackupSvc := NewFileBackupService("", "", "")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := originBackupSvc.Export(archivePath, ExportOptions{Hosts: hosts}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	destHome := t.TempDir()
	setHomeEnv(t, destHome)
	destBackupSvc := NewFileBackupService("", "", "")

	preview, err := destBackupSvc.PreviewImport(archivePath)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.HostsToAdd) != 2 {
		t.Fatalf("esperava 2 hosts a adicionar, preview=%+v", preview)
	}

	var skipKey string
	for _, h := range preview.HostsToAdd {
		if h.Patterns[0] == "b" {
			skipKey = HostImportKey(h.SourceFile, h.Patterns)
		}
	}
	if skipKey == "" {
		t.Fatal("não encontrei a chave de cherry-pick para o host b")
	}

	result, err := destBackupSvc.Import(archivePath, ImportResolutions{
		Hosts: map[string]HostResolution{skipKey: HostResolutionSkip},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(result.HostsAdded) != 1 || result.HostsAdded[0].Patterns[0] != "a" {
		t.Errorf("esperava só host 'a' adicionado, result=%+v", result)
	}
	if len(result.HostsSkipped) != 1 || result.HostsSkipped[0].Patterns[0] != "b" {
		t.Errorf("esperava host 'b' pulado, result=%+v", result)
	}
}

func TestImport_SettingsAppliedWhenPresent(t *testing.T) {
	originHome := t.TempDir()
	setHomeEnv(t, originHome)
	originKeySvc := NewFileKeyService("", "")
	if err := originKeySvc.UpdateSettings(AppSettings{AlertThresholdDays: 90, DefaultAlgorithm: AlgorithmRSA}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	originBackupSvc := NewFileBackupService("", "", "")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := originBackupSvc.Export(archivePath, ExportOptions{IncludeSettings: true}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	destHome := t.TempDir()
	setHomeEnv(t, destHome)
	destBackupSvc := NewFileBackupService("", "", "")
	result, err := destBackupSvc.Import(archivePath, ImportResolutions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !result.SettingsApplied {
		t.Fatal("esperava SettingsApplied=true")
	}

	destKeySvc := NewFileKeyService("", "")
	got, err := destKeySvc.Settings()
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if got.AlertThresholdDays != 90 || got.DefaultAlgorithm != AlgorithmRSA {
		t.Errorf("Settings = %+v", got)
	}
}

func TestImport_SettingsOmittedWhenNotIncluded(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)
	backupSvc := NewFileBackupService("", "", "")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := backupSvc.Export(archivePath, ExportOptions{IncludeSettings: false}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	result, err := backupSvc.Import(archivePath, ImportResolutions{})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.SettingsApplied {
		t.Error("SettingsApplied deveria ser false quando o pacote não inclui settings")
	}
}

func TestImport_FutureManifestVersion_Rejected(t *testing.T) {
	home := t.TempDir()
	setHomeEnv(t, home)

	manifest := backupManifest{Version: backupManifestVersion + 1}
	archivePath := buildManifestOnlyArchive(t, manifest)
	backupSvc := NewFileBackupService("", "", "")

	if _, err := backupSvc.PreviewImport(archivePath); err == nil {
		t.Fatal("esperava erro para versão de manifest futura")
	}
	if _, err := backupSvc.Import(archivePath, ImportResolutions{}); err == nil {
		t.Fatal("Import deveria falhar também")
	}
}

// TestImport_PartialFailureDoesNotAbortOtherItems força uma falha de E/S
// isolada num host (diretório de destino bloqueado por um arquivo no
// lugar), sem que isso seja capturado antes como um conflito legítimo — a
// falha precisa acontecer só no momento de aplicar (AddHost), não durante a
// análise, senão estaríamos testando outra coisa (rejeição do pacote
// inteiro). Um segundo host e uma chave, totalmente independentes, devem
// ser aplicados normalmente mesmo assim.
func TestImport_PartialFailureDoesNotAbortOtherItems(t *testing.T) {
	originHome := t.TempDir()
	setHomeEnv(t, originHome)
	originConfigSvc := NewFileConfigService("")
	if err := originConfigSvc.AddHost("", HostSpec{Patterns: []string{"ok-host"}, HostName: "1.1.1.1"}); err != nil {
		t.Fatalf("AddHost ok-host: %v", err)
	}
	brokenTarget := filepath.Join(originHome, ".ssh", "conf.d", "broken.conf")
	if err := originConfigSvc.AddHost(brokenTarget, HostSpec{Patterns: []string{"broken-host"}, HostName: "2.2.2.2"}); err != nil {
		t.Fatalf("AddHost broken-host (origem): %v", err)
	}

	originKeySvc := NewFileKeyService("", "")
	generated, err := originKeySvc.GenerateKey(GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	okHosts, err := originConfigSvc.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	// broken-host está num arquivo órfão (sem Include no principal), então
	// ListHosts() do principal não o enxerga — coletamos manualmente, igual
	// ao padrão já usado em TestExport_HostSourceFileCompactedRelativeToHome.
	brokenHosts, err := listHosts(brokenTarget, 0)
	if err != nil {
		t.Fatalf("listHosts (broken): %v", err)
	}
	hosts := append(okHosts, brokenHosts...)

	originBackupSvc := NewFileBackupService("", "", "")
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := originBackupSvc.Export(archivePath, ExportOptions{
		Hosts: hosts,
		Keys:  []KeySelection{{Fingerprint: generated.Metadata.Fingerprint, IncludePrivate: true}},
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	destHome := t.TempDir()
	setHomeEnv(t, destHome)
	destSSHDir := filepath.Join(destHome, ".ssh")
	if err := os.MkdirAll(destSSHDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// bloqueia conf.d/ com um ARQUIVO no lugar do diretório — AddHost de
	// broken-host vai falhar ao tentar criar esse diretório, sem que isso
	// seja detectável durante a análise (que não faz I/O de host nenhum).
	if err := os.WriteFile(filepath.Join(destSSHDir, "conf.d"), []byte("bloqueado"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	destBackupSvc := NewFileBackupService("", "", "")
	result, err := destBackupSvc.Import(archivePath, ImportResolutions{})
	if err == nil {
		t.Fatal("esperava erro agregado (falha no host broken-host), mas Import retornou nil")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("esperava 1 erro agregado, obteve %d: %v", len(result.Errors), result.Errors)
	}
	if len(result.HostsAdded) != 1 || result.HostsAdded[0].Patterns[0] != "ok-host" {
		t.Errorf("host independente deveria ter sido adicionado mesmo com falha em outro host, result=%+v", result)
	}
	if len(result.KeysAdded) != 1 {
		t.Errorf("chave independente deveria ter sido importada mesmo com falha no host, result=%+v", result)
	}
}
