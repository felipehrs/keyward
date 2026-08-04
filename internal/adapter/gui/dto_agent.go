package main

import "github.com/felipehrs/keyward/core"

// AgentInfoDTO espelha core.AgentInfo — resultado de uma sondagem ao
// ssh-agent no momento da chamada (nunca reaproveita conexão/estado de
// ListKeys, ver core.KeyService.DetectAgent).
type AgentInfoDTO struct {
	Detected   bool   `json:"detected"`
	SocketPath string `json:"socketPath,omitempty"`
	Name       string `json:"name,omitempty"`
}

func agentInfoToDTO(a core.AgentInfo) AgentInfoDTO {
	return AgentInfoDTO{Detected: a.Detected, SocketPath: a.SocketPath, Name: a.Name}
}

// HostLinkDTO espelha core.HostMetadata, com Orphan calculado nesta camada
// (App.ListHostLinks) cruzando HostKey com ConfigService.ListHosts() —
// core.KeyService nunca importa ConfigService (regra de fronteira
// arquitetural, ver CLAUDE.md), então essa detecção não pode viver lá.
type HostLinkDTO struct {
	HostKey             string `json:"hostKey"`
	AgentKeyFingerprint string `json:"agentKeyFingerprint"`
	Notes               string `json:"notes,omitempty"`
	// Orphan é true quando HostKey não corresponde a nenhum host atual de
	// ~/.ssh/config (renomeado ou removido) — spec ssh-agent-support,
	// requisito 5, fluxo de vínculo órfão.
	Orphan bool `json:"orphan"`
}

// LinkHostKeyInput é a entrada de LinkHostKey.
type LinkHostKeyInput struct {
	HostKey             string `json:"hostKey"`
	AgentKeyFingerprint string `json:"agentKeyFingerprint"`
	Notes               string `json:"notes,omitempty"`
}

// UnlinkHostKeyInput é a entrada de UnlinkHostKey.
type UnlinkHostKeyInput struct {
	HostKey             string `json:"hostKey"`
	AgentKeyFingerprint string `json:"agentKeyFingerprint"`
}
