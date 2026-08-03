package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevinburke/ssh_config"
)

// maxIncludeDepth espelha o limite interno da biblioteca ssh_config
// (maxRecurseDepth) para evitar recursão infinita em Include cíclico.
const maxIncludeDepth = 5

// Host é a configuração literal de um bloco `Host` do ssh_config — seja do
// arquivo principal ou de um arquivo trazido por uma diretiva Include.
type Host struct {
	Patterns     []string
	SourceFile   string
	HostName     string
	User         string
	Port         string
	IdentityFile []string
}

// HostSpec descreve o conteúdo desejado de um bloco `Host` para escrita
// (AddHost/ReplaceHost). Não reaproveita Host (o tipo de leitura) porque
// este carrega SourceFile, que não faz sentido como entrada de escrita — o
// arquivo de destino é sempre o parâmetro path, explícito nesses métodos.
type HostSpec struct {
	Patterns     []string
	HostName     string
	User         string
	Port         string
	IdentityFile []string
}

// ConfigService lê e escreve a configuração SSH do usuário (~/.ssh/config),
// seguindo diretivas Include recursivamente por padrão na leitura (decisão
// registrada na spec).
type ConfigService interface {
	// ListHosts retorna um Host para cada bloco `Host` nomeado encontrado no
	// arquivo principal e em qualquer arquivo trazido via Include. O bloco
	// implícito (diretivas antes do primeiro `Host`) e blocos catch-all
	// `Host *` não geram entradas — servem só como origem de Include e de
	// defaults, não como hosts navegáveis.
	ListHosts() ([]Host, error)

	// AddHost escreve um novo bloco Host no arquivo indicado por path (ou
	// no arquivo principal, se path == ""), construído do zero a partir de
	// spec.Patterns e das diretivas suportadas (HostName, User, Port,
	// IdentityFile). Retorna erro se já existir, EM path, um bloco cujo
	// conjunto de Patterns seja idêntico (mesma ordem) a spec.Patterns —
	// nunca sobrescreve implicitamente; para isso use ReplaceHost.
	//
	// Se existir um bloco catch-all `Host *` ao final do arquivo de
	// destino, o novo bloco é inserido imediatamente antes dele, para que
	// suas diretivas tenham precedência sobre os defaults do catch-all
	// (semântica "primeiro valor obtido vence" do ssh_config). Caso
	// contrário, é inserido ao final.
	//
	// Cria path (e diretórios pais) se ainda não existir. AddHost é uma
	// primitiva de baixo nível: se path referenciar um arquivo hoje não
	// alcançado por nenhuma diretiva Include, o arquivo é criado mesmo
	// assim e fica "órfão" até algo garantir esse Include — AddHost não
	// gerencia diretivas Include.
	AddHost(path string, spec HostSpec) error

	// ReplaceHost localiza, EM path (ou no arquivo principal, se path ==
	// ""), o bloco cujo conjunto de Patterns seja idêntico a oldPatterns, e
	// o substitui integralmente pelo conteúdo de newSpec. Retorna erro se
	// nenhum bloco correspondente for encontrado — sem upsert automático,
	// para simetria com AddHost (que erra se já existir).
	//
	// Qualquer comentário/formatação específica do bloco substituído é
	// perdida — o novo bloco é serializado do zero, sem indentação
	// customizada. Blocos não tocados (nesse arquivo ou em qualquer outro
	// trazido via Include) permanecem intactos byte-a-byte.
	ReplaceHost(path string, oldPatterns []string, newSpec HostSpec) error

	// RemoveHost localiza, EM path (ou no arquivo principal, se path ==
	// ""), o bloco cujo conjunto de Patterns seja idêntico a patterns, e o
	// remove por inteiro. Retorna erro se nenhum bloco correspondente for
	// encontrado. Blocos não tocados (nesse arquivo ou em qualquer outro
	// trazido via Include) permanecem intactos byte-a-byte.
	RemoveHost(path string, patterns []string) error
}

// FileConfigService implementa ConfigService lendo/escrevendo a partir do
// disco.
type FileConfigService struct {
	// Path é o arquivo ssh_config principal. Se vazio, usa ~/.ssh/config.
	Path string
}

func NewFileConfigService(path string) *FileConfigService {
	return &FileConfigService{Path: path}
}

