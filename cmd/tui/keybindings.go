package main

import "github.com/charmbracelet/bubbles/key"

// Bindings compartilhados por várias telas. Bindings específicos de uma
// única tela (ex. "g" para gerar chave) ficam declarados no arquivo
// daquela tela.
var (
	keyUp         = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "cima"))
	keyDown       = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "baixo"))
	keyEnter      = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "selecionar"))
	keyBack       = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "voltar"))
	keyQuit       = key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "sair"))
	keyReload     = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "recarregar"))
	keyGenerate   = key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "gerar chave"))
	keyNew        = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "novo"))
	keyDelete     = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remover"))
	keyEdit       = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "editar"))
	keyUnregister = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "unregister"))
	// keyRegisterOrphan usa "o" (não "r", já reservado globalmente para
	// recarregar) para registrar uma chave órfã encontrada em disco — a
	// spec/plano sugeria "r" aqui, mas isso colidiria com a convenção de
	// recarregar já adotada em toda tela de listagem.
	keyRegisterOrphan = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "registrar órfã"))
	keySettings       = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "configurações"))
)
