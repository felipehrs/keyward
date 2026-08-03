package core

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

// stagedImport é o resultado de extrair e interpretar um pacote de backup,
// antes de qualquer gravação em disco.
type stagedImport struct {
	manifest backupManifest
	files    map[string][]byte // conteúdo bruto de cada entrada do .tar.gz
}

func (s *FileBackupService) PreviewImport(srcPath string) (ImportPreview, error) {
	preview, _, err := s.analyzeImport(srcPath)
	return preview, err
}

func (s *FileBackupService) Import(srcPath string, resolutions ImportResolutions) (ImportResult, error) {
	preview, staged, err := s.analyzeImport(srcPath)
	if err != nil {
		return ImportResult{}, err
	}

	result := ImportResult{}
	var errs []error

	if preview.Settings != nil {
		if err := s.keyService().UpdateSettings(*preview.Settings); err != nil {
			errs = append(errs, fmt.Errorf("aplicando settings: %w", err))
		} else {
			result.SettingsApplied = true
		}
	}

	s.applyKeys(staged, preview, resolutions, &result, &errs)
	s.applyHosts(preview, resolutions, &result, &errs)

	result.Errors = errs
	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}
	return result, nil
}

// analyzeImport extrai srcPath, interpreta o manifest e classifica cada
// host/chave em "para adicionar", "sem mudança" ou "conflito", sem escrever
// nada em disco. staged é reaproveitado por Import para aplicar as
// resoluções sem precisar extrair o pacote de novo.
func (s *FileBackupService) analyzeImport(srcPath string) (ImportPreview, stagedImport, error) {
	files, err := extractArchive(srcPath)
	if err != nil {
		return ImportPreview{}, stagedImport{}, err
	}

	manifestData, ok := files["manifest.json"]
	if !ok {
		return ImportPreview{}, stagedImport{}, fmt.Errorf("pacote %s: manifest.json ausente", srcPath)
	}
	var manifest backupManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return ImportPreview{}, stagedImport{}, fmt.Errorf("interpretando manifest de %s: %w", srcPath, err)
	}
	if manifest.Version > backupManifestVersion {
		return ImportPreview{}, stagedImport{}, fmt.Errorf(
			"pacote %s: versão de manifest %d não suportada por esta versão do keyward (máximo %d)",
			srcPath, manifest.Version, backupManifestVersion)
	}

	staged := stagedImport{manifest: manifest, files: files}

	home, err := os.UserHomeDir()
	if err != nil {
		return ImportPreview{}, stagedImport{}, fmt.Errorf("resolvendo diretório home: %w", err)
	}

	preview := ImportPreview{
		ManifestVersion: manifest.Version,
		CreatedAt:       manifest.CreatedAt,
		Settings:        manifest.Settings,
	}

	if err := s.analyzeHosts(&preview, manifest, home); err != nil {
		return ImportPreview{}, stagedImport{}, fmt.Errorf("pacote %s: %w", srcPath, err)
	}
	if err := s.analyzeKeys(&preview, staged); err != nil {
		return ImportPreview{}, stagedImport{}, fmt.Errorf("pacote %s: %w", srcPath, err)
	}

	return preview, staged, nil
}

func (s *FileBackupService) analyzeHosts(preview *ImportPreview, manifest backupManifest, home string) error {
	localHosts, err := s.configService().ListHosts()
	if err != nil {
		return err
	}

	for _, bh := range manifest.Hosts {
		dest, external, err := resolveHostDestination(bh.SourceFile, home)
		if err != nil {
			return fmt.Errorf("host %v: %w", bh.Patterns, err)
		}

		h := hostFromBackupHost(bh, home, dest)
		existing := findLocalHost(localHosts, dest, bh.Patterns)

		switch {
		case existing == nil && !external:
			preview.HostsToAdd = append(preview.HostsToAdd, h)
		case existing != nil && !external && hostsContentEqual(*existing, h):
			preview.HostsUnchanged = append(preview.HostsUnchanged, h)
		default:
			preview.HostConflicts = append(preview.HostConflicts, HostConflict{
				Key:             hostImportKey(dest, bh.Patterns),
				Host:            h,
				DestinationFile: dest,
				ExternalPath:    external,
				Existing:        existing,
			})
		}
	}
	return nil
}

