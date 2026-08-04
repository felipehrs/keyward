//go:build windows

package sshagent

import (
	"net"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
)

// defaultPipePath é o named pipe padrão do OpenSSH for Windows
// (Win32-OpenSSH), usado tanto pelo ssh-agent nativo quanto — por
// convenção de compatibilidade — pelo agente de encaminhamento do
// 1Password (ver docs/specs/ssh-agent-support.md, seção 7).
const defaultPipePath = `\\.\pipe\openssh-ssh-agent`

// Dial conecta ao ssh-agent do Windows via named pipe, usando
// SSH_AUTH_SOCK como override de caminho quando definido — o Windows
// OpenSSH aceita esse mesmo nome de variável para apontar para um pipe
// alternativo, mantendo paridade com o Unix — e caindo para o pipe padrão
// caso contrário. O timeout é repassado a go-winio.DialPipe, que retorna
// erro (não trava) quando o pipe não existe ou não aceita a conexão dentro
// do prazo.
func Dial(timeout time.Duration) (net.Conn, error) {
	path := os.Getenv("SSH_AUTH_SOCK")
	if path == "" {
		path = defaultPipePath
	}
	return winio.DialPipe(path, &timeout)
}
