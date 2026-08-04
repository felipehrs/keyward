package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/felipehrs/keyward/core"
)

func newKeyCmd(keySvc core.KeyService) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Gerencia chaves SSH e sua metadata (expiração, rótulo, notas)",
	}

	cmd.AddCommand(
		newKeyGenerateCmd(keySvc),
		newKeyListCmd(keySvc),
		newKeyGetCmd(keySvc),
		newKeyUpdateCmd(keySvc),
		newKeyRegisterCmd(keySvc),
		newKeyRegisterAgentCmd(keySvc),
		newKeyUnregisterCmd(keySvc),
		newKeySettingsCmd(keySvc),
	)
	return cmd
}

func newKeyGenerateCmd(keySvc core.KeyService) *cobra.Command {
	var (
		algorithm       string
		rsaBits         int
		passphraseStdin bool
		comment         string
		fileName        string
		directory       string
		overwrite       bool
		label           string
		notes           string
		expiresAt       string
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Gera um novo par de chaves SSH",
		RunE: func(cmd *cobra.Command, args []string) error {
			expires, err := parseExpiresAt(expiresAt)
			if err != nil {
				return err
			}

			var passphrase []byte
			if passphraseStdin {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("lendo passphrase de stdin: %w", err)
				}
				passphrase = []byte(strings.TrimRight(string(data), "\r\n"))
			}

			key, err := keySvc.GenerateKey(core.GenerateKeyOptions{
				Algorithm:  core.Algorithm(algorithm),
				RSABits:    rsaBits,
				Passphrase: passphrase,
				Comment:    comment,
				FileName:   fileName,
				Overwrite:  overwrite,
				Directory:  directory,
				Label:      label,
				Notes:      notes,
				ExpiresAt:  expires,
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "chave gerada: %s (%s)\n", key.Metadata.Fingerprint, key.PrivateKeyPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&algorithm, "algorithm", "", "algoritmo da chave (ed25519|rsa); vazio usa o default configurado")
	cmd.Flags().IntVar(&rsaBits, "rsa-bits", 0, "bits da chave RSA (mínimo 4096); só relevante com --algorithm rsa")
	cmd.Flags().BoolVar(&passphraseStdin, "passphrase-stdin", false, "lê a passphrase de stdin em vez de gerar sem passphrase")
	cmd.Flags().StringVar(&comment, "comment", "", "comentário embutido no arquivo .pub")
	cmd.Flags().StringVar(&fileName, "file-name", "", "nome do arquivo de chave; vazio usa o default por algoritmo")
	cmd.Flags().StringVar(&directory, "directory", "", "diretório de destino; vazio usa ~/.ssh")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "sobrescreve arquivo existente com o mesmo nome")
	cmd.Flags().StringVar(&label, "label", "", "rótulo da chave na metadata")
	cmd.Flags().StringVar(&notes, "notes", "", "notas livres na metadata")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "data de expiração ("+dateLayout+")")

	return cmd
}

// keySourceFilter valores aceitos pela flag --source de `key list`.
const (
	keySourceFilterAll   = "all"
	keySourceFilterFile  = "file"
	keySourceFilterAgent = "agent"
)

func newKeyListCmd(keySvc core.KeyService) *cobra.Command {
	var source string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista as chaves conhecidas (arquivo e agente), ordenadas por proximidade de expiração",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch source {
			case keySourceFilterAll, keySourceFilterFile, keySourceFilterAgent:
			default:
				return fmt.Errorf("--source inválido: %q (use %q, %q ou %q)", source, keySourceFilterAll, keySourceFilterFile, keySourceFilterAgent)
			}

			keys, err := keySvc.ListKeys()
			if err != nil {
				return err
			}

			// Uma única tabela com coluna SOURCE, em vez de duas seções
			// separadas: segue a mesma convenção já usada por `host list`
			// (coluna SOURCE com a origem de cada linha) e mantém a chave
			// ordenada por proximidade de expiração vinda de ListKeys, sem
			// precisar particionar/reordenar por origem.
			w := newTableWriter(cmd.OutOrStdout())
			fmt.Fprintln(w, "FINGERPRINT\tALGO\tLABEL\tSTATUS\tEXPIRES\tSOURCE")
			attention := false
			for _, k := range keys {
				if !keyMatchesSourceFilter(k, source) {
					continue
				}
				status := keyStatusString(k.Status)
				if k.IsExpired {
					status = "EXPIRED"
					attention = true
				} else if k.IsExpiringSoon {
					status = "EXPIRING SOON"
					attention = true
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					k.Metadata.Fingerprint, k.Algorithm, firstNonEmpty(k.Metadata.Label), status, formatTime(k.Metadata.ExpiresAt), keySourceString(k))
			}
			if err := w.Flush(); err != nil {
				return err
			}

			if attention {
				fmt.Fprintln(cmd.ErrOrStderr(), "aviso: há chaves expiradas ou expirando em breve")
				os.Exit(1)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&source, "source", keySourceFilterAll, "filtra por origem da chave (all|file|agent)")
	return cmd
}