func (s *FileBackupService) analyzeKeys(preview *ImportPreview, staged stagedImport) error {
	keySvc := s.keyService()
	keyDir, err := s.keyDir()
	if err != nil {
		return err
	}

	for _, bk := range staged.manifest.Keys {
		if err := validateKeyFileName(bk.FileName); err != nil {
			return fmt.Errorf("chave %s: %w", bk.Metadata.Fingerprint, err)
		}
		destPath := filepath.Join(keyDir, bk.FileName)

		if bk.HasPrivateKey {
			pubData, ok := staged.files["keys/"+bk.FileName+".pub"]
			privOK := true
			if _, ok2 := staged.files["keys/"+bk.FileName]; !ok2 {
				privOK = false
			}
			if !ok || !privOK {
				return fmt.Errorf("chave %s: arquivo(s) ausente(s) no pacote", bk.Metadata.Fingerprint)
			}
			actualFingerprint, err := fingerprintFromPublicKeyBytes(pubData)
			if err != nil {
				return fmt.Errorf("chave %s: %w", bk.Metadata.Fingerprint, err)
			}
			if actualFingerprint != bk.Metadata.Fingerprint {
				return fmt.Errorf("chave declarada como %s mas o .pub extraído tem fingerprint %s — pacote possivelmente adulterado",
					bk.Metadata.Fingerprint, actualFingerprint)
			}
		}

		localKey, err := keySvc.GetKey(bk.Metadata.Fingerprint)
		if err == nil && localKey.Status != KeyStatusUnregistered {
			if keyMetadataEqual(localKey.Metadata, bk.Metadata) {
				preview.KeysUnchanged = append(preview.KeysUnchanged, bk.Metadata)
			} else {
				preview.KeyConflicts = append(preview.KeyConflicts, KeyConflict{
					Key:             bk.Metadata.Fingerprint,
					Kind:            KeyConflictAlreadyRegistered,
					Metadata:        bk.Metadata,
					HasPrivateKey:   bk.HasPrivateKey,
					DestinationPath: destPath,
				})
			}
			continue
		}
		if err != nil && !errors.Is(err, ErrKeyNotFound) {
			return err
		}

		if bk.HasPrivateKey {
			if existingFp, found := diskFingerprintAt(destPath); found && existingFp != bk.Metadata.Fingerprint {
				preview.KeyConflicts = append(preview.KeyConflicts, KeyConflict{
					Key:                 bk.Metadata.Fingerprint,
					Kind:                KeyConflictFileNameCollision,
					Metadata:            bk.Metadata,
					HasPrivateKey:       true,
					DestinationPath:     destPath,
					ExistingFingerprint: existingFp,
				})
				continue
			}
		}
		preview.KeysToAdd = append(preview.KeysToAdd, bk.Metadata)
	}
	return nil
}

func (s *FileBackupService) applyHosts(preview ImportPreview, resolutions ImportResolutions, result *ImportResult, errs *[]error) {
	configSvc := s.configService()

	for _, h := range preview.HostsToAdd {
		key := hostImportKey(h.SourceFile, h.Patterns)
		// Sem conflito: default é aplicar. Só pula se EXPLICITAMENTE
		// marcado Skip — resolutions.Hosts[key] sozinho não basta, porque o
		// zero value de HostResolution é HostResolutionSkip, e isso
		// confundiria "ausente do mapa" com "marcado Skip".
		if r, ok := resolutions.Hosts[key]; ok && r == HostResolutionSkip {
			result.HostsSkipped = append(result.HostsSkipped, h)
			continue
		}
		if err := configSvc.AddHost(h.SourceFile, hostSpecFromHost(h)); err != nil {
			*errs = append(*errs, fmt.Errorf("adicionando host %v em %s: %w", h.Patterns, h.SourceFile, err))
			continue
		}
		result.HostsAdded = append(result.HostsAdded, h)
	}

	for _, h := range preview.HostsUnchanged {
		key := hostImportKey(h.SourceFile, h.Patterns)
		if r, ok := resolutions.Hosts[key]; ok && r == HostResolutionSkip {
			result.HostsSkipped = append(result.HostsSkipped, h)
			continue
		}
		result.HostsUnchanged = append(result.HostsUnchanged, h)
	}

	for _, c := range preview.HostConflicts {
		if resolutions.Hosts[c.Key] != HostResolutionApply {
			result.HostsSkipped = append(result.HostsSkipped, c.Host)
			continue
		}
		spec := hostSpecFromHost(c.Host)
		if c.Existing == nil {
			if err := configSvc.AddHost(c.DestinationFile, spec); err != nil {
				*errs = append(*errs, fmt.Errorf("adicionando host %v em %s: %w", c.Host.Patterns, c.DestinationFile, err))
				continue
			}
			result.HostsAdded = append(result.HostsAdded, c.Host)
			continue
		}
		if err := configSvc.ReplaceHost(c.DestinationFile, c.Existing.Patterns, spec); err != nil {
			*errs = append(*errs, fmt.Errorf("substituindo host %v em %s: %w", c.Host.Patterns, c.DestinationFile, err))
			continue
		}
		result.HostsReplaced = append(result.HostsReplaced, c.Host)
	}
}

