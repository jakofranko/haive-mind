package host

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
)

// hostAfterSnapshot starts a real hub, sends the host its transcript
// snapshot of recorded, then any live messages, and feeds every resulting
// event through Update. It returns the host's visible screen.
func hostAfterSnapshot(t *testing.T, recorded []*hub.Message, live ...*hub.Message) string {
	t.Helper()
	h := hub.NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })

	h.SetDisplayName(hub.HostID, "TestHost")
	m := NewAppModel(h)
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 80})

	ctx := context.Background()
	h.Send(ctx, hub.NewTranscriptSnapshot(1, hub.HostID, recorded, nil))
	n := 1
	for _, l := range live {
		h.Send(ctx, l)
		n++
	}
	for i := 0; i < n; i++ {
		res := make(chan tea.Msg, 1)
		go func() { res <- m.waitForEvent()() }()
		select {
		case msg := <-res:
			model, _ = model.Update(msg)
		case <-time.After(10 * time.Second):
			t.Fatal("no event arrived")
		}
	}
	return visible(model.View())
}

func TestHost_SnapshotFillsTheTranscript(t *testing.T) {
	screen := hostAfterSnapshot(t, []*hub.Message{
		hub.NewMessage(hub.MessageTypeDirective, "host", "earlier directive", 1),
		hub.NewMessage(hub.MessageTypeAggregate, "hub", "earlier aggregate", 1),
	})
	for _, want := range []string{"earlier directive", "earlier aggregate"} {
		if !strings.Contains(screen, want) {
			t.Errorf("transcript is missing %q:\n%s", want, screen)
		}
	}
}
