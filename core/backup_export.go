package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (s *FileBackupService) Export(destPath string, opts ExportOptions) error {
	manifest := backupManifest{
		Version:   backupManifestVersion,
		CreatedAt: time.Now().UTC(),
	}

	for _, h := range opts.Hosts {
		manifest.Hosts = append(manifest.Hosts, backupHostFromHost(h))
	}

	var keyFiles []archiveFile
	if len(opts.Keys) > 0 {
		keySvc := s.keyService()
		for _, sel := range opts.Keys {
			bk, files, err := exportKey(keySvc, sel)
			if err != nil {
				return err
			}
			manifest.Keys = append(manifest.Keys, bk)
			keyFiles = append(keyFiles, files...)
		}
	}

	if opts.IncludeSettings {
		settings, err := s.keyService().Settings()
		if err != nil {
			return err
		}
		manifest.Settings = &settings
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("serializando manifest: %w", err)
	}

	files := make([]archiveFile, 0, 1+len(keyFiles))
	files = append(files, archiveFile{Name: "manifest.json", Mode: 0o600, Data: manifestData})
	files = append(files, keyFiles...)

	return createArchive(destPath, files)
}

// exportKey resolve uma KeySelection para a entrada de manifest
// correspondente e, se IncludePrivate, os bytes dos arquivos privado/.pub a
// incluir no pacote.
func exportKey(keySvc *FileKeyService, sel KeySelection) (backupKey, []archiveFile, error) {
	key, err := keySvc.GetKey(sel.Fingerprint)
	if err != nil {
		return backupKey{}, nil, fmt.Errorf("chave %s: %w", sel.Fingerprint, err)
	}
	if key.Status == KeyStatusUnregistered {
		return backupKey{}, nil, fmt.Errorf(
			"chave %s: não está registrada em metadata.json (registre com KeyService.RegisterKey antes de exportar)",
			sel.Fingerprint)
	}

	bk := backupKey{Metadata: key.Metadata}
	switch {
	case key.PrivateKeyPath != "":
		bk.FileName = filepath.Base(key.PrivateKeyPath)
	case key.Metadata.KeyPath != "":
		bk.FileName = filepath.Base(key.Metadata.KeyPath)
	default:
		return backupKey{}, nil, fmt.Errorf("chave %s: sem caminho conhecido, não é possível determinar nome de arquivo", sel.Fingerprint)
	}

	if !sel.IncludePrivate {
		return bk, nil, nil
	}
	if key.PrivateKeyPath == "" {
		return backupKey{}, nil, fmt.Errorf("chave %s: material privado não encontrado em disco (status %v)", sel.Fingerprint, key.Status)
	}

	privData, err := os.ReadFile(key.PrivateKeyPath)
	if err != nil {
		return backupKey{}, nil, fmt.Errorf("lendo %s: %w", key.PrivateKeyPath, err)
	}
	pubData, err := os.ReadFile(key.PublicKeyPath)
	if err != nil {
		return backupKey{}, nil, fmt.Errorf("lendo %s: %w", key.PublicKeyPath, err)
	}

	bk.HasPrivateKey = true
	files := []archiveFile{
		{Name: "keys/" + bk.FileName, Mode: 0o600, Data: privData},
		{Name: "keys/" + bk.FileName + ".pub", Mode: 0o644, Data: pubData},
	}
	return bk, files, nil
}

// backupHostFromHost converte um Host (leitura) para backupHost (manifest),
// compactando SourceFile e cada IdentityFile de volta para "~/..." quando
// estiverem sob o home da máquina de origem — ver achado de design sobre
// portabilidade no plano de implementação.
func backupHostFromHost(h Host) backupHost {
	identityFiles := make([]string, len(h.IdentityFile))
	for i, idf := range h.IdentityFile {
		identityFiles[i] = compactHome(idf)
	}
	return backupHost{
		Patterns:     h.Patterns,
		SourceFile:   compactHome(h.SourceFile),
		HostName:     h.HostName,
		User:         h.User,
		Port:         h.Port,
		IdentityFile: identityFiles,
	}
}
