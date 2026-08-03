package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// ErrKeyNotFound é retornado quando um fingerprint não corresponde a
// nenhuma chave conhecida (nem em disco, nem em metadata.json).
var ErrKeyNotFound = errors.New("keyward: chave não encontrada")

// KeyStatus indica o resultado da reconciliação entre ~/.ssh/ e
// metadata.json para uma determinada chave (spec seção 5.3.2).
type KeyStatus int

const (
	// KeyStatusOK: par de arquivos em disco reconciliado com um registro de
	// metadata pelo fingerprint.
	KeyStatusOK KeyStatus = iota
	// KeyStatusUnregistered: par de arquivos encontrado em ~/.ssh/ sem
	// registro correspondente em metadata.json.
	KeyStatusUnregistered
	// KeyStatusMissingFile: registro em metadata.json cujo fingerprint não
	// corresponde a nenhum par de arquivos encontrado em disco.
	KeyStatusMissingFile
)

// Key é a visão consolidada de uma chave SSH, unindo o que foi encontrado em
// disco (~/.ssh/) com o registro de KeyMetadata correspondente, quando
// existir. É o tipo retornado por ListKeys/GetKey — nunca é serializado
// diretamente em metadata.json (isso é papel exclusivo de KeyMetadata).
type Key struct {
	Metadata KeyMetadata // zero value quando Status == KeyStatusUnregistered

	PublicKeyPath  string // "" se Status == KeyStatusMissingFile
	PrivateKeyPath string // "" se Status == KeyStatusMissingFile
	Algorithm      Algorithm
	KeyBits        int    // bits efetivos (RSA); 256 fixo para ed25519
	Comment        string // comentário embutido no arquivo .pub

	Status KeyStatus

	// Calculados a partir de AppSettings.AlertThresholdDays no momento do
	// ListKeys, para centralizar a regra de alerta no core (spec 5.3.4).
	IsExpired      bool
	IsExpiringSoon bool
}

// GenerateKeyOptions parametriza a geração de uma nova chave (spec 5.2).
type GenerateKeyOptions struct {
	Algorithm  Algorithm // "" -> usa AppSettings.DefaultAlgorithm
	RSABits    int       // só relevante se Algorithm == AlgorithmRSA; 0 -> 4096; erro se < 4096
	Passphrase []byte    // nil/vazio -> chave sem passphrase
	Comment    string    // "" -> gera um default (ex. "keyward@<hostname>")
	FileName   string    // "" -> nome default por algoritmo (id_ed25519, id_rsa)
	Overwrite  bool      // permite sobrescrever arquivo já existente com esse nome
	Directory  string    // "" -> ~/.ssh

	Label     string
	Notes     string
	ExpiresAt *time.Time
}

// KeyMetadataPatch representa uma edição parcial de KeyMetadata — campos nil
// não são alterados. ExpiresAt usa **time.Time para distinguir "não alterar"
// (nil) de "remover data de expiração" (ponteiro para nil).
type KeyMetadataPatch struct {
	Label     *string
	Notes     *string
	ExpiresAt **time.Time
}