// mainPath resolve o arquivo ssh_config principal — s.Path se definido,
// senão ~/.ssh/config. Reaproveitado pela leitura e pela escrita sempre que
// path (parâmetro explícito de AddHost/ReplaceHost) vier vazio.
func (s *FileConfigService) mainPath() (string, error) {
	if s.Path != "" {
		return s.Path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolvendo diretório home: %w", err)
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

func (s *FileConfigService) ListHosts() ([]Host, error) {
	path, err := s.mainPath()
	if err != nil {
		return nil, err
	}
	return listHosts(path, 0)
}

func listHosts(path string, depth uint8) ([]Host, error) {
	if depth > maxIncludeDepth {
		return nil, fmt.Errorf("%s: profundidade máxima de Include (%d) excedida", path, maxIncludeDepth)
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("abrindo %s: %w", path, err)
	}
	defer f.Close()

	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("parseando %s: %w", path, err)
	}

	var hosts []Host
	for _, h := range cfg.Hosts {
		included, err := followIncludes(h, depth)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, included...)

		if isNamedHost(h) {
			hosts = append(hosts, hostFromBlock(h, path))
		}
	}
	return hosts, nil
}

// isNamedHost indica se um bloco `Host` representa um alias navegável, e não
// a seção implícita do topo do arquivo (sem nenhum `Host`) ou um bloco
// catch-all `Host *` usado só para aplicar defaults a todos os hosts.
func isNamedHost(h *ssh_config.Host) bool {
	if len(h.Patterns) == 0 {
		return false
	}
	if len(h.Patterns) == 1 && h.Patterns[0].String() == "*" {
		return false
	}
	return true
}

func hostFromBlock(h *ssh_config.Host, sourceFile string) Host {
	host := Host{SourceFile: sourceFile}
	for _, p := range h.Patterns {
		host.Patterns = append(host.Patterns, p.String())
	}
	for _, node := range h.Nodes {
		kv, ok := node.(*ssh_config.KV)
		if !ok {
			continue
		}
		switch strings.ToLower(kv.Key) {
		case "hostname":
			host.HostName = kv.Value
		case "user":
			host.User = kv.Value
		case "port":
			host.Port = kv.Value
		case "identityfile":
			host.IdentityFile = append(host.IdentityFile, expandHome(kv.Value))
		}
	}
	return host
}

// followIncludes localiza diretivas Include dentro de um bloco Host e
// retorna os hosts encontrados nos arquivos referenciados, seguindo-os
// recursivamente.
//
// A biblioteca kevinburke/ssh_config já resolve Include internamente durante
// Decode, mas não expõe os arquivos resolvidos publicamente — os campos
// `directives` e `files` de Include não são exportados, e Get/GetAll só
// resolvem o valor efetivo de uma chave para um alias, sem enumerar hosts.
// Por isso reconstruímos os argumentos do Include a partir de
// Include.String() (que É exportado e documentado) e refazemos a resolução
// de caminho nós mesmos, replicando as mesmas regras usadas internamente
// pela biblioteca (ver NewInclude em config.go do pacote ssh_config):
// caminho absoluto é usado como está; prefixo "~/" expande para o home;
// caso contrário resolve relativo a ~/.ssh/.
func followIncludes(h *ssh_config.Host, depth uint8) ([]Host, error) {
	var hosts []Host
	for _, node := range h.Nodes {
		inc, ok := node.(*ssh_config.Include)
		if !ok {
			continue
		}
		directives, err := parseIncludeDirectives(inc)
		if err != nil {
			return nil, err
		}
		paths, err := resolveIncludePaths(directives)
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			nested, err := listHosts(p, depth+1)
			if err != nil {
				return nil, err
			}
			hosts = append(hosts, nested...)
		}
	}
	return hosts, nil
}

func parseIncludeDirectives(inc *ssh_config.Include) ([]string, error) {
	line := strings.TrimSpace(inc.String())
	if inc.Comment != "" {
		line = strings.TrimSuffix(line, " #"+inc.Comment)
	}
	fields := strings.Fields(line)
	if len(fields) < 2 || !strings.EqualFold(fields[0], "include") {
		return nil, fmt.Errorf("não foi possível interpretar diretiva Include: %q", line)
	}
	rest := fields[1:]
	if len(rest) > 0 && rest[0] == "=" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return nil, fmt.Errorf("diretiva Include sem argumentos: %q", line)
	}
	return rest, nil
}

func resolveIncludePaths(directives []string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolvendo diretório home: %w", err)
	}

	var matches []string
	seen := map[string]bool{}
	for _, d := range directives {
		var pattern string
		switch {
		case filepath.IsAbs(d):
			pattern = d
		case strings.HasPrefix(d, "~/"):
			pattern = filepath.Join(home, d[2:])
		default:
			pattern = filepath.Join(home, ".ssh", d)
		}
		found, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("expandindo Include %q: %w", d, err)
		}
		for _, m := range found {
			if !seen[m] {
				seen[m] = true
				matches = append(matches, m)
			}
		}
	}
	return matches, nil
}

// expandHome resolve um "~" ou "~/" inicial em caminhos como IdentityFile
// para o diretório home real. Não implementa o restante da tokenização do
// OpenSSH (%d, %h, %r, etc.) — fora de escopo do MVP.
func expandHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
