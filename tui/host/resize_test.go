package host

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jakofranko/haive-mind/hub"
)

// After a resize, a long Transcript line stays within the Transcript's half of
// the window, and the right-hand panes keep their place beside it.
func TestHost_ALongTranscriptLineStaysInItsColumnAfterAResize(t *testing.T) {
	h, _ := serverHub(t)
	m := joinHost(t, h)
	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeSystem, "hub", "long-"+strings.Repeat("x", 300), 1))
	m = receiveUntil(t, m, shows("long-"))

	const width = 140
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m = updated.(*AppModel)

	screen := visible(m.View())
	if w := lipgloss.Width(screen); w > width {
		t.Errorf("the screen is %d columns wide, wider than the %d-column window:\n%s", w, width, screen)
	}
	// The Participants title sits just inside the right-hand pane, which
	// starts at the half-window line; border and padding move it a column or
	// two either way.
	const transcriptWidth = width / 2
	const slack = 2
	found := false
	for _, line := range strings.Split(screen, "\n") {
		idx := strings.Index(line, "Participants")
		if idx < 0 {
			continue
		}
		found = true
		if col := lipgloss.Width(line[:idx]); col < transcriptWidth-slack || col > transcriptWidth+slack {
			t.Errorf("the Participants title starts at column %d, not beside a %d-column Transcript:\n%s", col, transcriptWidth, screen)
		}
	}
	if !found {
		t.Fatalf("the Participants pane is not shown:\n%s", screen)
	}
}
