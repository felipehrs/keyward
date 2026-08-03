package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmModel é o modal de confirmação reutilizável para toda ação
// destrutiva da TUI (host replace/remove, sobrescrever chave, unregister,
// import com overwrite/apply, export com chave privada). Nunca importa
// core — só carrega o tea.Cmd já fechado (closure) que a tela de origem
// construiu; a mutação real só dispara se o usuário confirmar.
type confirmModel struct {
	title, body string
	danger      bool
	focused     int // 0 = Cancelar (default, seguro), 1 = Confirmar
	onConfirm   tea.Cmd

	width, height int
}

func newConfirmModel(title, body string, danger bool, onConfirm tea.Cmd) *confirmModel {
	return &confirmModel{title: title, body: body, danger: danger, onConfirm: onConfirm}
}

// Update processa a interação do modal. Retorna confirm == nil quando o
// modal deve fechar (confirmado ou cancelado) — o root interpreta isso
// como "sem modal ativo".
func (c *confirmModel) Update(msg tea.Msg) (*confirmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width, c.height = msg.Width, msg.Height
		return c, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "right", "tab", "h", "l":
			c.focused = 1 - c.focused
		case "y":
			return nil, c.onConfirm
		case "n":
			return nil, nil
		case "enter":
			if c.focused == 1 {
				return nil, c.onConfirm
			}
			return nil, nil
		case "esc":
			return nil, nil
		}
	}
	return c, nil
}

func (c *confirmModel) View() string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
	if c.danger {
		style = style.BorderForeground(lipgloss.AdaptiveColor{Light: "160", Dark: "203"})
	}

	cancel := "[ Cancelar ]"
	confirm := "[ Confirmar ]"
	if c.danger {
		confirm = StyleDanger.Render(confirm)
	}
	if c.focused == 0 {
		cancel = StyleSelected.Render(cancel)
	} else {
		confirm = StyleSelected.Render(confirm)
	}

	body := StyleTitle.Render(c.title) + "\n\n" + c.body + "\n\n" + cancel + "   " + confirm
	box := style.Render(body)

	if c.width > 0 && c.height > 0 {
		return lipgloss.Place(c.width, c.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
