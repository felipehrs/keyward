//go:build !windows

// Package sshagent fornece um dial multiplataforma para ssh-agent, com
// assinatura compatível com core.FileKeyService.AgentDial. Vive fora de
// core/ porque a variante Windows depende de github.com/Microsoft/go-winio,
// não permitida pela regra core-purity do depguard (ver CLAUDE.md, seção
// Arquitetura).
package sshagent

import (
	"net"
	"os"
	"time"
)

// Dial resolve SSH_AUTH_SOCK e conecta via Unix domain socket, respeitando
// o timeout recebido. Espelha core.defaultAgentDial — existe aqui por
// uniformidade de API do pacote entre plataformas, não por necessidade
// funcional (em Unix/macOS, core já cobre esse caso sozinho via stdlib).
func Dial(timeout time.Duration) (net.Conn, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, os.ErrNotExist
	}
	return net.DialTimeout("unix", sock, timeout)
}
