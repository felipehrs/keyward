// Command cli é o ponto de entrada da interface de linha de comando do keyward.
package main

import (
	"fmt"
	"os"

	"github.com/felipehrs/keyward/core"
)

// version é preenchido em tempo de build via `-ldflags "-X main.version=..."`
// (ver .goreleaser.yaml) — "dev" cobre `go run`/`go build` sem essa flag.
var version = "dev"

func main() {
	keySvc := core.NewFileKeyService("", "")
	configSvc := core.NewFileConfigService("")
	backupSvc := core.NewFileBackupService("", "", "")

	root := newRootCmd(keySvc, configSvc, backupSvc)
	root.Version = version
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
