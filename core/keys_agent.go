package core

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// AgentInfo descreve o resultado de uma sondagem a um ssh-agent (spec
// ssh-agent-support, requisito 7).
type AgentInfo struct {
	// Detected é true assim que o dial e a listagem de identidades tiverem
	// sucesso, mesmo que a lista venha vazia (agente presente, mas sem
	// chaves oferecidas — ex. cofre do 1Password bloqueado).
	Detected bool
	// SocketPath é o valor de SSH_AUTH_SOCK usado no dial. "" quando
	// Detected == false.
	SocketPath string
	// Name identifica o agente por heurística de caminho de socket (ex.
	// "1password"). "" para agente genérico não identificado ou quando
	// Detected == false.
	Name string
}

// defaultAgentDial é o AgentDial padrão de FileKeyService quando o campo
// está nil: resolve SSH_AUTH_SOCK e dial um Unix domain socket, só com
// stdlib (net.Dial), respeitando o timeout recebido do chamador.
func defaultAgentDial(timeout time.Duration) (net.Conn, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, os.ErrNotExist
	}
	return net.DialTimeout("unix", sock, timeout)
}

// agentName aplica a heurística de identificação por caminho de socket:
// caminhos contendo "1password" (case-insensitive) são identificados como
// "1password"; qualquer outro caminho é tratado como agente genérico ("").
func agentName(socketPath string) string {
	if strings.Contains(strings.ToLower(socketPath), "1password") {
		return "1password"
	}
	return ""
}

// dialResult carrega o resultado assíncrono de uma chamada a AgentDial.
type dialResult struct {
	conn net.Conn
	err  error
}

// dialWithTimeout executa dial em uma goroutine separada e impõe timeout
// como um limite superior duro no lado do core, independente de a
// implementação de dial respeitá-lo ou não — uma implementação de teste que
// nunca retorna não deve travar ListKeys()/DetectAgent() além do timeout
// configurado.
func dialWithTimeout(dial func(timeout time.Duration) (net.Conn, error), timeout time.Duration) (net.Conn, error) {
	ch := make(chan dialResult, 1)
	go func() {
		conn, err := dial(timeout)
		ch <- dialResult{conn: conn, err: err}
	}()

	select {
	case res := <-ch:
		return res.conn, res.err
	case <-time.After(timeout):
		// A goroutine de dial pode continuar rodando em segundo plano se a
		// implementação nunca retornar; fechamos a conexão dela (se algum
		// dia chegar) para não vazar o socket.
		go func() {
			if res := <-ch; res.conn != nil {
				_ = res.conn.Close()
			}
		}()
		return nil, os.ErrDeadlineExceeded
	}
}

// detectAgent sonda um ssh-agent com um dial novo (sem reaproveitar
// conexão ou estado de ListKeys), retornando Detected == true assim que
// dial + List() tiverem sucesso.
func detectAgent(dial func(timeout time.Duration) (net.Conn, error), timeout time.Duration) (AgentInfo, error) {
	conn, err := dialWithTimeout(dial, timeout)
	if err != nil {
		return AgentInfo{}, nil
	}
	defer func() { _ = conn.Close() }()

	client := agent.NewClient(conn)
	if _, err := client.List(); err != nil {
		return AgentInfo{}, nil
	}

	socketPath := os.Getenv("SSH_AUTH_SOCK")
	return AgentInfo{
		Detected:   true,
		SocketPath: socketPath,
		Name:       agentName(socketPath),
	}, nil
}

