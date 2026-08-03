package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMetadataStore_LoadMissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.json")
	store := newMetadataStore(path)

	mf, err := store.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(mf.Keys) != 0 {
		t.Errorf("esperava Keys vazio, obteve %+v", mf.Keys)
	}
	if mf.Settings.AlertThresholdDays != defaultAlertThresholdDays {
		t.Errorf("AlertThresholdDays = %d, esperado %d", mf.Settings.AlertThresholdDays, defaultAlertThresholdDays)
	}
	if mf.Settings.DefaultAlgorithm != defaultKeyAlgorithm {
		t.Errorf("DefaultAlgorithm = %q, esperado %q", mf.Settings.DefaultAlgorithm, defaultKeyAlgorithm)
	}
}

func TestMetadataStore_SaveThenLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keyward", "metadata.json")
	store := newMetadataStore(path)

	expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	original := metadataFile{
		Keys: []KeyMetadata{
			{
				ID:          "id-1",
				Fingerprint: "SHA256:abc",
				KeyPath:     "/home/user/.ssh/id_ed25519",
				Label:       "GitHub pessoal",
				Algorithm:   AlgorithmEd25519,
				CreatedAt:   time.Now().UTC().Truncate(time.Second),
				ExpiresAt:   &expiresAt,
				Notes:       "gerada para teste",
			},
		},
		Settings: AppSettings{AlertThresholdDays: 45, DefaultAlgorithm: AlgorithmRSA},
	}

	if err := store.save(original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(loaded.Keys) != 1 {
		t.Fatalf("esperava 1 KeyMetadata, obteve %d", len(loaded.Keys))
	}
	got := loaded.Keys[0]
	want := original.Keys[0]
	if got.ID != want.ID || got.Fingerprint != want.Fingerprint || got.KeyPath != want.KeyPath ||
		got.Label != want.Label || got.Algorithm != want.Algorithm || got.Notes != want.Notes {
		t.Errorf("KeyMetadata não bateu no roundtrip: got=%+v want=%+v", got, want)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt = %v, esperado %v", got.CreatedAt, want.CreatedAt)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(*want.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, esperado %v", got.ExpiresAt, want.ExpiresAt)
	}
	if loaded.Settings != original.Settings {
		t.Errorf("Settings = %+v, esperado %+v", loaded.Settings, original.Settings)
	}
}

func TestMetadataStore_SaveLeavesNoStrayTempFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	store := newMetadataStore(filepath.Join(dir, "metadata.json"))

	if err := store.save(metadataFile{Settings: defaultAppSettings()}); err != nil {
		t.Fatalf("save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "metadata.json" {
		t.Fatalf("esperava só metadata.json em %s, encontrou %+v", dir, entries)
	}
}

func TestMetadataStore_LoadCorruptJSONReturnsWrappedError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store := newMetadataStore(path)
	_, err := store.load()
	if err == nil {
		t.Fatal("esperava erro ao carregar JSON corrompido, obteve nil")
	}
}
