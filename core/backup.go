package core

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// KeySelection descreve uma chave a incluir num export e se o material
// privado dela deve entrar. As duas dimensões são ortogonais — uma chave
// pode entrar só com metadata (IncludePrivate=false), cobrindo o export
// "sync leve" da spec 5.4.5.
type KeySelection struct {
	Fingerprint    string
	IncludePrivate bool
}

// ExportOptions parametriza BackupService.Export.
type ExportOptions struct {
	// Hosts são os blocos a incluir no pacote — tipicamente um subconjunto
	// do retorno de ConfigService.ListHosts(), filtrado pelo chamador.
	Hosts []Host
	// Keys seleciona quais chaves entram no pacote.
	Keys []KeySelection
	// IncludeSettings inclui um snapshot de AppSettings no pacote.
	IncludeSettings bool
}

// HostConflict descreve um host do pacote que exige decisão explícita do
// chamador antes de Import poder aplicá-lo — porque já existe localmente
// com conteúdo diferente, e/ou porque o destino calculado cai fora do
// diretório home do usuário atual (nunca aplicado silenciosamente).
type HostConflict struct {
	// Key identifica esse conflito de forma estável entre PreviewImport e
	// Import — usada como chave em ImportResolutions.Hosts.
	Key string

	Host            Host // dados do pacote, já com IdentityFile reidratado na máquina atual
	DestinationFile string
	// ExternalPath é true quando DestinationFile cai fora do home do
	// usuário atual — exige confirmação mesmo que Existing seja nil.
	ExternalPath bool
	// Existing é o host local com os mesmos Patterns em DestinationFile,
	// hoje. nil se não existir (só ExternalPath motivou o conflito).
	Existing *Host
}

// KeyConflictKind distingue os dois tipos de conflito de chave possíveis no
// import.
type KeyConflictKind int

const (
	// KeyConflictAlreadyRegistered: já existe localmente uma chave com o
	// MESMO fingerprint — é a mesma chave; só a metadata pode divergir, o
	// arquivo nunca é reescrito.
	KeyConflictAlreadyRegistered KeyConflictKind = iota
	// KeyConflictFileNameCollision: o nome de arquivo da chave do pacote
	// colide, no KeyDir local, com uma chave de fingerprint DIFERENTE.
	KeyConflictFileNameCollision
)

// KeyConflict descreve um conflito de chave encontrado no import.
type KeyConflict struct {
	// Key identifica esse conflito de forma estável — usada como chave em
	// ImportResolutions.Keys. É o fingerprint da chave do pacote.
	Key  string
	Kind KeyConflictKind

	Metadata        KeyMetadata // dados da chave no pacote
	HasPrivateKey   bool
	DestinationPath string // caminho pretendido original (antes de eventual rename)

	// ExistingFingerprint só é preenchido quando Kind ==
	// KeyConflictFileNameCollision — fingerprint da chave hoje ocupando
	// DestinationPath.
	ExistingFingerprint string
}

// ImportPreview é o resultado de PreviewImport: tudo que Import faria, sem
// tocar em nada. Itens em HostsToAdd/KeysToAdd/HostsUnchanged/KeysUnchanged
// não têm conflito e são aplicados (ou pulados, no caso de "unchanged")
// automaticamente por Import, a menos que o chamador os marque
// explicitamente como Skip em ImportResolutions (cherry-pick). Itens em
// HostConflicts/KeyConflicts exigem resolução explícita — o default, se
// ausentes de ImportResolutions, é Skip (nunca sobrescreve silenciosamente).
type ImportPreview struct {
	ManifestVersion int
	CreatedAt       time.Time

	HostsToAdd     []Host
	HostsUnchanged []Host
	HostConflicts  []HostConflict

	KeysToAdd     []KeyMetadata
	KeysUnchanged []KeyMetadata
	KeyConflicts  []KeyConflict

	Settings *AppSettings // nil se o pacote não incluiu settings
}

// HostResolution decide o que Import faz com um host do pacote.
type HostResolution int

const (
	HostResolutionSkip HostResolution = iota
	// HostResolutionApply chama AddHost (Existing == nil) ou ReplaceHost
	// (Existing != nil).
	HostResolutionApply
)

// KeyResolution decide o que Import faz com uma chave do pacote.
type KeyResolution int

const (
	KeyResolutionSkip KeyResolution = iota
	KeyResolutionOverwrite
	// KeyResolutionImportRenamed só é válida para
	// KeyConflictFileNameCollision — grava sob nome alternativo,
	// preservando as duas chaves.
	KeyResolutionImportRenamed
)

