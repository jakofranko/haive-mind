package participant

import (
	"context"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/session"
)

// drainInbound pulls hub messages out of the model the way its Update loop
// does (the thin wait command, one event at a time) until nothing arrives for
// a quiet period, and returns the contents received.
func drainInbound(m *ParticipantModel) map[string]bool {
	got := map[string]bool{}
	wait := 10 * time.Second // generous until the first message; short once they are flowing
	for {
		res := make(chan tea.Msg, 1)
		go func() { res <- m.waitForEvent()() }()
		select {
		case msg := <-res:
			got[msg.(eventMsg).Event.(session.MessageEvent).Message.Content] = true
			wait = 500 * time.Millisecond
		case <-time.After(wait):
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
		go func() { res <- m.waitForEvent()() }()
		select {
		case msg := <-res:
			if got, want := msg.(eventMsg).Event.(session.MessageEvent).Message.Content, fmt.Sprintf("%03d", i); got != want {
				t.Fatalf("message %d arrived as %s, want %s (out of order)", i, got, want)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for message %d", i)
		}
	}
}

// G11: the synthesized aggregate draft is shared with everyone, so a
// participant receives it like any other transcript message.
func TestParticipantModel_ReceivesAggregateDraft(t *testing.T) {
	h := hub.NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	m := NewParticipantModel(h, "Alice", "alice")
	m.Init()

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", "draft-text", 1))

	if got := drainInbound(m); !got["draft-text"] {
		t.Errorf("participant did not receive the aggregate draft; got %v", got)
	}
}
