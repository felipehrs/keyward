package core

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBackupManifest_JSONRoundTrip(t *testing.T) {
	expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	settings := AppSettings{AlertThresholdDays: 45, DefaultAlgorithm: AlgorithmRSA}

	original := backupManifest{
		Version:   backupManifestVersion,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Hosts: []backupHost{
			{
				Patterns:     []string{"bastion"},
				SourceFile:   "~/.ssh/config",
				HostName:     "203.0.113.10",
				IdentityFile: []string{"~/.ssh/id_ed25519"},
			},
		},
		Keys: []backupKey{
			{
				Metadata: KeyMetadata{
					ID:          "id-1",
					Fingerprint: "SHA256:abc",
					Algorithm:   AlgorithmEd25519,
					CreatedAt:   time.Now().UTC().Truncate(time.Second),
					ExpiresAt:   &expiresAt,
				},
				FileName:      "id_ed25519",
				HasPrivateKey: true,
			},
		},
		Settings: &settings,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got backupManifest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Version != original.Version || len(got.Hosts) != 1 || len(got.Keys) != 1 {
		t.Fatalf("roundtrip incompleto: %+v", got)
	}
	if got.Hosts[0].SourceFile != "~/.ssh/config" {
		t.Errorf("SourceFile = %q", got.Hosts[0].SourceFile)
	}
	if got.Keys[0].Metadata.Fingerprint != "SHA256:abc" {
		t.Errorf("Fingerprint = %q", got.Keys[0].Metadata.Fingerprint)
	}
	if got.Settings == nil || got.Settings.AlertThresholdDays != 45 {
		t.Errorf("Settings = %+v", got.Settings)
	}
}
