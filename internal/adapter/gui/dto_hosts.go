package main

import "github.com/felipehrs/keyward/core"

// HostDTO espelha core.Host — sem armadilhas de serialização (nenhum campo
// problemático), mapeamento direto.
type HostDTO struct {
	Patterns     []string `json:"patterns"`
	SourceFile   string   `json:"sourceFile"`
	HostName     string   `json:"hostName,omitempty"`
	User         string   `json:"user,omitempty"`
	Port         string   `json:"port,omitempty"`
	IdentityFile []string `json:"identityFile,omitempty"`
}

func hostToDTO(h core.Host) HostDTO {
	return HostDTO{
		Patterns:     h.Patterns,
		SourceFile:   h.SourceFile,
		HostName:     h.HostName,
		User:         h.User,
		Port:         h.Port,
		IdentityFile: h.IdentityFile,
	}
}

func hostsToDTO(hosts []core.Host) []HostDTO {
	out := make([]HostDTO, len(hosts))
	for i, h := range hosts {
		out[i] = hostToDTO(h)
	}
	return out
}

// toCore é o inverso de hostToDTO — usado por Export, que recebe de volta
// os HostDTO completos que o frontend já carregou via ListHosts (evita um
// round-trip: o frontend não precisa re-consultar o core pra montar a
// seleção).
func (h HostDTO) toCore() core.Host {
	return core.Host{
		Patterns:     h.Patterns,
		SourceFile:   h.SourceFile,
		HostName:     h.HostName,
		User:         h.User,
		Port:         h.Port,
		IdentityFile: h.IdentityFile,
	}
}

// HostSpecInput espelha core.HostSpec — usado como entrada de AddHost e
// ReplaceHost (não tem SourceFile: o destino é sempre um parâmetro
// separado do método, igual ao core).
type HostSpecInput struct {
	Patterns     []string `json:"patterns"`
	HostName     string   `json:"hostName,omitempty"`
	User         string   `json:"user,omitempty"`
	Port         string   `json:"port,omitempty"`
	IdentityFile []string `json:"identityFile,omitempty"`
}

func (in HostSpecInput) toCore() core.HostSpec {
	return core.HostSpec{
		Patterns:     in.Patterns,
		HostName:     in.HostName,
		User:         in.User,
		Port:         in.Port,
		IdentityFile: in.IdentityFile,
	}
}

// ReplaceHostInput identifica o bloco a substituir (SourceFile +
// OldPatterns, ambos vindos de um HostDTO já carregado via ListHosts) e o
// conteúdo novo.
type ReplaceHostInput struct {
	SourceFile  string        `json:"sourceFile"`
	OldPatterns []string      `json:"oldPatterns"`
	NewSpec     HostSpecInput `json:"newSpec"`
}

// RemoveHostInput identifica o bloco a remover — mesma lógica de
// SourceFile+Patterns de ReplaceHostInput.
type RemoveHostInput struct {
	SourceFile string   `json:"sourceFile"`
	Patterns   []string `json:"patterns"`
}
