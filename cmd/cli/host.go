package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/felipehrs/keyward/core"
)

func newHostCmd(configSvc core.ConfigService) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Gerencia os hosts configurados em ~/.ssh/config",
	}

	cmd.AddCommand(
		newHostListCmd(configSvc),
		newHostAddCmd(configSvc),
		newHostReplaceCmd(configSvc),
	)
	return cmd
}

func newHostListCmd(configSvc core.ConfigService) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lista os hosts configurados (arquivo principal e Includes)",
		RunE: func(cmd *cobra.Command, args []string) error {
			hosts, err := configSvc.ListHosts()
			if err != nil {
				return err
			}

			w := newTableWriter(cmd.OutOrStdout())
			fmt.Fprintln(w, "PATTERNS\tHOSTNAME\tUSER\tPORT\tIDENTITYFILE\tSOURCE")
			for _, h := range hosts {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					strings.Join(h.Patterns, ","),
					firstNonEmpty(h.HostName),
					firstNonEmpty(h.User),
					firstNonEmpty(h.Port),
					firstNonEmpty(strings.Join(h.IdentityFile, ",")),
					h.SourceFile,
				)
			}
			return w.Flush()
		},
	}
}

func newHostAddCmd(configSvc core.ConfigService) *cobra.Command {
	var (
		hostName     string
		user         string
		port         string
		identityFile []string
		file         string
	)

	cmd := &cobra.Command{
		Use:   "add <pattern> [pattern...]",
		Short: "Adiciona um novo bloco Host",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := core.HostSpec{
				Patterns:     args,
				HostName:     hostName,
				User:         user,
				Port:         port,
				IdentityFile: identityFile,
			}
			if err := configSvc.AddHost(file, spec); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "host adicionado: %s\n", strings.Join(args, ","))
			return nil
		},
	}

	cmd.Flags().StringVar(&hostName, "host-name", "", "diretiva HostName")
	cmd.Flags().StringVar(&user, "user", "", "diretiva User")
	cmd.Flags().StringVar(&port, "port", "", "diretiva Port")
	cmd.Flags().StringArrayVar(&identityFile, "identity-file", nil, "diretiva IdentityFile (repetível)")
	cmd.Flags().StringVar(&file, "file", "", "arquivo de destino; vazio usa ~/.ssh/config")

	return cmd
}

func newHostReplaceCmd(configSvc core.ConfigService) *cobra.Command {
	var (
		newPatterns  []string
		hostName     string
		user         string
		port         string
		identityFile []string
		file         string
	)

	cmd := &cobra.Command{
		Use:   "replace <old-pattern> [old-pattern...]",
		Short: "Substitui integralmente um bloco Host existente",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			patterns := newPatterns
			if len(patterns) == 0 {
				patterns = args
			}
			spec := core.HostSpec{
				Patterns:     patterns,
				HostName:     hostName,
				User:         user,
				Port:         port,
				IdentityFile: identityFile,
			}
			if err := configSvc.ReplaceHost(file, args, spec); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "host substituído: %s\n", strings.Join(args, ","))
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&newPatterns, "pattern", nil, "novo conjunto de patterns (repetível); vazio mantém os patterns antigos")
	cmd.Flags().StringVar(&hostName, "host-name", "", "diretiva HostName")
	cmd.Flags().StringVar(&user, "user", "", "diretiva User")
	cmd.Flags().StringVar(&port, "port", "", "diretiva Port")
	cmd.Flags().StringArrayVar(&identityFile, "identity-file", nil, "diretiva IdentityFile (repetível)")
	cmd.Flags().StringVar(&file, "file", "", "arquivo de destino; vazio usa ~/.ssh/config")

	return cmd
}
