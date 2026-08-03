package main

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// newTestServices constrói os três File*Service reais sobre um
// t.TempDir() isolado — mesmo padrão usado em core/keys_test.go. Preferido
// a mocks hand-rolled das interfaces: mais barato de manter e testa a
// integração de verdade.
func newTestServices(t *testing.T) (core.ConfigService, core.KeyService, core.BackupService) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ssh", "config")
	keyDir := filepath.Join(dir, "ssh")
	metadataPath := filepath.Join(dir, "metadata.json")

	keySvc := core.NewFileKeyService(keyDir, metadataPath)
	configSvc := core.NewFileConfigService(configPath)
	backupSvc := core.NewFileBackupService(configPath, keyDir, metadataPath)
	return configSvc, keySvc, backupSvc
}

// resolveMsgs executa cmd e desdobra recursivamente o resultado caso seja
// um tea.BatchMsg, até só sobrarem mensagens concretas. Necessário porque
// chamar um tea.Cmd retornado por uma tela não garante uma mensagem
// "pura" — o root frequentemente combina o resultado real com o tick do
// spinner via tea.Batch (ver startAsyncMsg em model.go).
func resolveMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	return flattenMsg(cmd())
}

func flattenMsg(msg tea.Msg) []tea.Msg {
	switch m := msg.(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range m {
			out = append(out, resolveMsgs(c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

// findAsyncResult procura a primeira asyncResultMsg dentre msgs.
func findAsyncResult(msgs []tea.Msg) (asyncResultMsg, bool) {
	for _, msg := range msgs {
		if r, ok := msg.(asyncResultMsg); ok {
			return r, true
		}
	}
	return asyncResultMsg{}, false
}
