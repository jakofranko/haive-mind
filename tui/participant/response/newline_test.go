package response

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// G20: the input must offer a newline key a terminal can actually deliver.
// Ctrl+Enter is not one (a terminal sends the same CR as Enter), so Ctrl+J
// and Alt+Enter are the supported keys.
func TestInput_NewlineKeys_InsertANewline(t *testing.T) {
	keys := map[string]tea.KeyMsg{
		"ctrl+j":    {Type: tea.KeyCtrlJ},
		"alt+enter": {Type: tea.KeyEnter, Alt: true},
	}
	for name, k := range keys {
		t.Run(name, func(t *testing.T) {
			m := New()
			m.SetSize(80, 12)
			var tm tea.Model = m
			tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab")})
			tm, _ = tm.Update(k)
			tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("cd")})
			got := tm.(Model)
			if v := got.Value(); v != "ab\ncd" {
				t.Errorf("value = %q, want two lines %q", v, "ab\ncd")
			}
		})
	}
}
