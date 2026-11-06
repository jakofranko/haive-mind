package participant

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
func drainInbound(m *ParticipantModel) map[string]bool {
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
func TestParticipantModel_BurstOfMessages_NoneDropped(t *testing.T) {
	h := hub.NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	m := NewParticipantModel(h, "Alice", "alice")
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

// G23: messages of different types reach the model in the order they were
// sent, so the transcript the participant sees is linear.
func TestParticipantModel_MixedTypes_ArriveInSendOrder(t *testing.T) {
	h := hub.NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	m := NewParticipantModel(h, "Alice", "alice")
	m.Init()

	const n = 200
	ctx := context.Background()
	types := []hub.MessageType{hub.MessageTypeDirective, hub.MessageTypeResponse, hub.MessageTypeSystem, hub.MessageTypeAggregate}
	for i := 0; i < n; i++ {
		h.Send(ctx, hub.NewMessage(types[i%len(types)], "x", fmt.Sprintf("%03d", i), 1))
	}

	for i := 0; i < n; i++ {
		res := make(chan tea.Msg, 1)
		go func() { res <- m.listenForMessages()() }()
		select {
		case msg := <-res:
			if got, want := msg.(hubMessageMsg).Content, fmt.Sprintf("%03d", i); got != want {
				t.Fatalf("message %d arrived as %s, want %s (out of order)", i, got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for message %d", i)
		}
	}
}
