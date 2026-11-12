package participant

import (
	"context"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
)

// With the Transcript pane focused, a participant who pages up sees older
// messages that have scrolled out of view.
func TestParticipant_PagingUpTheTranscriptShowsOlderMessages(t *testing.T) {
	h := serverHub(t)
	m := join(t, h, "Agent-alice", "Alice")
	for i := 0; i < 40; i++ {
		h.Send(context.Background(), hub.NewMessage(hub.MessageTypeSystem, "hub", fmt.Sprintf("marker-%02d", i), 1))
	}
	m = receiveUntil(t, m, shows("marker-39"))
	if shows("marker-00")(visible(m.View())) {
		t.Fatal("the first message is still in view; the test needs more messages to scroll")
	}

	m = step(m, tea.KeyMsg{Type: tea.KeyTab}) // response → transcript
	for i := 0; i < 60; i++ {
		m = step(m, tea.KeyMsg{Type: tea.KeyPgUp})
	}

	if screen := visible(m.View()); !shows("marker-00")(screen) {
		t.Errorf("after paging up, the first message is not shown:\n%s", screen)
	}
}
