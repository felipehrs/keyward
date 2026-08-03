package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// genTestKeyPair gera um par de chaves ed25519 de verdade em dir (via
// generateKeyFiles) só para exercitar o algoritmo de reconciliação com
// arquivos reais, sem depender de fixtures manuais.
func genTestKeyPair(t *testing.T, dir, fileName string) generatedKey {
	t.Helper()
	got, err := generateKeyFiles(dir, AlgorithmEd25519, GenerateKeyOptions{FileName: fileName})
	if err != nil {
		t.Fatalf("generateKeyFiles(%s): %v", fileName, err)
	}
	return got
}

func TestReconcileKeys_MatchesByFingerprintEvenIfPathChanged(t *testing.T) {
	dir := t.TempDir()
	gen := genTestKeyPair(t, dir, "id_ed25519")

	mf := metadataFile{
		Settings: defaultAppSettings(),
		Keys: []KeyMetadata{
			{
				ID:          "id-1",
				Fingerprint: gen.fingerprint,
				KeyPath:     "/caminho/antigo/que/nao/existe/mais",
				Algorithm:   AlgorithmEd25519,
				CreatedAt:   time.Now(),
			},
		},
	}

	keys, err := reconcileKeys(dir, mf)
	if err != nil {
		t.Fatalf("reconcileKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("esperava 1 chave, obteve %d: %+v", len(keys), keys)
	}
	if keys[0].Status != KeyStatusOK {
		t.Errorf("Status = %v, esperado KeyStatusOK", keys[0].Status)
	}
	if keys[0].PrivateKeyPath != gen.privateKeyPath {
		t.Errorf("PrivateKeyPath = %q, esperado %q (deve refletir o disco, não o keyPath antigo)", keys[0].PrivateKeyPath, gen.privateKeyPath)
	}
}

func TestReconcileKeys_FlagsUnregisteredDiskPair(t *testing.T) {
	dir := t.TempDir()
	genTestKeyPair(t, dir, "id_ed25519")

	keys, err := reconcileKeys(dir, metadataFile{Settings: defaultAppSettings()})
	if err != nil {
		t.Fatalf("reconcileKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].Status != KeyStatusUnregistered {
		t.Fatalf("esperava 1 chave KeyStatusUnregistered, obteve %+v", keys)
	}
}

func TestReconcileKeys_FlagsMissingMetadataFile(t *testing.T) {
	dir := t.TempDir()
	mf := metadataFile{
		Settings: defaultAppSettings(),
		Keys: []KeyMetadata{
			{ID: "id-1", Fingerprint: "SHA256:naoexistenodisco", Algorithm: AlgorithmEd25519, CreatedAt: time.Now()},
		},
	}

	keys, err := reconcileKeys(dir, mf)
	if err != nil {
		t.Fatalf("reconcileKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].Status != KeyStatusMissingFile {
		t.Fatalf("esperava 1 chave KeyStatusMissingFile, obteve %+v", keys)
	}
}

func TestReconcileKeys_IgnoresFilesWithoutPubPair(t *testing.T) {
	dir := t.TempDir()
	genTestKeyPair(t, dir, "id_ed25519")

	// arquivos que convivem tipicamente em ~/.ssh/ e não devem virar "chaves"
	for _, name := range []string{"config", "known_hosts", "authorized_keys"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("conteúdo qualquer"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	// .pub órfão, sem chave privada irmã
	if err := os.WriteFile(filepath.Join(dir, "orfa.pub"), []byte("ssh-ed25519 AAAA orfa"), 0o644); err != nil {
		t.Fatalf("WriteFile(orfa.pub): %v", err)
	}

	keys, err := reconcileKeys(dir, metadataFile{Settings: defaultAppSettings()})
	if err != nil {
		t.Fatalf("reconcileKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("esperava só a chave real, obteve %d: %+v", len(keys), keys)
	}
}

func TestReconcileKeys_IgnoresInvalidPubFile(t *testing.T) {
	dir := t.TempDir()
	genTestKeyPair(t, dir, "id_ed25519")

	// par com .pub corrompido — não deve derrubar o scan das outras chaves
	if err := os.WriteFile(filepath.Join(dir, "quebrada"), []byte("não é uma chave"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "quebrada.pub"), []byte("isso não é um .pub válido"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	keys, err := reconcileKeys(dir, metadataFile{Settings: defaultAppSettings()})
	if err != nil {
		t.Fatalf("reconcileKeys não deveria falhar por causa de um .pub corrompido: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("esperava só a chave válida, obteve %d: %+v", len(keys), keys)
	}
}

func TestReconcileKeys_ComputesAlertFlagsFromSettings(t *testing.T) {
	dir := t.TempDir()
	expired := genTestKeyPair(t, dir, "expired")
	soon := genTestKeyPair(t, dir, "soon")
	future := genTestKeyPair(t, dir, "future")

	now := time.Now()
	pastDate := now.Add(-24 * time.Hour)
	soonDate := now.Add(10 * 24 * time.Hour)
	futureDate := now.Add(200 * 24 * time.Hour)

	mf := metadataFile{
		Settings: AppSettings{AlertThresholdDays: 30, DefaultAlgorithm: AlgorithmEd25519},
		Keys: []KeyMetadata{
			{ID: "1", Fingerprint: expired.fingerprint, Algorithm: AlgorithmEd25519, CreatedAt: now, ExpiresAt: &pastDate},
			{ID: "2", Fingerprint: soon.fingerprint, Algorithm: AlgorithmEd25519, CreatedAt: now, ExpiresAt: &soonDate},
			{ID: "3", Fingerprint: future.fingerprint, Algorithm: AlgorithmEd25519, CreatedAt: now, ExpiresAt: &futureDate},
		},
	}

	keys, err := reconcileKeys(dir, mf)
	if err != nil {
		t.Fatalf("reconcileKeys: %v", err)
	}

	byFingerprint := make(map[string]Key, len(keys))
	for _, k := range keys {
		byFingerprint[k.Metadata.Fingerprint] = k
	}

	if k := byFingerprint[expired.fingerprint]; !k.IsExpired || k.IsExpiringSoon {
		t.Errorf("chave expirada: IsExpired=%v IsExpiringSoon=%v", k.IsExpired, k.IsExpiringSoon)
	}
	if k := byFingerprint[soon.fingerprint]; k.IsExpired || !k.IsExpiringSoon {
		t.Errorf("chave expirando em breve: IsExpired=%v IsExpiringSoon=%v", k.IsExpired, k.IsExpiringSoon)
	}
	if k := byFingerprint[future.fingerprint]; k.IsExpired || k.IsExpiringSoon {
		t.Errorf("chave com expiração distante: IsExpired=%v IsExpiringSoon=%v", k.IsExpired, k.IsExpiringSoon)
	}
}

func TestReconcileKeys_SortedByExpirationProximity(t *testing.T) {
	dir := t.TempDir()
	expired := genTestKeyPair(t, dir, "expired")
	soon := genTestKeyPair(t, dir, "soon")
	future := genTestKeyPair(t, dir, "future")
	noExpiryOld := genTestKeyPair(t, dir, "no-expiry-old")
	noExpiryNew := genTestKeyPair(t, dir, "no-expiry-new")

	now := time.Now()
	pastDate := now.Add(-24 * time.Hour)
	soonDate := now.Add(10 * 24 * time.Hour)
	futureDate := now.Add(200 * 24 * time.Hour)

	mf := metadataFile{
		Settings: AppSettings{AlertThresholdDays: 30, DefaultAlgorithm: AlgorithmEd25519},
		Keys: []KeyMetadata{
			{ID: "1", Fingerprint: expired.fingerprint, Algorithm: AlgorithmEd25519, CreatedAt: now, ExpiresAt: &pastDate},
			{ID: "2", Fingerprint: soon.fingerprint, Algorithm: AlgorithmEd25519, CreatedAt: now, ExpiresAt: &soonDate},
			{ID: "3", Fingerprint: future.fingerprint, Algorithm: AlgorithmEd25519, CreatedAt: now, ExpiresAt: &futureDate},
			{ID: "4", Fingerprint: noExpiryOld.fingerprint, Algorithm: AlgorithmEd25519, CreatedAt: now.Add(-72 * time.Hour)},
			{ID: "5", Fingerprint: noExpiryNew.fingerprint, Algorithm: AlgorithmEd25519, CreatedAt: now.Add(-1 * time.Hour)},
		},
	}

	keys, err := reconcileKeys(dir, mf)
	if err != nil {
		t.Fatalf("reconcileKeys: %v", err)
	}
	if len(keys) != 5 {
		t.Fatalf("esperava 5 chaves, obteve %d", len(keys))
	}

	var gotOrder []string
	for _, k := range keys {
		gotOrder = append(gotOrder, k.Metadata.ID)
	}
	wantOrder := []string{"1", "2", "3", "5", "4"}
	if len(gotOrder) != len(wantOrder) {
		t.Fatalf("gotOrder = %v", gotOrder)
	}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("ordem = %v, esperada %v", gotOrder, wantOrder)
		}
	}
}
