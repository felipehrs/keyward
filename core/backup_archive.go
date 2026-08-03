package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// archiveFile representa uma entrada a incluir num pacote de backup.
type archiveFile struct {
	Name string // caminho dentro do pacote, sempre com "/" (ver validateArchiveName)
	Mode os.FileMode
	Data []byte
}

// createArchive grava em destPath um .tar.gz contendo files. Cria os
// diretórios pais de destPath se necessário e escreve atomicamente (mesmo
// padrão do restante do core, via atomicWriteFile).
func createArchive(destPath string, files []archiveFile) error {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	now := time.Now()
	for _, f := range files {
		if err := validateArchiveName(f.Name); err != nil {
			return fmt.Errorf("nome de entrada inválido %q: %w", f.Name, err)
		}
		hdr := &tar.Header{
			Name:     f.Name,
			Mode:     int64(f.Mode.Perm()),
			Size:     int64(len(f.Data)),
			Typeflag: tar.TypeReg,
			ModTime:  now,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("escrevendo header de %s: %w", f.Name, err)
		}
		if _, err := tw.Write(f.Data); err != nil {
			return fmt.Errorf("escrevendo conteúdo de %s: %w", f.Name, err)
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("fechando tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("fechando gzip: %w", err)
	}

	return atomicWriteFile(destPath, buf.Bytes(), 0o600)
}

// extractArchive lê o .tar.gz em srcPath e retorna o conteúdo de cada
// entrada válida, indexado pelo nome. Trata o pacote como entrada não
// confiável: rejeita qualquer entrada que não seja arquivo regular ou
// diretório, e qualquer nome que (após validateArchiveName) seja absoluto
// ou escape do diretório raiz do pacote (proteção contra "tar slip").
func extractArchive(srcPath string) (map[string][]byte, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("abrindo %s: %w", srcPath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("descomprimindo %s: %w", srcPath, err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	files := make(map[string][]byte)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("lendo %s: %w", srcPath, err)
		}

		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("%s: entrada %q com tipo não suportado (%c)", srcPath, hdr.Name, hdr.Typeflag)
		}
		if err := validateArchiveName(hdr.Name); err != nil {
			return nil, fmt.Errorf("%s: entrada %q inválida: %w", srcPath, hdr.Name, err)
		}

		data := make([]byte, hdr.Size)
		if _, err := io.ReadFull(tr, data); err != nil {
			return nil, fmt.Errorf("lendo conteúdo de %s em %s: %w", hdr.Name, srcPath, err)
		}
		files[hdr.Name] = data
	}

	return files, nil
}

// validateArchiveName garante que name é um caminho relativo, sem
// componentes ".." e sem indicar caminho absoluto — defesa clássica contra
// "tar slip"/"zip slip", necessária porque um .tar.gz de backup é sempre
// tratado como entrada potencialmente adulterada.
func validateArchiveName(name string) error {
	if name == "" {
		return fmt.Errorf("nome vazio")
	}
	cleaned := filepath.ToSlash(filepath.Clean(name))
	if strings.HasPrefix(cleaned, "/") {
		return fmt.Errorf("caminho absoluto não permitido")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("caminho escapa do pacote")
	}
	return nil
}