func (s *FileBackupService) applyKeys(staged stagedImport, preview ImportPreview, resolutions ImportResolutions, result *ImportResult, errs *[]error) {
	st, err := s.store()
	if err != nil {
		*errs = append(*errs, err)
		return
	}
	keyDir, err := s.keyDir()
	if err != nil {
		*errs = append(*errs, err)
		return
	}
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		*errs = append(*errs, fmt.Errorf("criando %s: %w", keyDir, err))
		return
	}

	mf, err := st.load()
	if err != nil {
		*errs = append(*errs, err)
		return
	}
	dirty := false

	upsertMetadata := func(meta KeyMetadata) {
		for i := range mf.Keys {
			if mf.Keys[i].Fingerprint == meta.Fingerprint {
				mf.Keys[i] = meta
				return
			}
		}
		mf.Keys = append(mf.Keys, meta)
	}

	writeKeyFiles := func(fileName string) (privPath, pubPath string, err error) {
		privData, ok1 := staged.files["keys/"+fileName]
		pubData, ok2 := staged.files["keys/"+fileName+".pub"]
		if !ok1 || !ok2 {
			return "", "", fmt.Errorf("arquivo(s) de %s ausente(s) no pacote", fileName)
		}
		privPath = filepath.Join(keyDir, fileName)
		pubPath = privPath + ".pub"
		if err := writeNewFile(privPath, privData, 0o600); err != nil {
			return "", "", err
		}
		if err := writeNewFile(pubPath, pubData, 0o644); err != nil {
			return "", "", err
		}
		return privPath, pubPath, nil
	}

	for _, meta := range preview.KeysToAdd {
		// Mesma ressalva de applyHosts: só pula se EXPLICITAMENTE marcado
		// Skip, já que o zero value de KeyResolution é KeyResolutionSkip.
		if r, ok := resolutions.Keys[meta.Fingerprint]; ok && r == KeyResolutionSkip {
			result.KeysSkipped = append(result.KeysSkipped, meta)
			continue
		}
		bk := findBackupKey(staged.manifest.Keys, meta.Fingerprint)
		newMeta := meta
		newMeta.ID = uuid.NewString()
		if bk.HasPrivateKey {
			privPath, _, err := writeKeyFiles(bk.FileName)
			if err != nil {
				*errs = append(*errs, fmt.Errorf("chave %s: %w", meta.Fingerprint, err))
				continue
			}
			newMeta.KeyPath = privPath
		} else {
			newMeta.KeyPath = filepath.Join(keyDir, bk.FileName)
		}
		upsertMetadata(newMeta)
		dirty = true
		result.KeysAdded = append(result.KeysAdded, newMeta)
	}

	for _, meta := range preview.KeysUnchanged {
		if r, ok := resolutions.Keys[meta.Fingerprint]; ok && r == KeyResolutionSkip {
			result.KeysSkipped = append(result.KeysSkipped, meta)
			continue
		}
		result.KeysUnchanged = append(result.KeysUnchanged, meta)
	}

	for _, c := range preview.KeyConflicts {
		resolution := resolutions.Keys[c.Key]
		if resolution == KeyResolutionSkip {
			result.KeysSkipped = append(result.KeysSkipped, c.Metadata)
			continue
		}

		switch c.Kind {
		case KeyConflictAlreadyRegistered:
			if resolution != KeyResolutionOverwrite {
				result.KeysSkipped = append(result.KeysSkipped, c.Metadata)
				continue
			}
			idx := indexOfFingerprint(mf.Keys, c.Key)
			if idx == -1 {
				*errs = append(*errs, fmt.Errorf("chave %s: registro local não encontrado ao aplicar overwrite", c.Key))
				continue
			}
			mf.Keys[idx].Label = c.Metadata.Label
			mf.Keys[idx].Notes = c.Metadata.Notes
			mf.Keys[idx].ExpiresAt = c.Metadata.ExpiresAt
			mf.Keys[idx].Algorithm = c.Metadata.Algorithm
			// ID, KeyPath e CreatedAt permanecem os locais (decisão registrada no plano).
			dirty = true
			result.KeysMetadataUpdated = append(result.KeysMetadataUpdated, mf.Keys[idx])

		case KeyConflictFileNameCollision:
			bk := findBackupKey(staged.manifest.Keys, c.Key)
			switch resolution {
			case KeyResolutionOverwrite:
				privPath, _, err := writeKeyFiles(bk.FileName)
				if err != nil {
					*errs = append(*errs, fmt.Errorf("chave %s: %w", c.Key, err))
					continue
				}
				newMeta := c.Metadata
				newMeta.ID = uuid.NewString()
				newMeta.KeyPath = privPath
				upsertMetadata(newMeta)
				dirty = true
				result.KeysFileOverwritten = append(result.KeysFileOverwritten, newMeta)

			case KeyResolutionImportRenamed:
				renamedName := renamedKeyFileName(bk.FileName, c.Key)
				privData, ok1 := staged.files["keys/"+bk.FileName]
				pubData, ok2 := staged.files["keys/"+bk.FileName+".pub"]
				if !ok1 || !ok2 {
					*errs = append(*errs, fmt.Errorf("chave %s: arquivo(s) ausente(s) no pacote", c.Key))
					continue
				}
				privPath := filepath.Join(keyDir, renamedName)
				pubPath := privPath + ".pub"
				if err := writeNewFile(privPath, privData, 0o600); err != nil {
					*errs = append(*errs, fmt.Errorf("chave %s: %w", c.Key, err))
					continue
				}
				if err := writeNewFile(pubPath, pubData, 0o644); err != nil {
					*errs = append(*errs, fmt.Errorf("chave %s: %w", c.Key, err))
					continue
				}
				newMeta := c.Metadata
				newMeta.ID = uuid.NewString()
				newMeta.KeyPath = privPath
				upsertMetadata(newMeta)
				dirty = true
				result.KeysImportedRenamed = append(result.KeysImportedRenamed, newMeta)

			default:
				result.KeysSkipped = append(result.KeysSkipped, c.Metadata)
			}
		}
	}

	if dirty {
		if err := st.save(mf); err != nil {
			*errs = append(*errs, fmt.Errorf("salvando metadata: %w", err))
		}
	}
}

