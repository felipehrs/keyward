// Command cli é o ponto de entrada da interface de linha de comando do keyward.
package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/felipehrs/keyward/core"
	"github.com/felipehrs/keyward/internal/sshagent"
)

// version é preenchido em tempo de build via `-ldflags "-X main.version=..."`
// (ver .goreleaser.yaml) — "dev" cobre `go run`/`go build` sem essa flag.
var version = "dev"

func main() {
	keySvc := core.NewFileKeyService("", "")
	if runtime.GOOS == "windows" {
		keySvc.AgentDial = sshagent.Dial
	}
	configSvc := core.NewFileConfigService("")
	backupSvc := core.NewFileBackupService("", "", "")

	root := newRootCmd(keySvc, configSvc, backupSvc)
	root.Version = version
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
