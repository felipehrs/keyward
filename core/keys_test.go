package core

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestKeyService(t *testing.T) *FileKeyService {
	t.Helper()
	dir := t.TempDir()
	return NewFileKeyService(
		filepath.Join(dir, "ssh"),
		filepath.Join(dir, "config", "metadata.json"),
	)
}

func TestKeyService_GenerateThenList_EndToEnd(t *testing.T) {
	svc := newTestKeyService(t)

	generated, err := svc.GenerateKey(GenerateKeyOptions{Label: "teste"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if generated.Status != KeyStatusOK {
		t.Fatalf("Status = %v, esperado KeyStatusOK", generated.Status)
	}
	if generated.Metadata.Label != "teste" {
		t.Errorf("Label = %q", generated.Metadata.Label)
	}

	keys, err := svc.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("esperava 1 chave, obteve %d", len(keys))
	}
	if keys[0].Metadata.Fingerprint != generated.Metadata.Fingerprint {
		t.Errorf("fingerprint não bate: %q vs %q", keys[0].Metadata.Fingerprint, generated.Metadata.Fingerprint)
	}
}

func TestKeyService_UpdateMetadata_PersistsAndReflectsInListKeys(t *testing.T) {
	svc := newTestKeyService(t)
	generated, err := svc.GenerateKey(GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	newLabel := "renomeada"
	expiresAt := time.Now().Add(60 * 24 * time.Hour).UTC().Truncate(time.Second)
	updated, err := svc.UpdateMetadata(generated.Metadata.Fingerprint, KeyMetadataPatch{
		Label:     &newLabel,
		ExpiresAt: ptrToPtr(&expiresAt),
	})
	if err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	if updated.Metadata.Label != newLabel {
		t.Errorf("Label = %q, esperado %q", updated.Metadata.Label, newLabel)
	}
	if updated.Metadata.ExpiresAt == nil || !updated.Metadata.ExpiresAt.Equal(expiresAt) {
		t.Errorf("ExpiresAt = %v, esperado %v", updated.Metadata.ExpiresAt, expiresAt)
	}

	keys, err := svc.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if keys[0].Metadata.Label != newLabel {
		t.Errorf("ListKeys não refletiu a atualização: Label = %q", keys[0].Metadata.Label)
	}
}

func TestKeyService_UpdateMetadata_ClearsExpiresAt(t *testing.T) {
	svc := newTestKeyService(t)
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	generated, err := svc.GenerateKey(GenerateKeyOptions{ExpiresAt: &expiresAt})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if generated.Metadata.ExpiresAt == nil {
		t.Fatal("esperava ExpiresAt definido após GenerateKey")
	}

	var nilTime *time.Time
	updated, err := svc.UpdateMetadata(generated.Metadata.Fingerprint, KeyMetadataPatch{
		ExpiresAt: &nilTime,
	})
	if err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	if updated.Metadata.ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, esperado nil após limpar", updated.Metadata.ExpiresAt)
	}
}

func TestKeyService_UpdateMetadata_UnknownFingerprintReturnsErrKeyNotFound(t *testing.T) {
	svc := newTestKeyService(t)
	label := "x"
	_, err := svc.UpdateMetadata("SHA256:desconhecida", KeyMetadataPatch{Label: &label})
	if err != ErrKeyNotFound {
		t.Fatalf("erro = %v, esperado ErrKeyNotFound", err)
	}
}

func TestKeyService_RegisterKey_AdoptsUnregisteredDiskPair(t *testing.T) {
	svc := newTestKeyService(t)
	dir, err := svc.keyDir()
	if err != nil {
		t.Fatalf("keyDir: %v", err)
	}
	gen := genTestKeyPair(t, dir, "id_ed25519")

	keysBefore, err := svc.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keysBefore) != 1 || keysBefore[0].Status != KeyStatusUnregistered {
		t.Fatalf("esperava 1 chave KeyStatusUnregistered antes do registro, obteve %+v", keysBefore)
	}

	label := "adotada"
	registered, err := svc.RegisterKey(gen.privateKeyPath, KeyMetadataPatch{Label: &label})
	if err != nil {
		t.Fatalf("RegisterKey: %v", err)
	}
	if registered.Status != KeyStatusOK {
		t.Errorf("Status = %v, esperado KeyStatusOK", registered.Status)
	}
	if registered.Metadata.Label != label {
		t.Errorf("Label = %q, esperado %q", registered.Metadata.Label, label)
	}

	if _, err := svc.RegisterKey(gen.privateKeyPath, KeyMetadataPatch{}); err == nil {
		t.Error("esperava erro ao registrar novamente uma chave já registrada")
	}
}

func TestKeyService_Unregister_RemovesOnlyMetadata(t *testing.T) {
	svc := newTestKeyService(t)
	generated, err := svc.GenerateKey(GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if err := svc.Unregister(generated.Metadata.Fingerprint); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	keys, err := svc.ListKeys()
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 1 || keys[0].Status != KeyStatusUnregistered {
		t.Fatalf("esperava a chave ainda em disco como KeyStatusUnregistered, obteve %+v", keys)
	}

	if err := svc.Unregister(generated.Metadata.Fingerprint); err != ErrKeyNotFound {
		t.Fatalf("segundo Unregister deveria retornar ErrKeyNotFound, obteve %v", err)
	}
}

func TestKeyService_Settings_DefaultsWhenNoFile(t *testing.T) {
	svc := newTestKeyService(t)
	settings, err := svc.Settings()
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if settings.AlertThresholdDays != defaultAlertThresholdDays || settings.DefaultAlgorithm != defaultKeyAlgorithm {
		t.Errorf("Settings = %+v, esperado defaults", settings)
	}
}

func TestKeyService_UpdateSettings_Persists(t *testing.T) {
	svc := newTestKeyService(t)
	newSettings := AppSettings{AlertThresholdDays: 90, DefaultAlgorithm: AlgorithmRSA}

	if err := svc.UpdateSettings(newSettings); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	got, err := svc.Settings()
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if got != newSettings {
		t.Errorf("Settings = %+v, esperado %+v", got, newSettings)
	}

	// GenerateKey sem Algorithm explícito deve passar a usar o novo default.
	generated, err := svc.GenerateKey(GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if generated.Metadata.Algorithm != AlgorithmRSA {
		t.Errorf("Algorithm = %q, esperado %q (novo default)", generated.Metadata.Algorithm, AlgorithmRSA)
	}
}

// ptrToPtr é um helper de teste para construir o **time.Time exigido por
// KeyMetadataPatch.ExpiresAt a partir de um *time.Time.
func ptrToPtr(t *time.Time) **time.Time {
	return &t
}
