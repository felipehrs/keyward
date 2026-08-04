// Command tui é o ponto de entrada da interface de terminal interativa do
// keyward.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
)

// version é preenchido em tempo de build via `-ldflags "-X main.version=..."`
// (ver .goreleaser.yaml) — "dev" cobre `go run`/`go build` sem essa flag.
var version = "dev"

// As variáveis de ambiente KEYWARD_CONFIG/KEYWARD_KEY_DIR/KEYWARD_METADATA
// permitem apontar a TUI para um ~/.ssh e metadata.json de teste, sem
// mexer no ambiente real do usuário durante o desenvolvimento — os
// File*Service não expõem override de path via flag hoje (cmd/cli sempre
// usa os defaults), e apontar só HOME não isola tudo, já que o default de
// MetadataPath vem de os.UserConfigDir() (%AppData% no Windows). Vazias,
// cada uma cai no mesmo default que a CLI usa.
func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("keyward-tui " + version)
		return
	}

	configPath := os.Getenv("KEYWARD_CONFIG")
	keyDir := os.Getenv("KEYWARD_KEY_DIR")
	metadataPath := os.Getenv("KEYWARD_METADATA")

	keySvc := core.NewFileKeyService(keyDir, metadataPath)
	configSvc := core.NewFileConfigService(configPath)
	backupSvc := core.NewFileBackupService(configPath, keyDir, metadataPath)

	m := newRootModel(keySvc, configSvc, backupSvc)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
