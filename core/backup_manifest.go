package core

import "time"

// backupManifestVersion identifica o formato de backupManifest dentro do
// pacote (mesmo padrão de metadataSchemaVersion em metadata.go). Import e
// PreviewImport recusam pacotes com uma versão maior que esta — sinal de
// keyward mais novo do que quem está lendo.
const backupManifestVersion = 1

// backupManifest é o documento raiz de manifest.json dentro do pacote de
// backup.
type backupManifest struct {
	Version   int          `json:"version"`
	CreatedAt time.Time    `json:"createdAt"`
	Hosts     []backupHost `json:"hosts"`
	Keys      []backupKey  `json:"keys"`
	Settings  *AppSettings `json:"settings,omitempty"`
}

// backupHost espelha Host (core/config.go), mas SourceFile e IdentityFile
// já passaram por compactHome() na máquina de origem — "~/..." quando
// estavam sob o home de origem, caminho absoluto intacto caso contrário.
// Ver o achado de design sobre portabilidade no plano de implementação.
type backupHost struct {
	Patterns     []string `json:"patterns"`
	SourceFile   string   `json:"sourceFile"`
	HostName     string   `json:"hostName,omitempty"`
	User         string   `json:"user,omitempty"`
	Port         string   `json:"port,omitempty"`
	IdentityFile []string `json:"identityFile,omitempty"`
}

// backupKey associa a KeyMetadata original (ID/CreatedAt preservados) ao
// arquivo de chave dentro do pacote, quando presente. O arquivo público
// segue a convenção FileName+".pub", já usada em keygen.go/key_reconcile.go
// — não precisa de um campo separado.
//
// Metadata.KeyPath continua sendo o caminho da máquina de origem (só valor
// histórico/auditoria) — o import nunca usa esse campo para decidir onde
// gravar; usa o KeyDir local + FileName.
type backupKey struct {
	Metadata      KeyMetadata `json:"metadata"`
	FileName      string      `json:"fileName"`
	HasPrivateKey bool        `json:"hasPrivateKey"`
}
