package main

import tea "github.com/charmbracelet/bubbletea"

// startAsync constrói o tea.Cmd que uma tela retorna para pedir ao root que
// execute fn em background, com spinner e descarte de respostas obsoletas
// geridos centralmente (ver model.go). fn só é invocada depois que o root
// atribuir um reqID a esta chamada — nunca diretamente pela tela.
func startAsync(fn func() tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return startAsyncMsg{run: fn}
	}
}
