package transcript

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jakofranko/haive-mind/hub"
)

// The app's layout() sizes the pane through SetSize. A window size that
// reaches Update is the whole window, not the pane's column, so the pane
// draws exactly as it did before the message.
func TestTranscript_KeepsItsSizeWhenAWindowSizeReachesIt(t *testing.T) {
	m := New("Welcome")
	m.SetSize(40, 12)
	m.AddMessage(hub.Message{From: "p", Content: strings.Repeat("word ", 60), Timestamp: time.Now(), Round: 1})
	before := m.View()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})

	if after := updated.(Model).View(); after != before {
		t.Errorf("a window size resized the pane; before:\n%s\nafter:\n%s", before, after)
	}
}
