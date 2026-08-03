package main

import tea "github.com/charmbracelet/bubbletea"

// pushScreenMsg navega para uma nova tela, empilhando a atual.
type pushScreenMsg struct{ screen screen }

// popScreenMsg volta um nível na pilha de navegação.
type popScreenMsg struct{}

// gotoMenuMsg descarta toda a pilha de navegação e volta pro menu
// principal — usado ao concluir um fluxo (ex. depois de um import de
// backup bem-sucedido).
type gotoMenuMsg struct{}

// requestConfirmMsg abre o modal de confirmação reutilizável (confirm.go).
// onConfirm só é executado se o usuário confirmar — nunca antes.
type requestConfirmMsg struct {
	title, body string
	danger      bool
	onConfirm   tea.Cmd
}

// errMsg exibe um erro não-fatal em banner, sem abortar a tela ativa.
type errMsg struct{ err error }

// startAsyncMsg pede ao root para rodar run em background, atribuindo um
// reqID e controlando o contador de operações em voo — ver async.go e a
// regra de roteamento em model.go. run deve ser barata de construir; a
// chamada de fato lenta só acontece quando o tea.Cmd resultante é
// executado pelo runtime do Bubble Tea.
type startAsyncMsg struct{ run func() tea.Msg }

// asyncResultMsg é o envelope que carrega de volta pro root o resultado
// real (run(), acima) junto do reqID que o originou — usado para descartar
// silenciosamente respostas obsoletas quando o usuário navega antes da
// resposta anterior chegar.
type asyncResultMsg struct {
	reqID int
	msg   tea.Msg
}
