package core

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// minRSABits é o piso exigido pela spec (seção 5.2.1) para chaves RSA.
const minRSABits = 4096

// defaultRSABits é usado quando GenerateKeyOptions.RSABits é 0.
const defaultRSABits = 4096

// generatedKey é o resultado interno de generateKeyFiles — keys.go monta o
// KeyMetadata/Key públicos a partir dele.
type generatedKey struct {
	privateKeyPath string
	publicKeyPath  string
	fingerprint    string
	bits           int
	comment        string
}

func defaultKeyFileName(algorithm Algorithm) (string, error) {
	switch algorithm {
	case AlgorithmEd25519:
		return "id_ed25519", nil
	case AlgorithmRSA:
		return "id_rsa", nil
	default:
		return "", fmt.Errorf("algoritmo de chave desconhecido: %q", algorithm)
	}
}

func defaultKeyComment() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "keyward"
	}
	return "keyward@" + host
}

// generateKeyFiles gera um novo par de chaves do algoritmo indicado, grava
// os arquivos privado/.pub em dir e retorna os metadados extraídos do par
// recém-criado. Não grava nada em metadata.json — isso é responsabilidade
// de quem chama (FileKeyService.GenerateKey).
func generateKeyFiles(dir string, algorithm Algorithm, opts GenerateKeyOptions) (generatedKey, error) {
	fileName := opts.FileName
	if fileName == "" {
		var err error
		fileName, err = defaultKeyFileName(algorithm)
		if err != nil {
			return generatedKey{}, err
		}
	}

	privatePath := filepath.Join(dir, fileName)
	publicPath := privatePath + ".pub"

	if !opts.Overwrite {
		if _, err := os.Stat(privatePath); err == nil {
			return generatedKey{}, fmt.Errorf("%s já existe (use Overwrite para sobrescrever)", privatePath)
		}
	}

	comment := opts.Comment
	if comment == "" {
		comment = defaultKeyComment()
	}

	var (
		privateKey crypto.PrivateKey
		publicKey  any
		bits       int
	)

	switch algorithm {
	case AlgorithmEd25519:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return generatedKey{}, fmt.Errorf("gerando chave ed25519: %w", err)
		}
		privateKey, publicKey, bits = priv, pub, 256

	case AlgorithmRSA:
		rsaBits := opts.RSABits
		if rsaBits == 0 {
			rsaBits = defaultRSABits
		}
		if rsaBits < minRSABits {
			return generatedKey{}, fmt.Errorf("RSABits deve ser >= %d (recebido %d)", minRSABits, rsaBits)
		}
		priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
		if err != nil {
			return generatedKey{}, fmt.Errorf("gerando chave RSA: %w", err)
		}
		privateKey, publicKey, bits = priv, &priv.PublicKey, rsaBits

	default:
		return generatedKey{}, fmt.Errorf("algoritmo de chave desconhecido: %q", algorithm)
	}

	var block *pem.Block
	var err error
	if len(opts.Passphrase) > 0 {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(privateKey, comment, opts.Passphrase)
	} else {
		block, err = ssh.MarshalPrivateKey(privateKey, comment)
	}
	if err != nil {
		return generatedKey{}, fmt.Errorf("serializando chave privada: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return generatedKey{}, fmt.Errorf("derivando chave pública: %w", err)
	}
	fingerprint := ssh.FingerprintSHA256(sshPub)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return generatedKey{}, fmt.Errorf("criando %s: %w", dir, err)
	}

	if err := writeNewFile(privatePath, pem.EncodeToMemory(block), 0o600); err != nil {
		return generatedKey{}, err
	}

	pubLine := bytes.TrimRight(ssh.MarshalAuthorizedKey(sshPub), "\n")
	pubContent := append(append(pubLine, ' '), []byte(comment+"\n")...)
	if err := writeNewFile(publicPath, pubContent, 0o644); err != nil {
		os.Remove(privatePath)
		return generatedKey{}, err
	}

	return generatedKey{
		privateKeyPath: privatePath,
		publicKeyPath:  publicPath,
		fingerprint:    fingerprint,
		bits:           bits,
		comment:        comment,
	}, nil
}

// writeNewFile grava conteúdo em path com a permissão indicada, permitindo
// sobrescrever um arquivo existente (a checagem de "já existe" é feita antes,
// em generateKeyFiles, condicionada a Overwrite).
//
// No Windows, o modo passado aqui só alterna o atributo read-only do arquivo
// — não restringe acesso via ACL a outros usuários da máquina, diferente do
// comportamento real em Linux/macOS. Gap aceito conscientemente no MVP (ver
// spec, seção 6): a proteção de fato depende da ACL herdada de
// %USERPROFILE%\.ssh.
func writeNewFile(path string, content []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, content, mode); err != nil {
		return fmt.Errorf("escrevendo %s: %w", path, err)
	}
	return nil
}

// fingerprintOfPrivateKey lê o arquivo .pub associado a uma chave privada e
// retorna seu fingerprint e algoritmo. Usado por RegisterKey.
func fingerprintOfPrivateKey(privateKeyPath string) (fingerprint string, algorithm Algorithm, err error) {
	pubBytes, err := os.ReadFile(privateKeyPath + ".pub")
	if err != nil {
		return "", "", fmt.Errorf("lendo %s.pub: %w", privateKeyPath, err)
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey(pubBytes)
	if err != nil {
		return "", "", fmt.Errorf("interpretando %s.pub: %w", privateKeyPath, err)
	}
	algorithm, err = algorithmFromKeyType(pubKey.Type())
	if err != nil {
		return "", "", err
	}
	return ssh.FingerprintSHA256(pubKey), algorithm, nil
}

func algorithmFromKeyType(keyType string) (Algorithm, error) {
	switch keyType {
	case ssh.KeyAlgoED25519:
		return AlgorithmEd25519, nil
	case ssh.KeyAlgoRSA:
		return AlgorithmRSA, nil
	default:
		return "", fmt.Errorf("tipo de chave não suportado pelo keyward: %q", keyType)
	}
}