// keyMatchesSourceFilter aplica o filtro --source de `key list`.
func keyMatchesSourceFilter(k core.Key, filter string) bool {
	switch filter {
	case keySourceFilterFile:
		return k.Source == core.KeySourceFile
	case keySourceFilterAgent:
		return k.Source == core.KeySourceAgent
	default:
		return true
	}
}

// keySourceString formata a coluna SOURCE de `key list`, incluindo o nome do
// agente entre parênteses quando identificado (ex. "agent (1password)").
func keySourceString(k core.Key) string {
	if k.Source != core.KeySourceAgent {
		return "file"
	}
	if k.AgentName == "" {
		return "agent"
	}
	return fmt.Sprintf("agent (%s)", k.AgentName)
}

func keyStatusString(s core.KeyStatus) string {
	switch s {
	case core.KeyStatusOK:
		return "ok"
	case core.KeyStatusUnregistered:
		return "unregistered"
	case core.KeyStatusMissingFile:
		return "missing-file"
	case core.KeyStatusAgentOffline:
		return "agent-offline"
	default:
		return "unknown"
	}
}

func newKeyGetCmd(keySvc core.KeyService) *cobra.Command {
	return &cobra.Command{
		Use:   "get <fingerprint>",
		Short: "Mostra os detalhes de uma chave pelo fingerprint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := keySvc.GetKey(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Fingerprint:   %s\n", key.Metadata.Fingerprint)
			fmt.Fprintf(out, "Algoritmo:     %s (%d bits)\n", key.Algorithm, key.KeyBits)
			fmt.Fprintf(out, "Status:        %s\n", keyStatusString(key.Status))
			fmt.Fprintf(out, "Label:         %s\n", firstNonEmpty(key.Metadata.Label))
			fmt.Fprintf(out, "Notes:         %s\n", firstNonEmpty(key.Metadata.Notes))
			fmt.Fprintf(out, "Criada em:     %s\n", key.Metadata.CreatedAt.Format(dateLayout))
			fmt.Fprintf(out, "Expira em:     %s\n", formatTime(key.Metadata.ExpiresAt))
			fmt.Fprintf(out, "Expirada:      %v\n", key.IsExpired)
			fmt.Fprintf(out, "Expira em breve: %v\n", key.IsExpiringSoon)
			fmt.Fprintf(out, "Chave privada: %s\n", firstNonEmpty(key.PrivateKeyPath))
			fmt.Fprintf(out, "Chave pública: %s\n", firstNonEmpty(key.PublicKeyPath))
			fmt.Fprintf(out, "Comentário:    %s\n", firstNonEmpty(key.Comment))
			return nil
		},
	}
}

func newKeyUpdateCmd(keySvc core.KeyService) *cobra.Command {
	var (
		label          string
		notes          string
		expiresAt      string
		clearExpiresAt bool
	)

	cmd := &cobra.Command{
		Use:   "update <fingerprint>",
		Short: "Atualiza label, notes e/ou data de expiração de uma chave já registrada",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			patch := core.KeyMetadataPatch{}
			if cmd.Flags().Changed("label") {
				patch.Label = &label
			}
			if cmd.Flags().Changed("notes") {
				patch.Notes = &notes
			}
			switch {
			case clearExpiresAt:
				var nilExpiry *time.Time
				patch.ExpiresAt = &nilExpiry
			case cmd.Flags().Changed("expires-at"):
				t, err := parseExpiresAt(expiresAt)
				if err != nil {
					return err
				}
				patch.ExpiresAt = &t
			}

			key, err := keySvc.UpdateMetadata(args[0], patch)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "chave atualizada: %s\n", key.Metadata.Fingerprint)
			return nil
		},
	}

	cmd.Flags().StringVar(&label, "label", "", "novo rótulo")
	cmd.Flags().StringVar(&notes, "notes", "", "novas notas")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "nova data de expiração ("+dateLayout+")")
	cmd.Flags().BoolVar(&clearExpiresAt, "clear-expires-at", false, "remove a data de expiração")
	cmd.MarkFlagsMutuallyExclusive("expires-at", "clear-expires-at")

	return cmd
}

