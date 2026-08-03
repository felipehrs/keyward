package core

import (
	"fmt"
	"os"
	"path/filepath"
)

// atomicWriteFile grava data em path de forma atômica: cria o diretório de
// destino se necessário, escreve em um arquivo temporário no mesmo
// diretório (garante que o rename fique no mesmo volume), sincroniza,
// ajusta a permissão e usa os.Rename para substituir o destino. os.Rename é
// atômico tanto em POSIX quanto no Windows, então um crash no meio da
// escrita deixa o arquivo original intacto ou o novo completo — nunca um
// arquivo truncado.
//
// Compartilhado por metadataStore.save (core/metadata.go) e pela escrita de
// ~/.ssh/config (core/config_write.go), para não duplicar essa lógica.
func atomicWriteFile(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("criando %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("criando arquivo temporário em %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("escrevendo %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sincronizando %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("fechando %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("ajustando permissão de %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("substituindo %s: %w", path, err)
	}
	renamed = true
	return nil
}