// resolveHostDestination calcula, na máquina atual, o caminho absoluto de
// destino para o sourceFile (já em forma portável) de um host do pacote.
// external indica que o destino fica fora do home do usuário atual —
// nunca aplicado automaticamente (sempre vira HostConflict). Retorna erro
// se sourceFile alegar portabilidade ("~/...") mas, após expandir contra o
// home atual, resultar fora dele — só acontece com manifest
// corrompido/adulterado, tratado como rejeição dura do pacote inteiro.
func resolveHostDestination(sourceFile, home string) (dest string, external bool, err error) {
	claimsPortable := sourceFile == "~" || strings.HasPrefix(sourceFile, "~/")

	var candidate string
	switch {
	case claimsPortable:
		candidate = expandHome(sourceFile)
	case filepath.IsAbs(sourceFile):
		candidate = sourceFile
	default:
		return "", false, fmt.Errorf("sourceFile %q inválido: nem absoluto, nem ~/", sourceFile)
	}
	candidate = filepath.Clean(candidate)

	rel, relErr := filepath.Rel(filepath.Clean(home), candidate)
	underHome := relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))

	if claimsPortable && !underHome {
		return "", false, fmt.Errorf("sourceFile %q alega ser portável mas resolve fora do home atual — pacote possivelmente adulterado", sourceFile)
	}
	return candidate, !underHome, nil
}

// hostFromBackupHost reidrata um backupHost do manifest para o tipo de
// domínio Host, usando o home da máquina ATUAL (não da máquina de origem)
// para expandir IdentityFile, e dest (já resolvido) como SourceFile.
func hostFromBackupHost(bh backupHost, home, dest string) Host {
	identityFiles := make([]string, len(bh.IdentityFile))
	for i, idf := range bh.IdentityFile {
		identityFiles[i] = expandHomeAt(idf, home)
	}
	return Host{
		Patterns:     bh.Patterns,
		SourceFile:   dest,
		HostName:     bh.HostName,
		User:         bh.User,
		Port:         bh.Port,
		IdentityFile: identityFiles,
	}
}