// reconcileAgentKeys cruza as identidades oferecidas por um ssh-agent com
// os registros de mf.Keys cujo Source == KeySourceAgent, espelhando a forma
// de reconcileKeys (core/key_reconcile.go) mas sem varrer disco.
//
// Qualquer falha de conexão (SSH_AUTH_SOCK ausente, dial com erro/timeout)
// faz TODOS os registros de origem agente virarem KeyStatusAgentOffline —
// não só os que não têm correspondência, já que a falta de conexão não
// permite confirmar nenhuma identidade.
func reconcileAgentKeys(dial func(timeout time.Duration) (net.Conn, error), timeout time.Duration, mf metadataFile) []Key {
	agentMetaByFingerprint := make(map[string]KeyMetadata)
	for _, m := range mf.Keys {
		if m.Source == KeySourceAgent {
			agentMetaByFingerprint[m.Fingerprint] = m
		}
	}

	conn, err := dialWithTimeout(dial, timeout)
	if err != nil {
		return offlineAgentKeys(agentMetaByFingerprint)
	}
	defer func() { _ = conn.Close() }()

	client := agent.NewClient(conn)
	identities, err := client.List()
	if err != nil {
		return offlineAgentKeys(agentMetaByFingerprint)
	}

	now := time.Now()
	threshold := time.Duration(mf.Settings.AlertThresholdDays) * 24 * time.Hour

	offered := make(map[string]*agent.Key, len(identities))
	for _, id := range identities {
		pubKey, err := ssh.ParsePublicKey(id.Marshal())
		if err != nil {
			continue
		}
		fingerprint := ssh.FingerprintSHA256(pubKey)
		offered[fingerprint] = id
	}

	fingerprints := make(map[string]bool, len(offered)+len(agentMetaByFingerprint))
	for fp := range offered {
		fingerprints[fp] = true
	}
	for fp := range agentMetaByFingerprint {
		fingerprints[fp] = true
	}

	keys := make([]Key, 0, len(fingerprints))
	for fp := range fingerprints {
		id, hasIdentity := offered[fp]
		meta, hasMeta := agentMetaByFingerprint[fp]

		k := Key{Source: KeySourceAgent}
		switch {
		case hasIdentity && hasMeta:
			k.Status = KeyStatusOK
		case hasIdentity && !hasMeta:
			k.Status = KeyStatusUnregistered
			// Sem registro de metadata ainda, mas a identidade tem um
			// fingerprint real oferecido pelo agente agora — expõe como
			// handle estável (equivalente ao PrivateKeyPath do caso
			// file-based) para que a UI consiga anotá-la via
			// RegisterAgentKey sem precisar reconciliar de novo. Os demais
			// campos de Metadata permanecem zerados: não há registro de
			// verdade ainda.
			k.Metadata.Fingerprint = fp
		case hasMeta && !hasIdentity:
			k.Status = KeyStatusAgentOffline
		}

		if hasIdentity {
			algorithm, algErr := algorithmFromKeyType(id.Type())
			if algErr == nil {
				k.Algorithm = algorithm
			}
			k.Comment = id.Comment
		}
		if hasMeta {
			k.Metadata = meta
			k.AgentName = meta.AgentName
			if !hasIdentity {
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

	return keys
}

// agentOfferedIdentityAlgorithm sonda o agente com um dial novo e confirma
// que fingerprint corresponde a uma identidade oferecida agora — usado por
// RegisterAgentKey para recusar registrar uma identidade que o agente não
// oferece mais (evita criar metadata órfã de uma identidade que nunca
// existiu de verdade nesse dial).
func agentOfferedIdentityAlgorithm(dial func(timeout time.Duration) (net.Conn, error), timeout time.Duration, fingerprint string) (Algorithm, error) {
	conn, err := dialWithTimeout(dial, timeout)
	if err != nil {
		return "", fmt.Errorf("keyward: ssh-agent inacessível: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client := agent.NewClient(conn)
	identities, err := client.List()
	if err != nil {
		return "", fmt.Errorf("keyward: listando identidades do ssh-agent: %w", err)
	}

	for _, id := range identities {
		pubKey, err := ssh.ParsePublicKey(id.Marshal())
		if err != nil {
			continue
		}
		if ssh.FingerprintSHA256(pubKey) != fingerprint {
			continue
		}
		return algorithmFromKeyType(id.Type())
	}

	return "", fmt.Errorf("keyward: identidade %s não é oferecida pelo ssh-agent no momento", fingerprint)
}

// offlineAgentKeys constrói a visão de Key para todo registro de metadata
// de origem agente quando o ssh-agent está inacessível — usado tanto para
// "sem SSH_AUTH_SOCK" quanto para falha/timeout de dial ou de List().
func offlineAgentKeys(agentMetaByFingerprint map[string]KeyMetadata) []Key {
	keys := make([]Key, 0, len(agentMetaByFingerprint))
	for _, meta := range agentMetaByFingerprint {
		keys = append(keys, Key{
			Metadata:  meta,
			Algorithm: meta.Algorithm,
			Source:    KeySourceAgent,
			AgentName: meta.AgentName,
			Status:    KeyStatusAgentOffline,
		})
	}
	return keys
}
