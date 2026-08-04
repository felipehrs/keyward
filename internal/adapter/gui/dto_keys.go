package main

import (
	"fmt"
	"time"

	"github.com/felipehrs/keyward/core"
)

// KeyStatusDTO traduz core.KeyStatus (int/iota) pra uma string nomeada —
// sem isso, o valor numérico não tem significado autoexplicativo no lado
// JS, e fica frágil a reordenação futura do iota no core (quebra
// silenciosa, sem erro de compilação).
type KeyStatusDTO string

const (
	KeyStatusOK           KeyStatusDTO = "ok"
	KeyStatusUnregistered KeyStatusDTO = "unregistered"
	KeyStatusMissingFile  KeyStatusDTO = "missingFile"
)

// keyStatusToDTO traduz por switch explícito — nunca cast numérico.
func keyStatusToDTO(s core.KeyStatus) KeyStatusDTO {
	switch s {
	case core.KeyStatusUnregistered:
		return KeyStatusUnregistered
	case core.KeyStatusMissingFile:
		return KeyStatusMissingFile
	default:
		return KeyStatusOK
	}
}

// KeyMetadataDTO espelha core.KeyMetadata.
type KeyMetadataDTO struct {
	ID          string  `json:"id"`
	Fingerprint string  `json:"fingerprint"`
	KeyPath     string  `json:"keyPath"`
	Label       string  `json:"label,omitempty"`
	Algorithm   string  `json:"algorithm"`
	CreatedAt   string  `json:"createdAt"`
	ExpiresAt   *string `json:"expiresAt,omitempty"`
	Notes       string  `json:"notes,omitempty"`
}

func keyMetadataToDTO(m core.KeyMetadata) KeyMetadataDTO {
	// Metadata é zero value quando Status == KeyStatusUnregistered (ver
	// core.Key) — nesse caso CreatedAt.IsZero() é true, e formatá-lo
	// produziria "0001-01-01T00:00:00Z", que pareceria uma data real em
	// vez de "sem registro". Fica vazio nesse caso, igual aos demais
	// campos de metadata.
	var createdAt string
	if !m.CreatedAt.IsZero() {
		if formatted := formatTime(&m.CreatedAt); formatted != nil {
			createdAt = *formatted
		}
	}
	return KeyMetadataDTO{
		ID:          m.ID,
		Fingerprint: m.Fingerprint,
		KeyPath:     m.KeyPath,
		Label:       m.Label,
		Algorithm:   string(m.Algorithm),
		CreatedAt:   createdAt,
		ExpiresAt:   formatTime(m.ExpiresAt),
		Notes:       m.Notes,
	}
}

// KeyDTO espelha core.Key — a visão consolidada de uma chave (disco +
// metadata reconciliados).
type KeyDTO struct {
	Metadata       KeyMetadataDTO `json:"metadata"`
	PublicKeyPath  string         `json:"publicKeyPath,omitempty"`
	PrivateKeyPath string         `json:"privateKeyPath,omitempty"`
	Algorithm      string         `json:"algorithm"`
	KeyBits        int            `json:"keyBits"`
	Comment        string         `json:"comment,omitempty"`
	Status         KeyStatusDTO   `json:"status"`
	IsExpired      bool           `json:"isExpired"`
	IsExpiringSoon bool           `json:"isExpiringSoon"`
}

func keyToDTO(k core.Key) KeyDTO {
	return KeyDTO{
		Metadata:       keyMetadataToDTO(k.Metadata),
		PublicKeyPath:  k.PublicKeyPath,
		PrivateKeyPath: k.PrivateKeyPath,
		Algorithm:      string(k.Algorithm),
		KeyBits:        k.KeyBits,
		Comment:        k.Comment,
		Status:         keyStatusToDTO(k.Status),
		IsExpired:      k.IsExpired,
		IsExpiringSoon: k.IsExpiringSoon,
	}
}

func keysToDTO(keys []core.Key) []KeyDTO {
	out := make([]KeyDTO, len(keys))
	for i, k := range keys {
		out[i] = keyToDTO(k)
	}
	return out
}

// parseDateOnly interpreta uma string "2006-01-02" (dateLayout) em UTC —
// formato de data "sem hora" usado nos formulários de expiração, mesmo
// formato usado por cmd/cli/cmd/tui.
func parseDateOnly(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, *s)
	if err != nil {
		return nil, fmt.Errorf("data inválida %q (esperado formato %s): %w", *s, dateLayout, err)
	}
	t = t.UTC()
	return &t, nil
}