// KeyService gera, cataloga e gerencia o ciclo de vida de chaves SSH,
// reconciliando o que existe fisicamente em ~/.ssh/ com a metadata própria
// do app (spec seções 5.2 e 5.3).
//
// Não há método de rotação dedicado: a UI compõe GenerateKey +
// UpdateMetadata (ex. anotando "substitui X" em Notes) — decisão registrada
// na spec para não estender o schema de KeyMetadata com um vínculo formal.
type KeyService interface {
	// GenerateKey cria um novo par de chaves (ed25519 por padrão, RSA >= 4096
	// bits quando solicitado), grava em disco com as permissões da seção 6,
	// e cria o registro de KeyMetadata correspondente (CreatedAt = agora).
	GenerateKey(opts GenerateKeyOptions) (Key, error)

	// ListKeys escaneia o diretório de chaves, reconcilia com metadata.json
	// por fingerprint (spec 5.3.2) e retorna a lista resultante ordenada por
	// proximidade de expiração (spec 5.3.3), com IsExpired/IsExpiringSoon já
	// calculados a partir de AppSettings.AlertThresholdDays.
	ListKeys() ([]Key, error)

	// GetKey retorna uma única Key pelo fingerprint. Retorna ErrKeyNotFound
	// se não houver chave em disco nem registro de metadata para ele.
	GetKey(fingerprint string) (Key, error)

	// UpdateMetadata edita os campos editáveis de uma chave já registrada
	// (label, notes, expiresAt) identificada pelo fingerprint. Não afeta os
	// arquivos em disco. Retorna ErrKeyNotFound se o fingerprint não tiver
	// registro de metadata (mesmo que exista em disco).
	UpdateMetadata(fingerprint string, patch KeyMetadataPatch) (Key, error)

	// RegisterKey adota um par de chaves encontrado em disco sem metadata
	// (KeyStatusUnregistered), a partir do caminho da chave privada, criando
	// um registro para ele.
	RegisterKey(privateKeyPath string, meta KeyMetadataPatch) (Key, error)

	// Unregister remove o registro de metadata de uma chave, identificado
	// por fingerprint — nunca toca em arquivos em disco. Usado sobretudo
	// para limpar registros KeyStatusMissingFile.
	Unregister(fingerprint string) error

	// Settings retorna a configuração atual do app (com defaults aplicados
	// se metadata.json ainda não existir).
	Settings() (AppSettings, error)

	// UpdateSettings persiste uma nova AppSettings.
	UpdateSettings(AppSettings) error
}

// FileKeyService implementa KeyService lendo/escrevendo chaves em disco e
// metadata em um arquivo JSON local.
type FileKeyService struct {
	// KeyDir é o diretório onde as chaves SSH vivem. Se vazio, usa ~/.ssh.
	KeyDir string
	// MetadataPath é o arquivo de metadata.json. Se vazio, usa
	// os.UserConfigDir()/keyward/metadata.json.
	MetadataPath string
}

func NewFileKeyService(keyDir, metadataPath string) *FileKeyService {
	return &FileKeyService{KeyDir: keyDir, MetadataPath: metadataPath}
}

func (s *FileKeyService) keyDir() (string, error) {
	if s.KeyDir != "" {
		return s.KeyDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolvendo diretório home: %w", err)
	}
	return filepath.Join(home, ".ssh"), nil
}

func (s *FileKeyService) metadataPath() (string, error) {
	if s.MetadataPath != "" {
		return s.MetadataPath, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolvendo diretório de configuração: %w", err)
	}
	return filepath.Join(configDir, "keyward", "metadata.json"), nil
}

func (s *FileKeyService) store() (*metadataStore, error) {
	path, err := s.metadataPath()
	if err != nil {
		return nil, err
	}
	return newMetadataStore(path), nil
}

func (s *FileKeyService) GenerateKey(opts GenerateKeyOptions) (Key, error) {
	dir := opts.Directory
	if dir == "" {
		var err error
		dir, err = s.keyDir()
		if err != nil {
			return Key{}, err
		}
	}

	st, err := s.store()
	if err != nil {
		return Key{}, err
	}
	mf, err := st.load()
	if err != nil {
		return Key{}, err
	}

	algorithm := opts.Algorithm
	if algorithm == "" {
		algorithm = mf.Settings.DefaultAlgorithm
		if algorithm == "" {
			algorithm = defaultKeyAlgorithm
		}
	}

	generated, err := generateKeyFiles(dir, algorithm, opts)
	if err != nil {
		return Key{}, err
	}

	meta := KeyMetadata{
		ID:          uuid.NewString(),
		Fingerprint: generated.fingerprint,
		KeyPath:     generated.privateKeyPath,
		Label:       opts.Label,
		Algorithm:   algorithm,
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   opts.ExpiresAt,
		Notes:       opts.Notes,
	}
	mf.Keys = append(mf.Keys, meta)
	if err := st.save(mf); err != nil {
		return Key{}, err
	}

	return Key{
		Metadata:       meta,
		PublicKeyPath:  generated.publicKeyPath,
		PrivateKeyPath: generated.privateKeyPath,
		Algorithm:      algorithm,
		KeyBits:        generated.bits,
		Comment:        generated.comment,
		Status:         KeyStatusOK,
	}, nil
}

