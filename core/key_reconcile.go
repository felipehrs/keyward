package core

import (
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// diskKey é o que scanKeyDir consegue extrair diretamente de um par de
// arquivos em ~/.ssh/, sem nenhuma informação de metadata.json.
type diskKey struct {
	privateKeyPath string
	publicKeyPath  string
	algorithm      Algorithm
	bits           int
	comment        string
}

// scanKeyDir procura pares de chave (arquivo privado + arquivo ".pub"
// irmão) em dir e retorna um mapa indexado por fingerprint. Arquivos sem
// par (config, known_hosts, authorized_keys, ".pub" órfão sem base
// correspondente) são ignorados silenciosamente. Um ".pub" que existe mas
// não parseia como chave SSH válida também é ignorado — não derruba o scan
// inteiro por causa de um arquivo corrompido.
func scanKeyDir(dir string) (map[string]diskKey, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]diskKey{}, nil
		}
		return nil, fmt.Errorf("lendo %s: %w", dir, err)
	}

	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names[e.Name()] = true
		}
	}

	result := make(map[string]diskKey)
	for name := range names {
		if !strings.HasSuffix(name, ".pub") {
			continue
		}
		base := strings.TrimSuffix(name, ".pub")
		if !names[base] {
			continue // .pub órfão, sem chave privada irmã — ignorado
		}

		pubBytes, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		pubKey, comment, _, _, err := ssh.ParseAuthorizedKey(pubBytes)
		if err != nil {
			continue // .pub inválido/corrompido — ignora o par
		}
		algorithm, err := algorithmFromKeyType(pubKey.Type())
		if err != nil {
			continue // algoritmo não suportado pelo keyward — ignora
		}

		bits := 256
		if algorithm == AlgorithmRSA {
			if cpk, ok := pubKey.(ssh.CryptoPublicKey); ok {
				if rsaPub, ok := cpk.CryptoPublicKey().(*rsa.PublicKey); ok {
					bits = rsaPub.N.BitLen()
				}
			}
		}

		fingerprint := ssh.FingerprintSHA256(pubKey)
		result[fingerprint] = diskKey{
			privateKeyPath: filepath.Join(dir, base),
			publicKeyPath:  filepath.Join(dir, name),
			algorithm:      algorithm,
			bits:           bits,
			comment:        comment,
		}
	}
	return result, nil
}

// reconcileKeys casa as chaves encontradas em dir com os registros de
// mf.Keys por fingerprint (spec 5.3.2), calcula os alertas de expiração a
// partir de mf.Settings.AlertThresholdDays (spec 5.3.4) e ordena o
// resultado por proximidade de expiração (spec 5.3.3).
func reconcileKeys(dir string, mf metadataFile) ([]Key, error) {
	onDisk, err := scanKeyDir(dir)
	if err != nil {
		return nil, err
	}

	metaByFingerprint := make(map[string]KeyMetadata, len(mf.Keys))
	for _, m := range mf.Keys {
		metaByFingerprint[m.Fingerprint] = m
	}

	now := time.Now()
	threshold := time.Duration(mf.Settings.AlertThresholdDays) * 24 * time.Hour

	fingerprints := make(map[string]bool, len(onDisk)+len(metaByFingerprint))
	for fp := range onDisk {
		fingerprints[fp] = true
	}
	for fp := range metaByFingerprint {
		fingerprints[fp] = true
	}

	keys := make([]Key, 0, len(fingerprints))
	for fp := range fingerprints {
		disk, hasDisk := onDisk[fp]
		meta, hasMeta := metaByFingerprint[fp]

		k := Key{}
		switch {
		case hasDisk && hasMeta:
			k.Status = KeyStatusOK
		case hasDisk && !hasMeta:
			k.Status = KeyStatusUnregistered
		case hasMeta && !hasDisk:
			k.Status = KeyStatusMissingFile
		}

		if hasDisk {
			k.PrivateKeyPath = disk.privateKeyPath
			k.PublicKeyPath = disk.publicKeyPath
			k.Algorithm = disk.algorithm
			k.KeyBits = disk.bits
			k.Comment = disk.comment
		}
		if hasMeta {
			k.Metadata = meta
			if !hasDisk {
				k.Algorithm = meta.Algorithm
			}
			if meta.ExpiresAt != nil {
				exp := *meta.ExpiresAt
				k.IsExpired = exp.Before(now)
				if !k.IsExpired {
					k.IsExpiringSoon = exp.Before(now.Add(threshold))
				}
			}
		}

		keys = append(keys, k)
	}

	sortKeys(keys)
	return keys, nil
}

// sortKeys ordena por proximidade de expiração (spec 5.3.3): expiradas
// primeiro (mais antiga primeiro), depois expirando dentro do threshold
// (mais próxima primeiro), depois demais com ExpiresAt futuro distante
// (mais próxima primeiro também), depois chaves sem ExpiresAt conhecido —
// nesse último grupo, desempate por CreatedAt decrescente (mais recente
// primeiro); chaves KeyStatusUnregistered sem metadata têm CreatedAt zero e
// naturalmente vão para o final desse grupo.
func sortKeys(keys []Key) {
	sort.SliceStable(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		ra, rb := expirationRank(a), expirationRank(b)
		if ra != rb {
			return ra < rb
		}
		if ra < 3 {
			return a.Metadata.ExpiresAt.Before(*b.Metadata.ExpiresAt)
		}
		return a.Metadata.CreatedAt.After(b.Metadata.CreatedAt)
	})
}

func expirationRank(k Key) int {
	switch {
	case k.Metadata.ExpiresAt == nil:
		return 3
	case k.IsExpired:
		return 0
	case k.IsExpiringSoon:
		return 1
	default:
		return 2
	}
}
