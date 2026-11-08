package host

import (
	"context"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
)

// drainInbound pulls hub messages out of the model the way its Update loop
// does (one listenForMessages command at a time) until nothing arrives for
// a quiet period, and returns the contents received.
func drainInbound(m *AppModel) map[string]bool {
	got := map[string]bool{}
	for {
		res := make(chan tea.Msg, 1)
		go func() { res <- m.listenForMessages()() }()
		select {
		case msg := <-res:
			hm := msg.(hubMessageMsg)
			got[hm.Content] = true
		case <-time.After(500 * time.Millisecond):
			return got
		}
	}
}

// G6: a burst of hub messages larger than the model's inbound buffer must
// all be delivered, not silently dropped.
func TestAppModel_BurstOfMessages_NoneDropped(t *testing.T) {
	h := hub.NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	m := NewAppModel(h)
	m.Init() // subscribe; nothing is draining yet

	const n = 300
	ctx := context.Background()
	for i := 0; i < n; i++ {
		h.Send(ctx, hub.NewMessage(hub.MessageTypeSystem, "hub", fmt.Sprintf("msg-%03d", i), 1))
	}

	got := drainInbound(m)
	if len(got) != n {
		t.Errorf("model received %d of %d messages", len(got), n)
	}
}