func (s *FileKeyService) ListKeys() ([]Key, error) {
	dir, err := s.keyDir()
	if err != nil {
		return nil, err
	}
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	mf, err := st.load()
	if err != nil {
		return nil, err
	}
	return reconcileKeys(dir, mf)
}

func (s *FileKeyService) GetKey(fingerprint string) (Key, error) {
	keys, err := s.ListKeys()
	if err != nil {
		return Key{}, err
	}
	for _, k := range keys {
		if k.Metadata.Fingerprint == fingerprint {
			return k, nil
		}
	}
	return Key{}, ErrKeyNotFound
}

func (s *FileKeyService) UpdateMetadata(fingerprint string, patch KeyMetadataPatch) (Key, error) {
	st, err := s.store()
	if err != nil {
		return Key{}, err
	}
	mf, err := st.load()
	if err != nil {
		return Key{}, err
	}

	idx := -1
	for i := range mf.Keys {
		if mf.Keys[i].Fingerprint == fingerprint {
			idx = i
			break
		}
	}
	if idx == -1 {
		return Key{}, ErrKeyNotFound
	}

	if patch.Label != nil {
		mf.Keys[idx].Label = *patch.Label
	}
	if patch.Notes != nil {
		mf.Keys[idx].Notes = *patch.Notes
	}
	if patch.ExpiresAt != nil {
		mf.Keys[idx].ExpiresAt = *patch.ExpiresAt
	}

	if err := st.save(mf); err != nil {
		return Key{}, err
	}
	return s.GetKey(fingerprint)
}

func (s *FileKeyService) RegisterKey(privateKeyPath string, meta KeyMetadataPatch) (Key, error) {
	fingerprint, algorithm, err := fingerprintOfPrivateKey(privateKeyPath)
	if err != nil {
		return Key{}, err
	}

	st, err := s.store()
	if err != nil {
		return Key{}, err
	}
	mf, err := st.load()
	if err != nil {
		return Key{}, err
	}
	for _, k := range mf.Keys {
		if k.Fingerprint == fingerprint {
			return Key{}, fmt.Errorf("chave %s já está registrada (fingerprint %s)", privateKeyPath, fingerprint)
		}
	}

	record := KeyMetadata{
		ID:          uuid.NewString(),
		Fingerprint: fingerprint,
		KeyPath:     privateKeyPath,
		Algorithm:   algorithm,
		CreatedAt:   time.Now().UTC(),
	}
	if meta.Label != nil {
		record.Label = *meta.Label
	}
	if meta.Notes != nil {
		record.Notes = *meta.Notes
	}
	if meta.ExpiresAt != nil {
		record.ExpiresAt = *meta.ExpiresAt
	}

	mf.Keys = append(mf.Keys, record)
	if err := st.save(mf); err != nil {
		return Key{}, err
	}
	return s.GetKey(fingerprint)
}

func (s *FileKeyService) Unregister(fingerprint string) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	mf, err := st.load()
	if err != nil {
		return err
	}

	idx := -1
	for i := range mf.Keys {
		if mf.Keys[i].Fingerprint == fingerprint {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrKeyNotFound
	}

	mf.Keys = append(mf.Keys[:idx], mf.Keys[idx+1:]...)
	return st.save(mf)
}

func (s *FileKeyService) Settings() (AppSettings, error) {
	st, err := s.store()
	if err != nil {
		return AppSettings{}, err
	}
	mf, err := st.load()
	if err != nil {
		return AppSettings{}, err
	}
	return mf.Settings, nil
}

func (s *FileKeyService) UpdateSettings(settings AppSettings) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	mf, err := st.load()
	if err != nil {
		return err
	}
	mf.Settings = settings
	return st.save(mf)
}