func newKeyRegisterCmd(keySvc core.KeyService) *cobra.Command {
	var (
		label     string
		notes     string
		expiresAt string
	)

	cmd := &cobra.Command{
		Use:   "register <private-key-path>",
		Short: "Registra na metadata uma chave já existente em disco, sem registro",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expires, err := parseExpiresAt(expiresAt)
			if err != nil {
				return err
			}
			patch := core.KeyMetadataPatch{}
			if cmd.Flags().Changed("label") {
				patch.Label = &label
			}
			if cmd.Flags().Changed("notes") {
				patch.Notes = &notes
			}
			if cmd.Flags().Changed("expires-at") {
				patch.ExpiresAt = &expires
			}

			key, err := keySvc.RegisterKey(args[0], patch)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "chave registrada: %s\n", key.Metadata.Fingerprint)
			return nil
		},
	}

	cmd.Flags().StringVar(&label, "label", "", "rótulo da chave")
	cmd.Flags().StringVar(&notes, "notes", "", "notas livres")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "data de expiração ("+dateLayout+")")

	return cmd
}

func newKeyRegisterAgentCmd(keySvc core.KeyService) *cobra.Command {
	var (
		label     string
		notes     string
		expiresAt string
	)

	cmd := &cobra.Command{
		Use:   "register-agent <fingerprint>",
		Short: "Registra na metadata uma identidade oferecida por um ssh-agent, sem registro",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expires, err := parseExpiresAt(expiresAt)
			if err != nil {
				return err
			}
			patch := core.KeyMetadataPatch{}
			if cmd.Flags().Changed("label") {
				patch.Label = &label
			}
			if cmd.Flags().Changed("notes") {
				patch.Notes = &notes
			}
			if cmd.Flags().Changed("expires-at") {
				patch.ExpiresAt = &expires
			}

			key, err := keySvc.RegisterAgentKey(args[0], patch)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "chave de agente registrada: %s\n", key.Metadata.Fingerprint)
			return nil
		},
	}

	cmd.Flags().StringVar(&label, "label", "", "rótulo da chave")
	cmd.Flags().StringVar(&notes, "notes", "", "notas livres")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "data de expiração ("+dateLayout+")")

	return cmd
}

func newKeyUnregisterCmd(keySvc core.KeyService) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "unregister <fingerprint>",
		Short: "Remove o registro de metadata de uma chave (não toca em arquivos em disco)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes && !confirm(fmt.Sprintf("remover o registro de metadata de %s?", args[0])) {
				fmt.Fprintln(cmd.OutOrStdout(), "cancelado")
				return nil
			}
			if err := keySvc.Unregister(args[0]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "registro removido")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "não pedir confirmação")
	return cmd
}

func newKeySettingsCmd(keySvc core.KeyService) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Consulta e altera as configurações do app (AppSettings)",
	}
	cmd.AddCommand(newKeySettingsShowCmd(keySvc), newKeySettingsSetCmd(keySvc))
	return cmd
}

func newKeySettingsShowCmd(keySvc core.KeyService) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Mostra as configurações atuais",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := keySvc.Settings()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "alertThresholdDays: %d\n", settings.AlertThresholdDays)
			fmt.Fprintf(out, "defaultAlgorithm:   %s\n", settings.DefaultAlgorithm)
			return nil
		},
	}
}

func newKeySettingsSetCmd(keySvc core.KeyService) *cobra.Command {
	var (
		alertThresholdDays int
		defaultAlgorithm   string
	)

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Atualiza as configurações do app",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("default-algorithm") {
				if defaultAlgorithm != string(core.AlgorithmEd25519) && defaultAlgorithm != string(core.AlgorithmRSA) {
					return fmt.Errorf("--default-algorithm inválido: %q (use %q ou %q)", defaultAlgorithm, core.AlgorithmEd25519, core.AlgorithmRSA)
				}
			}

			settings, err := keySvc.Settings()
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("alert-threshold-days") {
				settings.AlertThresholdDays = alertThresholdDays
			}
			if cmd.Flags().Changed("default-algorithm") {
				settings.DefaultAlgorithm = core.Algorithm(defaultAlgorithm)
			}

			if err := keySvc.UpdateSettings(settings); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "configurações atualizadas")
			return nil
		},
	}

	cmd.Flags().IntVar(&alertThresholdDays, "alert-threshold-days", 0, "dias antes da expiração para alertar")
	cmd.Flags().StringVar(&defaultAlgorithm, "default-algorithm", "", "algoritmo padrão (ed25519|rsa)")

	return cmd
}
