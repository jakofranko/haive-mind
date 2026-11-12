package host

import (
	"context"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/jakofranko/haive-mind/tui/session"
)

// drainMessages pulls events out of the model the way its Update loop does
// (the thin wait command, one event at a time) until nothing arrives for a
// quiet period, and returns the transcript messages' contents.
func drainMessages(m *AppModel) map[string]bool {
	got := map[string]bool{}
	wait := 10 * time.Second // generous until the first message; short once they are flowing
	for {
		res := make(chan tea.Msg, 1)
		go func() { res <- m.waitForEvent()() }()
		select {
		case msg := <-res:
			if ev, ok := msg.(eventMsg).Event.(session.MessageEvent); ok {
				got[ev.Message.Content] = true
			}
			wait = 500 * time.Millisecond
		case <-time.After(wait):
			return got
		}
	}
}

// G6: a burst of hub messages larger than the model's inbound buffer must
// all be delivered, not silently dropped.
func TestAppModel_BurstOfMessages_NoneDropped(t *testing.T) {
	h, _ := serverHub(t)
	m := joinHost(t, h) // nothing drains while the burst is sent

	const n = 300
	ctx := context.Background()
	for i := 0; i < n; i++ {
		h.Send(ctx, hub.NewMessage(hub.MessageTypeSystem, "hub", fmt.Sprintf("msg-%03d", i), 1))
	}

	if got := drainMessages(m); len(got) != n {
		t.Errorf("model received %d of %d messages", len(got), n)
	}
}
