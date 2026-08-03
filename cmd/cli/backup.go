package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"hrs.dev.br/keyward/core"
)

func newBackupCmd(backupSvc core.BackupService, keySvc core.KeyService, configSvc core.ConfigService) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Exporta e importa pacotes de backup (config + chaves + metadata)",
	}

	cmd.AddCommand(
		newBackupExportCmd(backupSvc, keySvc, configSvc),
		newBackupPreviewImportCmd(backupSvc),
		newBackupImportCmd(backupSvc),
	)
	return cmd
}

func newBackupExportCmd(backupSvc core.BackupService, keySvc core.KeyService, configSvc core.ConfigService) *cobra.Command {
	var (
		hostSelectors   []string
		allHosts        bool
		keys            []string
		keysWithPrivate []string
		allKeys         bool
		includeSettings bool
	)

	cmd := &cobra.Command{
		Use:   "export <dest-path>",
		Short: "Exporta hosts e chaves selecionados para um pacote .tar.gz",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hosts, err := configSvc.ListHosts()
			if err != nil {
				return err
			}

			var selectedHosts []core.Host
			if allHosts {
				selectedHosts = hosts
			} else if len(hostSelectors) > 0 {
				selectedHosts, err = resolveHostSelectors(hosts, hostSelectors)
				if err != nil {
					return err
				}
			}

			var selections []core.KeySelection
			if allKeys {
				all, err := keySvc.ListKeys()
				if err != nil {
					return err
				}
				for _, k := range all {
					selections = append(selections, core.KeySelection{Fingerprint: k.Metadata.Fingerprint, IncludePrivate: false})
				}
			}
			for _, fp := range keys {
				selections = append(selections, core.KeySelection{Fingerprint: fp, IncludePrivate: false})
			}
			for _, fp := range keysWithPrivate {
				selections = append(selections, core.KeySelection{Fingerprint: fp, IncludePrivate: true})
			}

			opts := core.ExportOptions{
				Hosts:           selectedHosts,
				Keys:            selections,
				IncludeSettings: includeSettings,
			}
			if err := backupSvc.Export(args[0], opts); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "backup exportado para %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&hostSelectors, "host", nil, "patterns de host a incluir, no formato exibido por 'host list' (repetível)")
	cmd.Flags().BoolVar(&allHosts, "all-hosts", false, "inclui todos os hosts")
	cmd.Flags().StringArrayVar(&keys, "key", nil, "fingerprint de chave a incluir, só metadata (repetível)")
	cmd.Flags().StringArrayVar(&keysWithPrivate, "key-with-private", nil, "fingerprint de chave a incluir com material privado em texto claro (repetível)")
	cmd.Flags().BoolVar(&allKeys, "all-keys", false, "inclui todas as chaves (só metadata, nunca material privado implicitamente)")
	cmd.Flags().BoolVar(&includeSettings, "include-settings", false, "inclui um snapshot de AppSettings no pacote")

	return cmd
}

// resolveHostSelectors casa cada seletor (patterns separados por vírgula,
// mesmo formato exibido pela coluna PATTERNS de 'host list') contra os hosts
// retornados por ListHosts.
func resolveHostSelectors(hosts []core.Host, selectors []string) ([]core.Host, error) {
	byPatterns := make(map[string]core.Host, len(hosts))
	for _, h := range hosts {
		byPatterns[strings.Join(h.Patterns, ",")] = h
	}

	var selected []core.Host
	for _, sel := range selectors {
		h, ok := byPatterns[sel]
		if !ok {
			return nil, fmt.Errorf("--host %q não corresponde a nenhum host de 'keyward host list'", sel)
		}
		selected = append(selected, h)
	}
	return selected, nil
}

func newBackupPreviewImportCmd(backupSvc core.BackupService) *cobra.Command {
	return &cobra.Command{
		Use:   "preview-import <src-path>",
		Short: "Mostra o que 'backup import' faria, sem escrever nada",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			preview, err := backupSvc.PreviewImport(args[0])
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "hosts a adicionar: %d, sem alteração: %d, com conflito: %d\n",
				len(preview.HostsToAdd), len(preview.HostsUnchanged), len(preview.HostConflicts))
			for _, c := range preview.HostConflicts {
				fmt.Fprintf(out, "  conflito de host [%s]: %s (destino %s, externo=%v)\n",
					c.Key, strings.Join(c.Host.Patterns, ","), c.DestinationFile, c.ExternalPath)
			}

			fmt.Fprintf(out, "chaves a adicionar: %d, sem alteração: %d, com conflito: %d\n",
				len(preview.KeysToAdd), len(preview.KeysUnchanged), len(preview.KeyConflicts))
			for _, c := range preview.KeyConflicts {
				fmt.Fprintf(out, "  conflito de chave [%s]: kind=%v destino=%s\n", c.Key, c.Kind, c.DestinationPath)
			}

			if preview.Settings != nil {
				fmt.Fprintln(out, "pacote inclui um snapshot de AppSettings")
			}
			return nil
		},
	}
}

