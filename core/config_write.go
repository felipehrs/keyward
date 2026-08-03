package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevinburke/ssh_config"
)

func (s *FileConfigService) AddHost(path string, spec HostSpec) error {
	target, err := s.resolveTargetPath(path)
	if err != nil {
		return err
	}

	return s.mutate(target, func(cfg *ssh_config.Config) error {
		for _, h := range cfg.Hosts {
			if isNamedHost(h) && patternsEqual(patternStrings(h), spec.Patterns) {
				return fmt.Errorf("já existe um bloco Host com os patterns %v em %s", spec.Patterns, target)
			}
		}

		newHost, err := buildSSHHost(spec)
		if err != nil {
			return err
		}

		idx := insertionIndex(cfg.Hosts)
		cfg.Hosts = append(cfg.Hosts, nil)
		copy(cfg.Hosts[idx+1:], cfg.Hosts[idx:])
		cfg.Hosts[idx] = newHost
		return nil
	})
}

func (s *FileConfigService) ReplaceHost(path string, oldPatterns []string, newSpec HostSpec) error {
	target, err := s.resolveTargetPath(path)
	if err != nil {
		return err
	}

	return s.mutate(target, func(cfg *ssh_config.Config) error {
		for i, h := range cfg.Hosts {
			if !isNamedHost(h) || !patternsEqual(patternStrings(h), oldPatterns) {
				continue
			}
			newHost, err := buildSSHHost(newSpec)
			if err != nil {
				return err
			}
			cfg.Hosts[i] = newHost
			return nil
		}
		return fmt.Errorf("nenhum bloco Host com os patterns %v encontrado em %s", oldPatterns, target)
	})
}

func (s *FileConfigService) RemoveHost(path string, patterns []string) error {
	target, err := s.resolveTargetPath(path)
	if err != nil {
		return err
	}

	return s.mutate(target, func(cfg *ssh_config.Config) error {
		for i, h := range cfg.Hosts {
			if !isNamedHost(h) || !patternsEqual(patternStrings(h), patterns) {
				continue
			}
			cfg.Hosts = append(cfg.Hosts[:i], cfg.Hosts[i+1:]...)
			return nil
		}
		return fmt.Errorf("nenhum bloco Host com os patterns %v encontrado em %s", patterns, target)
	})
}

func (s *FileConfigService) resolveTargetPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	return s.mainPath()
}

// mutate decodifica target (ou parte de um Config vazio, se o arquivo ainda
// não existir), aplica fn sobre o *ssh_config.Config em memória e, se fn não
// retornar erro, reserializa o Config inteiro e sobrescreve target
// atomicamente. Blocos Host não tocados por fn preservam seu rawValue
// original (ver achados no plano) e saem byte-a-byte idênticos ao que
// entrou.
func (s *FileConfigService) mutate(target string, fn func(cfg *ssh_config.Config) error) error {
	cfg, perm, err := loadOrCreateConfig(target)
	if err != nil {
		return err
	}

	if err := fn(cfg); err != nil {
		return err
	}

	return atomicWriteFile(target, []byte(cfg.String()), perm)
}

// loadOrCreateConfig decodifica target. Se o arquivo não existir, retorna um
// *ssh_config.Config vazio (só com o host implícito que a própria lib
// sempre cria) e a permissão default 0600 para um arquivo novo. Se existir,
// retorna o Config decodificado e a permissão atual do arquivo, para
// preservá-la na regravação (ex. não forçar 0600 sobre um ~/.ssh/config que
// já era 644).
func loadOrCreateConfig(target string) (*ssh_config.Config, os.FileMode, error) {
	f, err := os.Open(target)
	if err != nil {
		if os.IsNotExist(err) {
			cfg, decErr := ssh_config.Decode(strings.NewReader(""))
			if decErr != nil {
				return nil, 0, fmt.Errorf("criando config vazio: %w", decErr)
			}
			return cfg, 0o600, nil
		}
		return nil, 0, fmt.Errorf("abrindo %s: %w", target, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("obtendo informações de %s: %w", target, err)
	}

	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil, 0, fmt.Errorf("parseando %s: %w", target, err)
	}
	return cfg, info.Mode().Perm(), nil
}

// buildSSHHost constrói um *ssh_config.Host inteiramente novo a partir de
// spec. Só os campos exportados de Host/KV são usados (Patterns, Nodes,
// Key, Value) — os demais (indentação, "Host = x" vs "Host x", etc.) ficam
// com zero value, o que produz formatação limpa e padrão, sem tentar imitar
// um estilo específico (não há como, para um bloco novo).
func buildSSHHost(spec HostSpec) (*ssh_config.Host, error) {
	if len(spec.Patterns) == 0 {
		return nil, fmt.Errorf("host sem Patterns")
	}

	patterns := make([]*ssh_config.Pattern, 0, len(spec.Patterns))
	for _, p := range spec.Patterns {
		pat, err := ssh_config.NewPattern(p)
		if err != nil {
			return nil, fmt.Errorf("pattern %q inválido: %w", p, err)
		}
		patterns = append(patterns, pat)
	}

	var nodes []ssh_config.Node
	if spec.HostName != "" {
		nodes = append(nodes, &ssh_config.KV{Key: "HostName", Value: spec.HostName})
	}
	if spec.User != "" {
		nodes = append(nodes, &ssh_config.KV{Key: "User", Value: spec.User})
	}
	if spec.Port != "" {
		nodes = append(nodes, &ssh_config.KV{Key: "Port", Value: spec.Port})
	}
	for _, idf := range spec.IdentityFile {
		nodes = append(nodes, &ssh_config.KV{Key: "IdentityFile", Value: compactHome(idf)})
	}

	return &ssh_config.Host{Patterns: patterns, Nodes: nodes}, nil
}

// insertionIndex decide onde um novo bloco Host deve entrar em hosts: se o
// último elemento for um bloco catch-all "Host *" (e não o host implícito
// do índice 0, que a lib sempre cria mesmo em arquivo vazio), insere
// imediatamente antes dele — para que as diretivas do novo host tenham
// precedência sobre os defaults do catch-all, respeitando a semântica
// "primeiro valor obtido vence" do ssh_config. Caso contrário, insere ao
// final.
//
// Limitação conhecida: só detecta um catch-all à direita — múltiplos
// "Host *" consecutivos no fim (incomum) só são contornados pelo último.
func insertionIndex(hosts []*ssh_config.Host) int {
	last := len(hosts) - 1
	if last >= 1 && !isNamedHost(hosts[last]) {
		return last
	}
	return len(hosts)
}

func patternStrings(h *ssh_config.Host) []string {
	out := make([]string, 0, len(h.Patterns))
	for _, p := range h.Patterns {
		out = append(out, p.String())
	}
	return out
}

// patternsEqual compara duas listas de patterns por igualdade exata,
// posição a posição — critério simples e previsível para identificar "o
// mesmo bloco Host", já que a lib não expõe nenhum identificador estável.
func patternsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// compactHome é o inverso de expandHome: se path estiver sob o diretório
// home do usuário atual, retorna a forma compacta com "~/", em vez do
// caminho absoluto específico desta máquina. Evita que IdentityFile gravado
// por AddHost/ReplaceHost vaze um caminho que não existe em outra máquina
// (relevante sobretudo para o BackupService, que restaura em outro host).
// Caminhos fora do home, ou quando o home não é resolvível, passam
// intactos.
func compactHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	if rel == "." {
		return "~"
	}
	return "~/" + filepath.ToSlash(rel)
}
