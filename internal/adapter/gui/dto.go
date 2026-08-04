package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/felipehrs/keyward/core"
)

// dateLayout é o formato usado nos DTOs pra campos de data "sem hora" (ex.
// ExpiresAt em formulários) — mesmo formato usado por cmd/cli e cmd/tui,
// mantido idêntico por consistência de UX entre as três interfaces.
const dateLayout = "2006-01-02"

// formatTime converte um *time.Time do core pra string RFC3339 em UTC —
// nil vira nil no DTO (campo ausente do JSON via `omitempty`), nunca
// string vazia. Centralizado aqui pra manter a representação de tempo
// consistente em todo DTO que atravessa a fronteira Go->JS.
func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// mapErr embute um prefixo estável no único erro sentinela exportado pelo
// core (core.ErrKeyNotFound), pra o frontend poder branchar por código sem
// depender de comparação de texto. Os demais erros do core (sem sentinela)
// atravessam como estão — já são texto pt-BR exibível direto, e o runtime
// do Wails os entrega ao JS como RuntimeError.message (confirmado no spike
// da Etapa 0 inspecionando @wailsio/runtime).
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, core.ErrKeyNotFound) {
		return fmt.Errorf("[key_not_found] %w", err)
	}
	return err
}
