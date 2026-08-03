package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConfirmModel_DefaultFocusIsCancel(t *testing.T) {
	c := newConfirmModel("t", "b", false, nil)
	if c.focused != 0 {
		t.Fatalf("foco inicial deveria ser Cancelar (0), obteve %d", c.focused)
	}
}

func TestConfirmModel_EnterOnCancel_ClosesWithoutOnConfirm(t *testing.T) {
	fired := false
	c := newConfirmModel("t", "b", false, func() tea.Msg { fired = true; return nil })

	next, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != nil {
		t.Fatal("modal deveria fechar (next == nil) ao confirmar com foco em Cancelar")
	}
	if cmd != nil {
		cmd()
	}
	if fired {
		t.Fatal("onConfirm não deveria disparar com foco em Cancelar")
	}
}

func TestConfirmModel_EnterOnConfirm_FiresOnConfirm(t *testing.T) {
	fired := false
	c := newConfirmModel("t", "b", false, func() tea.Msg { fired = true; return nil })

	next, _ := c.Update(tea.KeyMsg{Type: tea.KeyTab})
	if next == nil {
		t.Fatal("modal não deveria fechar só por alternar o foco")
	}
	c = next

	next, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next != nil {
		t.Fatal("modal deveria fechar (next == nil) ao confirmar")
	}
	if cmd == nil {
		t.Fatal("esperava o onConfirm Cmd de volta")
	}
	cmd()
	if !fired {
		t.Fatal("onConfirm deveria ter disparado")
	}
}

func TestConfirmModel_Esc_ClosesWithoutOnConfirm(t *testing.T) {
	fired := false
	c := newConfirmModel("t", "b", true, func() tea.Msg { fired = true; return nil })

	next, _ := c.Update(tea.KeyMsg{Type: tea.KeyTab})
	c = next
	next, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if next != nil {
		t.Fatal("modal deveria fechar com esc mesmo com foco em Confirmar")
	}
	if cmd != nil {
		cmd()
	}
	if fired {
		t.Fatal("onConfirm não deveria disparar com esc")
	}
}
