package main

import "strings"

// HostKey calcula a chave estável usada para vincular um host de
// ~/.ssh/config a uma chave de agente (core.HostMetadata.HostKey, opaca
// para core.KeyService — a convenção é definida e aplicada pelo chamador,
// spec ssh-agent-support seção 4). O frontend chama isso antes de
// LinkHostKey/UnlinkHostKey pra obter a mesma chave usada por ListHostLinks.
func (a *App) HostKey(patterns []string) string {
	return strings.Join(patterns, "\x00")
}

// DetectAgent sonda a presença de um ssh-agent acessível no momento da
// chamada, sempre com um dial novo — nunca reaproveita estado de
// ListKeys().
func (a *App) DetectAgent() (AgentInfoDTO, error) {
	info, err := a.keySvc.DetectAgent()
	if err != nil {
		return AgentInfoDTO{}, mapErr(err)
	}
	return agentInfoToDTO(info), nil
}

// RegisterAgentKey anota (Label/Notes/ExpiresAt) uma identidade já
// oferecida por um ssh-agent, mas ainda sem registro de metadata
// (KeyStatusUnregistered, Source == KeySourceAgent) — espelha RegisterKey,
// mas identifica a chave por fingerprint (já exposto por ListKeys pra esse
// caso, ver core/keys_agent.go) em vez de um caminho de arquivo.
func (a *App) RegisterAgentKey(in RegisterAgentKeyInput) (KeyDTO, error) {
	patch, err := in.toCorePatch()
	if err != nil {
		return KeyDTO{}, err
	}
	key, err := a.keySvc.RegisterAgentKey(in.Fingerprint, patch)
	if err != nil {
		return KeyDTO{}, mapErr(err)
	}
	return keyToDTO(key), nil
}

// LinkHostKey persiste um vínculo informativo entre um host e uma
// identidade de ssh-agent — nunca escreve IdentityFile em ~/.ssh/config
// (isso é uma ação separada, via ReplaceHost). Chamar de novo com o mesmo
// par (HostKey, AgentKeyFingerprint) é idempotente (atualiza Notes).
func (a *App) LinkHostKey(in LinkHostKeyInput) error {
	return mapErr(a.keySvc.LinkHostKey(in.HostKey, in.AgentKeyFingerprint, in.Notes))
}

// UnlinkHostKey remove o vínculo identificado pelo par (HostKey,
// AgentKeyFingerprint). Erro "[key_not_found]" não se aplica aqui —
// core.ErrHostLinkNotFound atravessa como texto pt-BR direto (sem
// sentinela mapeado, mesma regra dos demais erros não mapeados de mapErr).
func (a *App) UnlinkHostKey(in UnlinkHostKeyInput) error {
	return mapErr(a.keySvc.UnlinkHostKey(in.HostKey, in.AgentKeyFingerprint))
}

// ListHostLinks retorna todos os vínculos host/chave-de-agente persistidos,
// com Orphan calculado cruzando cada HostKey contra os hosts atuais de
// ConfigService.ListHosts() — a detecção de vínculo órfão é responsabilidade
// desta camada (App), nunca de core.KeyService.ListHostLinks (que retorna
// os vínculos verbatim, sem cruzar com ConfigService).
func (a *App) ListHostLinks() ([]HostLinkDTO, error) {
	links, err := a.keySvc.ListHostLinks()
	if err != nil {
		return nil, mapErr(err)
	}
	hosts, err := a.configSvc.ListHosts()
	if err != nil {
		return nil, mapErr(err)
	}

	currentHostKeys := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		currentHostKeys[a.HostKey(h.Patterns)] = true
	}

	out := make([]HostLinkDTO, len(links))
	for i, l := range links {
		out[i] = HostLinkDTO{
			HostKey:             l.HostKey,
			AgentKeyFingerprint: l.AgentKeyFingerprint,
			Notes:               l.Notes,
			Orphan:              !currentHostKeys[l.HostKey],
		}
	}
	return out, nil
}
