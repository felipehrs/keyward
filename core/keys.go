package core

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// ErrKeyNotFound é retornado quando um fingerprint não corresponde a
// nenhuma chave conhecida (nem em disco, nem em metadata.json).
var ErrKeyNotFound = errors.New("keyward: chave não encontrada")

// ErrHostLinkNotFound é retornado por UnlinkHostKey quando o par
// (hostKey, agentKeyFingerprint) não corresponde a nenhum HostMetadata
// persistido.
var ErrHostLinkNotFound = errors.New("keyward: vínculo host/chave não encontrado")

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
	// KeyStatusAgentOffline: registro em metadata.json com Source ==
	// KeySourceAgent cuja identidade não pôde ser confirmada porque o
	// ssh-agent está inacessível (sem SSH_AUTH_SOCK, dial falhou, timeout) ou
	// não a oferece mais.
	KeyStatusAgentOffline
)

// KeySource indica de onde uma Key foi obtida: um par de arquivos em
// ~/.ssh/ (padrão, comportamento histórico) ou uma identidade oferecida por
// um ssh-agent em execução (spec ssh-agent-support, requisitos 1-4).
type KeySource int

const (
	// KeySourceFile: chave lida de um par de arquivos em disco. É o valor
	// zero de KeySource — registros de metadata.json anteriores a este
	// campo existir decodificam como KeySourceFile automaticamente.
	KeySourceFile KeySource = iota
	// KeySourceAgent: chave cuja identidade vem de um ssh-agent, sem par de
	// arquivos correspondente em disco necessariamente presente/legível.
	KeySourceAgent
)

// Key é a visão consolidada de uma chave SSH, unindo o que foi encontrado em
// disco (~/.ssh/) com o registro de KeyMetadata correspondente, quando
// existir. É o tipo retornado por ListKeys/GetKey — nunca é serializado
// diretamente em metadata.json (isso é papel exclusivo de KeyMetadata).
type Key struct {
	Metadata KeyMetadata // zero value quando Status == KeyStatusUnregistered

	PublicKeyPath  string // "" se Status == KeyStatusMissingFile ou Source == KeySourceAgent
	PrivateKeyPath string // "" se Status == KeyStatusMissingFile ou Source == KeySourceAgent
	Algorithm      Algorithm
	KeyBits        int    // bits efetivos (RSA); 256 fixo para ed25519
	Comment        string // comentário embutido no arquivo .pub

	Status KeyStatus

	// Source indica se esta Key veio de um par de arquivos em disco ou de
	// uma identidade oferecida por um ssh-agent (spec ssh-agent-support).
	Source KeySource
	// AgentName identifica o agente de origem (ex. "1password") quando
	// Source == KeySourceAgent; "" para agente genérico não identificado.
	AgentName string

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

	// RegisterAgentKey adota uma identidade oferecida por um ssh-agent, sem
	// registro de metadata ainda (KeyStatusUnregistered, Source ==
	// KeySourceAgent), a partir do fingerprint já exposto por ListKeys —
	// espelha RegisterKey, mas sem caminho de arquivo (a identidade já vem
	// do agente, não há fingerprint a calcular). Retorna erro se já houver
	// registro para esse fingerprint (mesma checagem de duplicata de
	// RegisterKey).
	RegisterAgentKey(fingerprint string, meta KeyMetadataPatch) (Key, error)

	// DetectAgent verifica se há um ssh-agent acessível via SSH_AUTH_SOCK,
	// sempre com um dial novo — nunca reaproveita conexão ou estado de
	// ListKeys(). Detected == true assim que o dial e a listagem de
	// identidades tiverem sucesso, mesmo que a lista venha vazia (ex.
	// cofre do 1Password bloqueado, mas o agente está presente).
	DetectAgent() (AgentInfo, error)

	// LinkHostKey persiste um vínculo entre um host de ~/.ssh/config
	// (identificado por hostKey, valor opaco para este pacote — a convenção
	// de calculá-lo a partir dos Patterns do Host é responsabilidade do
	// chamador) e uma identidade de ssh-agent (agentKeyFingerprint). Chamar
	// de novo com o mesmo par (hostKey, agentKeyFingerprint) é idempotente:
	// atualiza Notes do registro existente em vez de duplicar.
	LinkHostKey(hostKey, agentKeyFingerprint, notes string) error

	// UnlinkHostKey remove o vínculo identificado pelo par (hostKey,
	// agentKeyFingerprint). Retorna ErrHostLinkNotFound se não houver
	// registro correspondente.
	UnlinkHostKey(hostKey, agentKeyFingerprint string) error

	// ListHostLinks retorna todos os vínculos host/chave-de-agente
	// persistidos, verbatim — sem cruzar com ConfigService.ListHosts()
	// (detecção de vínculo órfão é responsabilidade da camada de UI).
	ListHostLinks() ([]HostMetadata, error)
}