// expandHomeAt é como expandHome, mas recebe home explicitamente em vez de
// resolvê-lo via os.UserHomeDir() — usado no import para garantir que a
// expansão sempre usa o mesmo home já resolvido para o resto da operação.
func expandHomeAt(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// hostImportKey é a chave estável de um host para ImportResolutions.Hosts
// e HostConflict.Key — combina o destino local resolvido com os Patterns,
// já que Patterns sozinho não é único entre arquivos diferentes.
func hostImportKey(destinationFile string, patterns []string) string {
	return destinationFile + "#" + strings.Join(patterns, ",")
}

func findLocalHost(hosts []Host, sourceFile string, patterns []string) *Host {
	for i := range hosts {
		if hosts[i].SourceFile == sourceFile && patternsEqual(hosts[i].Patterns, patterns) {
			return &hosts[i]
		}
	}
	return nil
}

func hostsContentEqual(a, b Host) bool {
	if a.HostName != b.HostName || a.User != b.User || a.Port != b.Port {
		return false
	}
	if len(a.IdentityFile) != len(b.IdentityFile) {
		return false
	}
	for i := range a.IdentityFile {
		if a.IdentityFile[i] != b.IdentityFile[i] {
			return false
		}
	}
	return true
}

func hostSpecFromHost(h Host) HostSpec {
	return HostSpec{
		Patterns:     h.Patterns,
		HostName:     h.HostName,
		User:         h.User,
		Port:         h.Port,
		IdentityFile: h.IdentityFile,
	}
}

// validateKeyFileName garante que name é um nome de arquivo puro (sem
// diretórios, sem "." nem ".."), já que ele é usado para montar um caminho
// de destino a partir de entrada não confiável (o pacote de backup).
func validateKeyFileName(name string) error {
	if name == "" {
		return fmt.Errorf("fileName vazio")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("fileName inválido: %q", name)
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("fileName deve ser um nome puro, sem diretórios: %q", name)
	}
	return nil
}

// fingerprintFromPublicKeyBytes calcula o fingerprint SHA256 de um arquivo
// .pub extraído do pacote — usado para reconferir a integridade de cada
// chave contra o que o manifest declara.
func fingerprintFromPublicKeyBytes(pubBytes []byte) (string, error) {
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(pubBytes)
	if err != nil {
		return "", fmt.Errorf("interpretando chave pública: %w", err)
	}
	return ssh.FingerprintSHA256(pubKey), nil
}

// diskFingerprintAt retorna o fingerprint da chave privada em path, se o
// arquivo existir. found=false se path não existir. Se path existir mas o
// fingerprint não puder ser determinado (.pub ausente/corrompido),
// retorna found=true com fingerprint vazio — tratado pelo chamador como
// "há algo nesse path, mas não sabemos o quê", ainda assim um conflito a
// confirmar.
func diskFingerprintAt(path string) (fingerprint string, found bool) {
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	fp, _, err := fingerprintOfPrivateKey(path)
	if err != nil {
		return "", true
	}
	return fp, true
}

func keyMetadataEqual(a, b KeyMetadata) bool {
	if a.Label != b.Label || a.Notes != b.Notes || a.Algorithm != b.Algorithm {
		return false
	}
	switch {
	case a.ExpiresAt == nil && b.ExpiresAt == nil:
		return true
	case a.ExpiresAt == nil || b.ExpiresAt == nil:
		return false
	default:
		return a.ExpiresAt.Equal(*b.ExpiresAt)
	}
}

func findBackupKey(keys []backupKey, fingerprint string) backupKey {
	for _, k := range keys {
		if k.Metadata.Fingerprint == fingerprint {
			return k
		}
	}
	return backupKey{}
}

func indexOfFingerprint(keys []KeyMetadata, fingerprint string) int {
	for i := range keys {
		if keys[i].Fingerprint == fingerprint {
			return i
		}
	}
	return -1
}

// renamedKeyFileName monta o nome de arquivo usado por
// KeyResolutionImportRenamed: um sufixo hex curto derivado do fingerprint,
// não o fingerprint bruto (que tem caracteres inseguros como nome de
// arquivo em algumas plataformas, ex. "/" em "SHA256:base64").
func renamedKeyFileName(originalFileName, fingerprint string) string {
	sum := sha256.Sum256([]byte(fingerprint))
	return fmt.Sprintf("%s.imported-%x", originalFileName, sum[:4])
}
