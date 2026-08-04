package main

import (
	"path/filepath"
	"testing"

	"github.com/felipehrs/keyward/core"
)

// newTestApp constrói um *App sobre os três File*Service reais, rodando
// num t.TempDir() isolado — mesmo padrão usado em cmd/tui/testhelpers_test.go
// e core/keys_test.go, sem mocks hand-rolled das interfaces.
func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ssh", "config")
	keyDir := filepath.Join(dir, "ssh")
	metadataPath := filepath.Join(dir, "metadata.json")

	configSvc := core.NewFileConfigService(configPath)
	keySvc := core.NewFileKeyService(keyDir, metadataPath)
	backupSvc := core.NewFileBackupService(configPath, keyDir, metadataPath)
	return newApp(configSvc, keySvc, backupSvc)
}