// FileKeyService implementa KeyService lendo/escrevendo chaves em disco e
// metadata em um arquivo JSON local.
type FileKeyService struct {
	// KeyDir é o diretório onde as chaves SSH vivem. Se vazio, usa ~/.ssh.
	KeyDir string
	// MetadataPath é o arquivo de metadata.json. Se vazio, usa
	// os.UserConfigDir()/keyward/metadata.json.
	MetadataPath string
	// AgentDial abre a conexão com o ssh-agent, respeitando o timeout
	// recebido como parâmetro — a implementação não deve inventar seu
	// próprio deadline, quem o impõe é o chamador (core). Se nil, usa
	// defaultAgentDial (core/keys_agent.go), que dial um Unix domain socket
	// via SSH_AUTH_SOCK.
	AgentDial func(timeout time.Duration) (net.Conn, error)
}

func NewFileKeyService(keyDir, metadataPath string) *FileKeyService {
	return &FileKeyService{KeyDir: keyDir, MetadataPath: metadataPath}
}

// agentDialTimeout é o timeout imposto ao dial do ssh-agent tanto em
// ListKeys() quanto em DetectAgent() (spec ssh-agent-support, seção 4).
const agentDialTimeout = 500 * time.Millisecond

func (s *FileKeyService) agentDial() func(timeout time.Duration) (net.Conn, error) {
	if s.AgentDial != nil {
		return s.AgentDial
	}
	return defaultAgentDial
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
	fileKeys, err := reconcileKeys(dir, mf)
	if err != nil {
		return nil, err
	}
	agentKeys := reconcileAgentKeys(s.agentDial(), agentDialTimeout, mf)
	return append(fileKeys, agentKeys...), nil
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

func (s *FileKeyService) RegisterAgentKey(fingerprint string, meta KeyMetadataPatch) (Key, error) {
	algorithm, err := agentOfferedIdentityAlgorithm(s.agentDial(), agentDialTimeout, fingerprint)
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
			return Key{}, fmt.Errorf("chave %s já está registrada", fingerprint)
		}
	}

	record := KeyMetadata{
		ID:          uuid.NewString(),
		Fingerprint: fingerprint,
		Algorithm:   algorithm,
		CreatedAt:   time.Now().UTC(),
		Source:      KeySourceAgent,
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

func (s *FileKeyService) DetectAgent() (AgentInfo, error) {
	return detectAgent(s.agentDial(), agentDialTimeout)
}

func (s *FileKeyService) LinkHostKey(hostKey, agentKeyFingerprint, notes string) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	mf, err := st.load()
	if err != nil {
		return err
	}

	for i := range mf.Hosts {
		if mf.Hosts[i].HostKey == hostKey && mf.Hosts[i].AgentKeyFingerprint == agentKeyFingerprint {
			mf.Hosts[i].Notes = notes
			return st.save(mf)
		}
	}

	mf.Hosts = append(mf.Hosts, HostMetadata{
		HostKey:             hostKey,
		AgentKeyFingerprint: agentKeyFingerprint,
		Notes:               notes,
	})
	return st.save(mf)
}

func (s *FileKeyService) UnlinkHostKey(hostKey, agentKeyFingerprint string) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	mf, err := st.load()
	if err != nil {
		return err
	}

	idx := -1
	for i := range mf.Hosts {
		if mf.Hosts[i].HostKey == hostKey && mf.Hosts[i].AgentKeyFingerprint == agentKeyFingerprint {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrHostLinkNotFound
	}

	mf.Hosts = append(mf.Hosts[:idx], mf.Hosts[idx+1:]...)
	return st.save(mf)
}

func (s *FileKeyService) ListHostLinks() ([]HostMetadata, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	mf, err := st.load()
	if err != nil {
		return nil, err
	}
	return mf.Hosts, nil
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