func newBackupImportCmd(backupSvc core.BackupService) *cobra.Command {
	var (
		onHostConflict string
		onKeyConflict  string
	)

	cmd := &cobra.Command{
		Use:   "import <src-path>",
		Short: "Importa um pacote de backup, aplicando uma política de resolução de conflitos",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hostResolution, err := parseHostConflictPolicy(onHostConflict)
			if err != nil {
				return err
			}
			keyResolution, err := parseKeyConflictPolicy(onKeyConflict)
			if err != nil {
				return err
			}

			preview, err := backupSvc.PreviewImport(args[0])
			if err != nil {
				return err
			}

			resolutions := core.ImportResolutions{
				Hosts: make(map[string]core.HostResolution, len(preview.HostConflicts)),
				Keys:  make(map[string]core.KeyResolution, len(preview.KeyConflicts)),
			}
			for _, c := range preview.HostConflicts {
				resolutions.Hosts[c.Key] = hostResolution
			}
			for _, c := range preview.KeyConflicts {
				resolutions.Keys[c.Key] = keyResolution
			}

			result, err := backupSvc.Import(args[0], resolutions)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "hosts: %d adicionados, %d substituídos, %d sem alteração, %d ignorados\n",
				len(result.HostsAdded), len(result.HostsReplaced), len(result.HostsUnchanged), len(result.HostsSkipped))
			fmt.Fprintf(out, "chaves: %d adicionadas, %d sem alteração, %d metadata atualizada, %d arquivo sobrescrito, %d renomeadas, %d ignoradas\n",
				len(result.KeysAdded), len(result.KeysUnchanged), len(result.KeysMetadataUpdated),
				len(result.KeysFileOverwritten), len(result.KeysImportedRenamed), len(result.KeysSkipped))

			if len(result.Errors) > 0 {
				for _, e := range result.Errors {
					fmt.Fprintf(cmd.ErrOrStderr(), "erro: %v\n", e)
				}
				return fmt.Errorf("import concluído com %d erro(s)", len(result.Errors))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&onHostConflict, "on-host-conflict", "skip", "política para conflitos de host: skip|apply")
	cmd.Flags().StringVar(&onKeyConflict, "on-key-conflict", "skip", "política para conflitos de chave: skip|overwrite|rename (rename só é válido para colisão de nome de arquivo; para chave já registrada é tratado como skip)")

	return cmd
}

func parseHostConflictPolicy(s string) (core.HostResolution, error) {
	switch s {
	case "skip":
		return core.HostResolutionSkip, nil
	case "apply":
		return core.HostResolutionApply, nil
	default:
		return 0, fmt.Errorf("--on-host-conflict inválido: %q (use skip|apply)", s)
	}
}

func parseKeyConflictPolicy(s string) (core.KeyResolution, error) {
	switch s {
	case "skip":
		return core.KeyResolutionSkip, nil
	case "overwrite":
		return core.KeyResolutionOverwrite, nil
	case "rename":
		return core.KeyResolutionImportRenamed, nil
	default:
		return 0, fmt.Errorf("--on-key-conflict inválido: %q (use skip|overwrite|rename)", s)
	}
}
