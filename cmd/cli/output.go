package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

const dateLayout = "2006-01-02"

// newTableWriter cria um tabwriter configurado para as tabelas do CLI.
func newTableWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
}

// formatTime formata um horário no layout de data usado pelo CLI, ou "-" se nil.
func formatTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(dateLayout)
}

// parseExpiresAt interpreta uma data no formato YYYY-MM-DD (UTC, meia-noite).
func parseExpiresAt(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return nil, fmt.Errorf("data inválida %q (esperado formato %s): %w", s, dateLayout, err)
	}
	t = t.UTC()
	return &t, nil
}

// firstNonEmpty retorna s se não vazio, senão "-" — usado para colunas de tabela.
func firstNonEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// confirm pergunta ao usuário via stdin/stdout, retornando true só para "y"/"yes".
func confirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
