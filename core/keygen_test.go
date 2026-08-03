package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateKeyFiles_Ed25519Defaults(t *testing.T) {
	dir := t.TempDir()

	got, err := generateKeyFiles(dir, AlgorithmEd25519, GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("generateKeyFiles: %v", err)
	}

	if got.privateKeyPath != filepath.Join(dir, "id_ed25519") {
		t.Errorf("privateKeyPath = %q", got.privateKeyPath)
	}
	if got.publicKeyPath != filepath.Join(dir, "id_ed25519.pub") {
		t.Errorf("publicKeyPath = %q", got.publicKeyPath)
	}
	if got.bits != 256 {
		t.Errorf("bits = %d, esperado 256", got.bits)
	}

	privPEM, err := os.ReadFile(got.privateKeyPath)
	if err != nil {
		t.Fatalf("lendo chave privada: %v", err)
	}
	if _, err := ssh.ParsePrivateKey(privPEM); err != nil {
		t.Errorf("chave privada gerada não parseia com ssh.ParsePrivateKey: %v", err)
	}

	pubBytes, err := os.ReadFile(got.publicKeyPath)
	if err != nil {
		t.Fatalf("lendo chave pública: %v", err)
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(pubBytes)
	if err != nil {
		t.Fatalf("chave pública gerada não parseia: %v", err)
	}
	if ssh.FingerprintSHA256(pubKey) != got.fingerprint {
		t.Errorf("fingerprint do .pub em disco (%s) difere do retornado (%s)", ssh.FingerprintSHA256(pubKey), got.fingerprint)
	}
}

func TestGenerateKeyFiles_RSAMinimumBits(t *testing.T) {
	dir := t.TempDir()

	if _, err := generateKeyFiles(dir, AlgorithmRSA, GenerateKeyOptions{RSABits: 2048}); err == nil {
		t.Fatal("esperava erro para RSABits < 4096")
	}

	got, err := generateKeyFiles(dir, AlgorithmRSA, GenerateKeyOptions{RSABits: 0, FileName: "id_rsa_default"})
	if err != nil {
		t.Fatalf("generateKeyFiles com RSABits=0: %v", err)
	}
	if got.bits != defaultRSABits {
		t.Errorf("bits = %d, esperado default %d", got.bits, defaultRSABits)
	}
}

func TestGenerateKeyFiles_WithPassphrase(t *testing.T) {
	dir := t.TempDir()
	passphrase := []byte("correct horse battery staple")

	got, err := generateKeyFiles(dir, AlgorithmEd25519, GenerateKeyOptions{Passphrase: passphrase})
	if err != nil {
		t.Fatalf("generateKeyFiles: %v", err)
	}

	privPEM, err := os.ReadFile(got.privateKeyPath)
	if err != nil {
		t.Fatalf("lendo chave privada: %v", err)
	}

	if _, err := ssh.ParsePrivateKey(privPEM); err == nil {
		t.Error("esperava erro ao parsear sem passphrase uma chave protegida")
	}
	if _, err := ssh.ParsePrivateKeyWithPassphrase(privPEM, []byte("senha errada")); err == nil {
		t.Error("esperava erro com passphrase incorreta")
	}
	if _, err := ssh.ParsePrivateKeyWithPassphrase(privPEM, passphrase); err != nil {
		t.Errorf("passphrase correta deveria parsear, erro: %v", err)
	}
}

func TestGenerateKeyFiles_WithoutPassphrase(t *testing.T) {
	dir := t.TempDir()

	got, err := generateKeyFiles(dir, AlgorithmEd25519, GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("generateKeyFiles: %v", err)
	}

	privPEM, err := os.ReadFile(got.privateKeyPath)
	if err != nil {
		t.Fatalf("lendo chave privada: %v", err)
	}
	if _, err := ssh.ParsePrivateKey(privPEM); err != nil {
		t.Errorf("chave sem passphrase deveria parsear direto, erro: %v", err)
	}
}

func TestGenerateKeyFiles_RefusesOverwriteByDefault(t *testing.T) {
	dir := t.TempDir()

	if _, err := generateKeyFiles(dir, AlgorithmEd25519, GenerateKeyOptions{FileName: "id_ed25519"}); err != nil {
		t.Fatalf("primeira geração: %v", err)
	}

	if _, err := generateKeyFiles(dir, AlgorithmEd25519, GenerateKeyOptions{FileName: "id_ed25519"}); err == nil {
		t.Fatal("esperava erro ao gerar sobre arquivo existente sem Overwrite")
	}

	if _, err := generateKeyFiles(dir, AlgorithmEd25519, GenerateKeyOptions{FileName: "id_ed25519", Overwrite: true}); err != nil {
		t.Fatalf("com Overwrite=true deveria funcionar, erro: %v", err)
	}
}

func TestGenerateKeyFiles_FingerprintFormat(t *testing.T) {
	dir := t.TempDir()
	got, err := generateKeyFiles(dir, AlgorithmEd25519, GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("generateKeyFiles: %v", err)
	}
	if !strings.HasPrefix(got.fingerprint, "SHA256:") {
		t.Errorf("fingerprint = %q, esperado prefixo SHA256:", got.fingerprint)
	}
}

func TestGenerateKeyFiles_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissões POSIX não se aplicam da mesma forma no Windows — ver limitação documentada em keygen.go")
	}

	dir := t.TempDir()
	got, err := generateKeyFiles(dir, AlgorithmEd25519, GenerateKeyOptions{})
	if err != nil {
		t.Fatalf("generateKeyFiles: %v", err)
	}

	info, err := os.Stat(got.privateKeyPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissão da chave privada = %o, esperado 600", perm)
	}
}