// GenerateKeyInput é a entrada de GenerateKey. Passphrase é string (não
// []byte) — armadilha de serialização: []byte viraria base64 visível em
// devtools/logs do Wails, e nenhum método do core lê passphrase de volta
// (só escrita única no momento de gerar), então não há necessidade de
// round-trip.
type GenerateKeyInput struct {
	Algorithm  string  `json:"algorithm,omitempty"`
	RSABits    int     `json:"rsaBits,omitempty"`
	Passphrase string  `json:"passphrase,omitempty"`
	Comment    string  `json:"comment,omitempty"`
	FileName   string  `json:"fileName,omitempty"`
	Overwrite  bool    `json:"overwrite,omitempty"`
	Directory  string  `json:"directory,omitempty"`
	Label      string  `json:"label,omitempty"`
	Notes      string  `json:"notes,omitempty"`
	ExpiresAt  *string `json:"expiresAt,omitempty"`
}

func (in GenerateKeyInput) toCore() (core.GenerateKeyOptions, error) {
	expiresAt, err := parseDateOnly(in.ExpiresAt)
	if err != nil {
		return core.GenerateKeyOptions{}, err
	}
	opts := core.GenerateKeyOptions{
		Algorithm: core.Algorithm(in.Algorithm),
		RSABits:   in.RSABits,
		Comment:   in.Comment,
		FileName:  in.FileName,
		Overwrite: in.Overwrite,
		Directory: in.Directory,
		Label:     in.Label,
		Notes:     in.Notes,
		ExpiresAt: expiresAt,
	}
	if in.Passphrase != "" {
		opts.Passphrase = []byte(in.Passphrase)
	}
	return opts, nil
}

// RegisterKeyInput é a entrada de RegisterKey. Diferente de
// KeyMetadataPatchInput (Etapa 6, usado por UpdateKeyMetadata), não
// precisa da semântica de 3 estados (keep/clear/set) — é sempre um
// registro novo, então Label/Notes são sempre atribuídos (mesmo que
// vazios) e ExpiresAt só é definido quando informado.
type RegisterKeyInput struct {
	PrivateKeyPath string  `json:"privateKeyPath"`
	Label          string  `json:"label,omitempty"`
	Notes          string  `json:"notes,omitempty"`
	ExpiresAt      *string `json:"expiresAt,omitempty"`
}

func (in RegisterKeyInput) toCorePatch() (core.KeyMetadataPatch, error) {
	expiresAt, err := parseDateOnly(in.ExpiresAt)
	if err != nil {
		return core.KeyMetadataPatch{}, err
	}
	label := in.Label
	notes := in.Notes
	patch := core.KeyMetadataPatch{Label: &label, Notes: &notes}
	if expiresAt != nil {
		patch.ExpiresAt = &expiresAt
	}
	return patch, nil
}

// ExpiresAtAction resolve a armadilha de KeyMetadataPatch.ExpiresAt
// (**time.Time — nil = não altera, ponteiro-pra-nil = limpa, ponteiro-pra-
// valor = define): ponteiro duplo não atravessa JSON de forma sensata, e é
// reexpresso aqui como um enum de ação explícito de 3 estados.
type ExpiresAtAction string

const (
	ExpiresAtKeep  ExpiresAtAction = "keep"
	ExpiresAtClear ExpiresAtAction = "clear"
	ExpiresAtSet   ExpiresAtAction = "set"
)

// KeyMetadataPatchInput é a entrada de UpdateKeyMetadata.
type KeyMetadataPatchInput struct {
	Label          *string         `json:"label,omitempty"`
	Notes          *string         `json:"notes,omitempty"`
	ExpiresAt      ExpiresAtAction `json:"expiresAt,omitempty"`
	ExpiresAtValue *string         `json:"expiresAtValue,omitempty"`
}

func (in KeyMetadataPatchInput) toCore() (core.KeyMetadataPatch, error) {
	patch := core.KeyMetadataPatch{Label: in.Label, Notes: in.Notes}
	switch in.ExpiresAt {
	case "", ExpiresAtKeep:
		// patch.ExpiresAt fica nil — core não mexe na expiração atual.
	case ExpiresAtClear:
		var cleared *time.Time
		patch.ExpiresAt = &cleared
	case ExpiresAtSet:
		t, err := parseDateOnly(in.ExpiresAtValue)
		if err != nil {
			return core.KeyMetadataPatch{}, err
		}
		if t == nil {
			return core.KeyMetadataPatch{}, fmt.Errorf(`expiresAtValue é obrigatório quando expiresAt="set"`)
		}
		patch.ExpiresAt = &t
	default:
		return core.KeyMetadataPatch{}, fmt.Errorf("expiresAt inválido: %q", in.ExpiresAt)
	}
	return patch, nil
}
