package main

import "github.com/charmbracelet/lipgloss"

// Convenções visuais compartilhadas por toda a TUI. Usam
// lipgloss.AdaptiveColor (downgrade automático de truecolor para 256/16
// cores, e escolha de tonalidade clara/escura) — mas nenhum estado crítico
// depende só de cor: labels de status também mudam de texto (ex.
// "EXPIRADA"), já que alguns terminais não respondem à query de tema do
// lipgloss e caem num default fixo.
var (
	// StyleTitle destaca títulos de tela e cabeçalhos.
	StyleTitle = lipgloss.NewStyle().Bold(true)

	// StyleMuted marca conteúdo secundário: campos vazios ("-"),
	// SourceFile, e status KeyStatusUnregistered/KeyStatusMissingFile.
	StyleMuted = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "245", Dark: "241"})

	// StyleOK marca uma chave sem alerta de expiração.
	StyleOK = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "42"})

	// StyleWarn marca uma chave expirando dentro de AlertThresholdDays
	// (Key.IsExpiringSoon).
	StyleWarn = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"}).
			Bold(true)

	// StyleDanger marca uma chave expirada (Key.IsExpired), um erro, ou o
	// estado "confirmar" de uma ação destrutiva.
	StyleDanger = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"}).
			Bold(true)

	// StyleHighContrastWarn é o aviso de alto contraste exigido pela spec
	// (seção 6) antes de incluir chave privada num export.
	StyleHighContrastWarn = lipgloss.NewStyle().
				Background(lipgloss.AdaptiveColor{Light: "196", Dark: "196"}).
				Foreground(lipgloss.Color("15")).
				Bold(true).
				Padding(0, 1)

	// StyleSelected marca o item com foco numa lista ou menu.
	StyleSelected = lipgloss.NewStyle().Reverse(true)

	// StyleHelp é o rodapé de atalhos contextual.
	StyleHelp = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "245", Dark: "241"})

	// StyleAgentBadge marca a origem de uma chave de agente (AgentName ou
	// "Agente SSH" genérico) na listagem — visualmente diferente de
	// StyleMuted pra não ser confundida com um status de erro/aviso.
	StyleAgentBadge = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "25", Dark: "39"})

	// StyleAgentOffline marca KeyStatusAgentOffline: distinto tanto de
	// StyleMuted (KeyStatusMissingFile, que é uma condição estável — o
	// registro só existe até alguém o remover) quanto de StyleDanger
	// (chave expirada) — é um estado tipicamente transitório (agente
	// fechado/bloqueado no momento), daí o amarelo em vez de vermelho.
	StyleAgentOffline = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "214"})
)
