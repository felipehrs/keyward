package main

import (
	"github.com/spf13/cobra"

	"github.com/felipehrs/keyward/core"
)

func newRootCmd(keySvc core.KeyService, configSvc core.ConfigService, backupSvc core.BackupService) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "keyward",
		Short:         "keyward — gerencia hosts e chaves SSH (~/.ssh/config, ~/.ssh/)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(
		newKeyCmd(keySvc),
		newHostCmd(configSvc, keySvc),
		newBackupCmd(backupSvc, keySvc, configSvc),
	)
	return cmd
}
