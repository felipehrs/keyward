// Command cli é o ponto de entrada da interface de linha de comando do keyward.
package main

import (
	"fmt"
	"os"

	"github.com/felipehrs/keyward/core"
)

func main() {
	keySvc := core.NewFileKeyService("", "")
	configSvc := core.NewFileConfigService("")
	backupSvc := core.NewFileBackupService("", "", "")

	root := newRootCmd(keySvc, configSvc, backupSvc)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
