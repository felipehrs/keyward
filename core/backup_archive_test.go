package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestArchive_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "backup.tar.gz")

	files := []archiveFile{
		{Name: "manifest.json", Mode: 0o600, Data: []byte(`{"version":1}`)},
		{Name: "keys/id_ed25519", Mode: 0o600, Data: []byte("private key content")},
		{Name: "keys/id_ed25519.pub", Mode: 0o644, Data: []byte("public key content")},
	}

	if err := createArchive(dest, files); err != nil {
		t.Fatalf("createArchive: %v", err)
	}

	got, err := extractArchive(dest)
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	if len(got) != len(files) {
		t.Fatalf("esperava %d entradas, obteve %d: %v", len(files), len(got), got)
	}
	for _, f := range files {
		if string(got[f.Name]) != string(f.Data) {
			t.Errorf("entrada %s: got %q, want %q", f.Name, got[f.Name], f.Data)
		}
	}
}

func TestCreateArchive_RejectsPathTraversalOnWrite(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "backup.tar.gz")

	err := createArchive(dest, []archiveFile{{Name: "../escape", Mode: 0o600, Data: []byte("x")}})
	if err == nil {
		t.Fatal("esperava erro para nome de entrada com ../")
	}
}

func TestExtractArchive_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "malicious.tar.gz")
	writeRawTarGz(t, dest, map[string]string{"../escape": "conteúdo malicioso"})

	if _, err := extractArchive(dest); err == nil {
		t.Fatal("esperava erro ao extrair pacote com ../")
	}
}

func TestExtractArchive_RejectsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "malicious.tar.gz")
	writeRawTarGz(t, dest, map[string]string{"/etc/passwd": "conteúdo malicioso"})

	if _, err := extractArchive(dest); err == nil {
		t.Fatal("esperava erro ao extrair pacote com path absoluto")
	}
}

func TestExtractArchive_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "malicious.tar.gz")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "evil-link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o600}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := extractArchive(dest); err == nil {
		t.Fatal("esperava erro ao extrair pacote com symlink")
	}
}

// writeRawTarGz escreve um .tar.gz com as entradas dadas, sem passar pela
// validação de createArchive — usado para simular pacotes adulterados nos
// testes de extractArchive.
func writeRawTarGz(t *testing.T, dest string, entries map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		hdr := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
