package response

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The participant's layout() sizes the pane through SetSize. A window size
// that reaches Update is the whole window, not the pane's column, so the
// pane draws exactly as it did before the message.
func TestResponse_KeepsItsSizeWhenAWindowSizeReachesIt(t *testing.T) {
	m := New()
	m.SetSize(40, 10)
	before := m.View()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})

	if after := updated.(Model).View(); after != before {
		t.Errorf("a window size resized the pane; before:\n%s\nafter:\n%s", before, after)
	}
}
