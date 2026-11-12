package coordinator_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jakofranko/haive-mind/coordinator"
	"github.com/jakofranko/haive-mind/hub"
)

// Adapter tests: the Coordinator on a real hub applies messages in the order
// the hub sent them and publishes what the core returns, in order. The state
// behaviour itself is tested through the core in state_test.go.

func flag(h hub.Hub, id string, on bool) {
	m := hub.NewFlag(1, id)
	if !on {
		m = hub.NewUnflag(1, id)
	}
	h.Send(context.Background(), m)
}

// respond sends a participant response the way a participant does: From is the
// display name, FromID the stable id.
func respond(h hub.Hub, id, display, text string, round int) {
	h.Send(context.Background(), response(id, display, text, round))
}

func flaggedIs(c *coordinator.Coordinator, want ...string) bool {
	return strings.Join(c.Flagged(), ",") == strings.Join(want, ",")
}

// collect records every message of typ published on the hub.
type collected struct {
	mu   sync.Mutex
	msgs []*hub.Message
}

func collect(t *testing.T, h hub.Hub, typ hub.MessageType) *collected {
	t.Helper()
	c := &collected{}
	if _, err := h.Subscribe(context.Background(), typ, func(m *hub.Message) error {
		c.mu.Lock()
		c.msgs = append(c.msgs, m)
		c.mu.Unlock()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return c
}

func (c *collected) all() []*hub.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*hub.Message(nil), c.msgs...)
}

// A host flag sent after a participant's response stays raised: the response
// is applied first, so it is not a resubmission that lifts the flag. Many
// participants give a flag many chances to overtake its response.
func TestAdapter_FlagSentAfterResponseStaysRaised(t *testing.T) {
	h, c, _ := newRig(t)

	const n = 200
	var want []string
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("Agent-p%03d", i)
		want = append(want, id)
		respond(h, id, id, "answer", 1)
		flag(h, id, true)
	}

	eventually(t, "every participant flagged", func() bool { return len(c.Flagged()) == n })
	time.Sleep(100 * time.Millisecond) // negative check: no flag is lifted afterwards
	if !flaggedIs(c, want...) {
		t.Errorf("flagged = %v, want all %d participants", c.Flagged(), n)
	}
}

// What the Coordinator publishes goes out in the order Apply returned it: each
// new round's Round state, then its Round change, round after round.
func TestAdapter_SendsGoOutInOrder(t *testing.T) {
	h, _, _ := newRig(t)
	var mu sync.Mutex
	var got []string
	if _, err := h.SubscribeTypes(context.Background(), []hub.MessageType{hub.MessageTypeRoundState, hub.MessageTypeRoundChange}, func(m *hub.Message) error {
		mu.Lock()
		got = append(got, fmt.Sprintf("%s %d", m.Type, m.Round))
		mu.Unlock()
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	const n = 30
	var want []string
	for r := 1; r <= n; r++ {
		h.StartRound(r)
		want = append(want, fmt.Sprintf("round_state %d", r+1), fmt.Sprintf("round_change %d", r+1))
	}
	eventually(t, "every round announced", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == len(want)
	})

	mu.Lock()
	defer mu.Unlock()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("published\n%v\nwant\n%v", got, want)
	}
}

// A Transcript snapshot's last submission and transcript always agree: if the
// snapshot carries the addressee's response, so does its transcript, and the
// other way round. Each participant connects just after responding, so many
// snapshots are built while responses are being applied.
func TestAdapter_SnapshotLastSubmissionAgreesWithItsTranscript(t *testing.T) {
	h, _, _ := newRig(t)
	snaps := collect(t, h, hub.MessageTypeTranscriptSnapshot)

	const n = 200
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("Agent-p%03d", i)
		respond(h, id, id, "answer", 1)
		if err := h.RegisterParticipant(hub.NewParticipantEvent(id, id, hub.StatusIdle, hub.ParticipantEventConnect)); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, "a snapshot per connect", func() bool { return len(snaps.all()) == n })

	for _, m := range snaps.all() {
		transcript, last, err := m.TranscriptSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		inTranscript := false
		for _, r := range transcript {
			if r.Type == hub.MessageTypeResponse && r.FromID == m.To {
				inTranscript = true
			}
		}
		if inTranscript != (last != nil) {
			t.Errorf("snapshot for %s: response in transcript %v, last submission %+v", m.To, inTranscript, last)
		}
	}
}
