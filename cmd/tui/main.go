// Command tui é o ponto de entrada da interface de terminal interativa do
// keyward.
package main

import (
	"fmt"
	"os"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/felipehrs/keyward/core"
	"github.com/felipehrs/keyward/internal/sshagent"
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
	if runtime.GOOS == "windows" {
		// No Windows, ssh-agent (inclusive o do OpenSSH e o do 1Password) é
		// acessado via named pipe, não via Unix domain socket — o dial
		// default de FileKeyService só cobre Unix. Ver internal/sshagent.
		keySvc.AgentDial = sshagent.Dial
	}
	configSvc := core.NewFileConfigService(configPath)
	backupSvc := core.NewFileBackupService(configPath, keyDir, metadataPath)

	m := newRootModel(keySvc, configSvc, backupSvc)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
