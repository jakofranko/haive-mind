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

// drainMessages pulls events out of the model the way its Update loop does
// (the thin wait command, one event at a time) until nothing arrives for a
// quiet period, and returns the transcript messages' contents in order.
func drainMessages(m *ParticipantModel) []string {
	var got []string
	wait := 10 * time.Second // generous until the first message; short once they are flowing
	for {
		res := make(chan tea.Msg, 1)
		go func() { res <- m.waitForEvent()() }()
		select {
		case msg := <-res:
			if ev, ok := msg.(eventMsg).Event.(session.MessageEvent); ok {
				got = append(got, ev.Message.Content)
			}
			wait = 500 * time.Millisecond
		case <-time.After(wait):
			return got
		}
	}
}

// G6: a burst of hub messages larger than the model's inbound buffer must
// all be delivered, not silently dropped.
func TestParticipantModel_BurstOfMessages_NoneDropped(t *testing.T) {
	h := serverHub(t)
	m := join(t, h, "Agent-alice", "Alice") // nothing drains while the burst is sent

	const n = 300
	ctx := context.Background()
	for i := 0; i < n; i++ {
		h.Send(ctx, hub.NewMessage(hub.MessageTypeSystem, "hub", fmt.Sprintf("msg-%03d", i), 1))
	}

	if got := drainMessages(m); len(got) != n {
		t.Errorf("model received %d of %d messages", len(got), n)
	}
}

// G23: messages of different types reach the model in the order they were
// sent, so the transcript the participant sees is linear.
func TestParticipantModel_MixedTypes_ArriveInSendOrder(t *testing.T) {
	h := serverHub(t)
	m := join(t, h, "Agent-alice", "Alice")

	const n = 200
	ctx := context.Background()
	types := []hub.MessageType{hub.MessageTypeResponse, hub.MessageTypeSystem, hub.MessageTypeAggregateDraft}
	for i := 0; i < n; i++ {
		h.Send(ctx, hub.NewMessage(types[i%len(types)], "x", fmt.Sprintf("%03d", i), 1))
	}

	got := drainMessages(m)
	if len(got) != n {
		t.Fatalf("model received %d of %d messages", len(got), n)
	}
	for i, c := range got {
		if want := fmt.Sprintf("%03d", i); c != want {
			t.Fatalf("message %d arrived as %s, want %s (out of order)", i, c, want)
		}
	}
}

// G11: the synthesized aggregate draft is shared with everyone, so a
// participant receives it like any other transcript message.
func TestParticipantModel_ReceivesAggregateDraft(t *testing.T) {
	h := serverHub(t)
	m := join(t, h, "Agent-alice", "Alice")

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", "draft-text", 1))

	receiveUntil(t, m, shows("draft-text"))
}
