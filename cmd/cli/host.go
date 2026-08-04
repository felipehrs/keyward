package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/felipehrs/keyward/core"
)

func newHostCmd(configSvc core.ConfigService, keySvc core.KeyService) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Gerencia os hosts configurados em ~/.ssh/config",
	}

	cmd.AddCommand(
		newHostListCmd(configSvc),
		newHostAddCmd(configSvc),
		newHostReplaceCmd(configSvc),
		newHostLinkKeyCmd(keySvc),
		newHostUnlinkKeyCmd(keySvc),
		newHostListKeyLinksCmd(keySvc),
	)
	return cmd
}

// hostKeyFromPatterns aplica a convenção de HostKey definida por
// core.HostMetadata (opaco para o core, calculado pelo chamador):
// strings.Join(Patterns, "\x00"). Fica aqui, e não em core, porque cada
// interface (CLI/TUI/GUI) decide como o usuário identifica um host —
// aqui, pelos mesmos patterns aceitos por `host add`/`host replace`.
func hostKeyFromPatterns(patterns []string) string {
	return strings.Join(patterns, "\x00")
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

func newHostLinkKeyCmd(keySvc core.KeyService) *cobra.Command {
	var (
		fingerprint string
		notes       string
	)

	cmd := &cobra.Command{
		Use:   "link-key <pattern> [pattern...]",
		Short: "Vincula um host a uma identidade de ssh-agent (--fingerprint)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if fingerprint == "" {
				return fmt.Errorf("--fingerprint é obrigatório")
			}
			if err := keySvc.LinkHostKey(hostKeyFromPatterns(args), fingerprint, notes); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "vínculo criado: %s -> %s\n", strings.Join(args, ","), fingerprint)
			return nil
		},
	}

	cmd.Flags().StringVar(&fingerprint, "fingerprint", "", "fingerprint da chave de agente a vincular (obrigatório)")
	cmd.Flags().StringVar(&notes, "notes", "", "notas livres sobre o vínculo")
	return cmd
}

func newHostUnlinkKeyCmd(keySvc core.KeyService) *cobra.Command {
	var fingerprint string

	cmd := &cobra.Command{
		Use:   "unlink-key <pattern> [pattern...]",
		Short: "Remove o vínculo entre um host e uma identidade de ssh-agent (--fingerprint)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if fingerprint == "" {
				return fmt.Errorf("--fingerprint é obrigatório")
			}
			if err := keySvc.UnlinkHostKey(hostKeyFromPatterns(args), fingerprint); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "vínculo removido: %s -> %s\n", strings.Join(args, ","), fingerprint)
			return nil
		},
	}

	cmd.Flags().StringVar(&fingerprint, "fingerprint", "", "fingerprint da chave de agente a desvincular (obrigatório)")
	return cmd
}

func newHostListKeyLinksCmd(keySvc core.KeyService) *cobra.Command {
	return &cobra.Command{
		Use:   "list-key-links",
		Short: "Lista os vínculos host/chave-de-agente persistidos",
		RunE: func(cmd *cobra.Command, args []string) error {
			links, err := keySvc.ListHostLinks()
			if err != nil {
				return err
			}

			w := newTableWriter(cmd.OutOrStdout())
			fmt.Fprintln(w, "HOST\tAGENT-FINGERPRINT\tNOTES")
			for _, l := range links {
				host := strings.ReplaceAll(l.HostKey, "\x00", ",")
				fmt.Fprintf(w, "%s\t%s\t%s\n", host, l.AgentKeyFingerprint, firstNonEmpty(l.Notes))
			}
			return w.Flush()
		},
	}
}
