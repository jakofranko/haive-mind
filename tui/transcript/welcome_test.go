package transcript

import (
	"strings"
	"testing"
	"time"

	"github.com/jakofranko/haive-mind/hub"
)

// Each TUI gives its own welcome; earlier messages from a snapshot go after
// it, not before.
func TestTranscript_ShowsTheCallersWelcomeAboveEarlierMessages(t *testing.T) {
	m := New("Welcome to the test TUI")
	m.SetSize(80, 20)
	m.AddMessage(hub.Message{From: "p", Content: "live-message", Timestamp: time.Now(), Round: 1})
	m.PrependMessages([]hub.Message{{From: "p", Content: "earlier-message", Timestamp: time.Now(), Round: 1}})

	view := stripANSI(m.View())
	welcome, earlier, live := strings.Index(view, "Welcome to the test TUI"), strings.Index(view, "earlier-message"), strings.Index(view, "live-message")
	if welcome < 0 || earlier < 0 || live < 0 {
		t.Fatalf("view lacks the welcome or a message:\n%s", view)
	}
	if !(welcome < earlier && earlier < live) {
		t.Errorf("order is welcome %d, earlier %d, live %d; want welcome, earlier, live:\n%s", welcome, earlier, live, view)
	}
}