// ImportResolutions carrega a decisão do chamador por item. Também serve
// para cherry-pick: uma entrada aqui para um item SEM conflito com valor
// Skip o exclui do import mesmo sem divergência. Itens ausentes deste mapa
// seguem o default descrito em ImportPreview (aplicar se sem conflito,
// pular se com conflito).
//
// Chave usada em cada mapa:
//   - Hosts: para itens em HostConflicts, use HostConflict.Key. Para itens
//     em HostsToAdd/HostsUnchanged (sem conflito), a chave é
//     hostImportKey(h.SourceFile, h.Patterns) — h.SourceFile já vem
//     resolvido para o destino local nesses dois casos.
//   - Keys: em todos os casos (com ou sem conflito), a chave é o
//     Fingerprint da chave — KeyConflict.Key e KeyMetadata.Fingerprint
//     usam o mesmo valor.
type ImportResolutions struct {
	Hosts map[string]HostResolution
	Keys  map[string]KeyResolution
}

// HostImportKey calcula a chave estável de um host para uso em
// ImportResolutions.Hosts, a partir do destino local (h.SourceFile, já
// resolvido) e dos Patterns — mesma chave usada internamente para
// HostConflict.Key.
func HostImportKey(destinationFile string, patterns []string) string {
	return hostImportKey(destinationFile, patterns)
}

// ImportResult resume o que Import efetivamente aplicou.
type ImportResult struct {
	HostsAdded     []Host
	HostsReplaced  []Host
	HostsUnchanged []Host
	HostsSkipped   []Host

	KeysAdded           []KeyMetadata
	KeysUnchanged       []KeyMetadata
	KeysMetadataUpdated []KeyMetadata // AlreadyRegistered + Overwrite
	KeysFileOverwritten []KeyMetadata // FileNameCollision + Overwrite
	KeysImportedRenamed []KeyMetadata // FileNameCollision + ImportRenamed
	KeysSkipped         []KeyMetadata

	SettingsApplied bool

	// Errors agrega falhas por item (ex. permissão negada ao escrever uma
	// chave) sem abortar o processamento dos demais itens independentes.
	Errors []error
}

// BackupService exporta/importa um pacote único (.tar.gz) com hosts
// selecionados, chaves selecionadas (com ou sem material privado) e a
// metadata associada (spec 5.4). Granularidade por host: usa
// ConfigService.AddHost/ReplaceHost como primitivas de escrita, não copia
// arquivos de config inteiros.
type BackupService interface {
	// Export grava em destPath um pacote .tar.gz com os hosts e chaves
	// selecionados em opts. Por padrão nenhuma chave privada é incluída —
	// só quando KeySelection.IncludePrivate=true, caso em que o material sai
	// em texto claro (decisão de escopo já registrada na spec; proteger o
	// arquivo resultante é responsabilidade de quem chama).
	Export(destPath string, opts ExportOptions) error

	// PreviewImport lê srcPath e retorna tudo que Import faria, sem
	// escrever nada — inclusive a resolução de destino de cada host
	// (considerando paths fora do home) e a detecção dos conflitos
	// possíveis.
	PreviewImport(srcPath string) (ImportPreview, error)

	// Import lê srcPath e aplica o conteúdo do pacote, usando resolutions
	// para decidir cada conflito reportado por PreviewImport (e,
	// opcionalmente, para excluir itens sem conflito via cherry-pick).
	// Retorna erro sem escrever nada se o pacote for inválido, tiver versão
	// de manifest não suportada, ou contiver uma referência de path que
	// alega portabilidade mas escapa do home do usuário atual. Falhas
	// isoladas por item não abortam os demais itens independentes — ver
	// ImportResult.Errors.
	Import(srcPath string, resolutions ImportResolutions) (ImportResult, error)
}

// FileBackupService implementa BackupService compondo os mesmos caminhos
// que FileConfigService/FileKeyService usam.
type FileBackupService struct {
	// ConfigPath é o arquivo ssh_config principal. Se vazio, usa
	// ~/.ssh/config (mesmo default de FileConfigService.Path).
	ConfigPath string
	// KeyDir é o diretório onde as chaves SSH vivem. Se vazio, usa ~/.ssh.
	KeyDir string
	// MetadataPath é o arquivo de metadata.json. Se vazio, usa
	// os.UserConfigDir()/keyward/metadata.json.
	MetadataPath string
}

func NewFileBackupService(configPath, keyDir, metadataPath string) *FileBackupService {
	return &FileBackupService{ConfigPath: configPath, KeyDir: keyDir, MetadataPath: metadataPath}
}

func (s *FileBackupService) configService() *FileConfigService {
	return &FileConfigService{Path: s.ConfigPath}
}

func (s *FileBackupService) keyService() *FileKeyService {
	return &FileKeyService{KeyDir: s.KeyDir, MetadataPath: s.MetadataPath}
}

func (s *FileBackupService) keyDir() (string, error) {
	if s.KeyDir != "" {
		return s.KeyDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolvendo diretório home: %w", err)
	}
	return filepath.Join(home, ".ssh"), nil
}

func (s *FileBackupService) store() (*metadataStore, error) {
	path := s.MetadataPath
	if path == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("resolvendo diretório de configuração: %w", err)
		}
		path = filepath.Join(configDir, "keyward", "metadata.json")
	}
	return newMetadataStore(path), nil
}
