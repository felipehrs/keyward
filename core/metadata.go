package core

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Algorithm identifica o algoritmo de uma chave SSH suportado pelo keyward.
type Algorithm string

const (
	AlgorithmEd25519 Algorithm = "ed25519"
	AlgorithmRSA     Algorithm = "rsa"
)

// defaultAlertThresholdDays e defaultAlgorithm são os valores de AppSettings
// usados quando metadata.json ainda não existe (spec seção 4.2).
const (
	defaultAlertThresholdDays = 30
	defaultKeyAlgorithm       = AlgorithmEd25519
)

// KeyMetadata é o registro persistido em metadata.json para uma chave SSH
// (spec seção 4.2). O vínculo com a chave real em disco é feito por
// Fingerprint, não por KeyPath — sobrevive a renomear/mover o arquivo.
type KeyMetadata struct {
	ID          string     `json:"id"`
	Fingerprint string     `json:"fingerprint"`
	KeyPath     string     `json:"keyPath"`
	Label       string     `json:"label,omitempty"`
	Algorithm   Algorithm  `json:"algorithm"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	Notes       string     `json:"notes,omitempty"`
}

// AppSettings mapeia a entidade homônima da spec (seção 4.2).
type AppSettings struct {
	AlertThresholdDays int       `json:"alertThresholdDays"`
	DefaultAlgorithm   Algorithm `json:"defaultAlgorithm"`
}

func defaultAppSettings() AppSettings {
	return AppSettings{
		AlertThresholdDays: defaultAlertThresholdDays,
		DefaultAlgorithm:   defaultKeyAlgorithm,
	}
}

// metadataSchemaVersion identifica o formato de metadataFile. Não faz parte
// do schema documentado na spec (que só descreve KeyMetadata/AppSettings) —
// é um detalhe do wrapper de persistência, para viabilizar migrações futuras
// sem custo extra agora.
const metadataSchemaVersion = 1

// metadataFile é o documento raiz de metadata.json.
type metadataFile struct {
	Version  int           `json:"version"`
	Keys     []KeyMetadata `json:"keys"`
	Settings AppSettings   `json:"settings"`
}

// metadataStore encapsula leitura/escrita de metadata.json.
//
// Sem lock de concorrência entre processos (CLI/TUI/GUI podem rodar ao mesmo
// tempo) — decisão consciente de adiar essa proteção no MVP, documentada na
// spec. O uso típico é uma interface por vez.
type metadataStore struct {
	path string
}

func newMetadataStore(path string) *metadataStore {
	return &metadataStore{path: path}
}

// load lê metadata.json. Se o arquivo não existir, retorna um metadataFile
// vazio com Settings preenchido pelos defaults, sem erro — mesmo padrão de
// "arquivo ausente = estado vazio" usado em FileConfigService.ListHosts.
func (s *metadataStore) load() (metadataFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return metadataFile{Version: metadataSchemaVersion, Settings: defaultAppSettings()}, nil
		}
		return metadataFile{}, fmt.Errorf("lendo %s: %w", s.path, err)
	}

	var mf metadataFile
	if err := json.Unmarshal(data, &mf); err != nil {
		return metadataFile{}, fmt.Errorf("interpretando %s: %w", s.path, err)
	}
	if mf.Settings == (AppSettings{}) {
		mf.Settings = defaultAppSettings()
	}
	return mf, nil
}

// save grava data de forma atômica via atomicWriteFile (core/atomicfile.go),
// serializando em JSON indentado antes de escrever.
func (s *metadataStore) save(data metadataFile) error {
	data.Version = metadataSchemaVersion

	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando metadata: %w", err)
	}

	return atomicWriteFile(s.path, encoded, 0o600)
}
