package coordinator_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"jakofranko/haive-mind/coordinator"
	"jakofranko/haive-mind/hub"
)

// updates records every conflict_update the coordinator publishes.
type updates struct {
	mu   sync.Mutex
	list [][]string
}

func watchUpdates(t *testing.T, h hub.Hub) *updates {
	t.Helper()
	u := &updates{}
	if _, err := h.Subscribe(context.Background(), hub.MessageTypeConflictUpdate, func(m *hub.Message) error {
		var ids []string
		if err := json.Unmarshal([]byte(m.Content), &ids); err != nil {
			t.Errorf("conflict_update content %q is not a JSON list: %v", m.Content, err)
		}
		u.mu.Lock()
		u.list = append(u.list, ids)
		u.mu.Unlock()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return u
}

func (u *updates) last() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.list) == 0 {
		return nil
	}
	return u.list[len(u.list)-1]
}

func (u *updates) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.list)
}

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
	m := hub.NewMessage(hub.MessageTypeResponse, display, text, round)
	m.FromID = id
	h.Send(context.Background(), m)
}

func flaggedIs(c *coordinator.Coordinator, want ...string) bool {
	got := c.Flagged()
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// G12: the host flags a participant and everyone is told who is flagged.
func TestConflict_HostFlagsAndUnflags(t *testing.T) {
	h, c, _ := newRig(t)
	u := watchUpdates(t, h)

	flag(h, "Agent-alice", true)
	eventually(t, "alice flagged", func() bool { return flaggedIs(c, "Agent-alice") })
	eventually(t, "update published", func() bool { return strings.Join(u.last(), ",") == "Agent-alice" })

	flag(h, "Agent-alice", false)
	eventually(t, "alice unflagged", func() bool { return flaggedIs(c) })
	eventually(t, "empty update published", func() bool { return u.count() == 2 && len(u.last()) == 0 })
}

// Flagging someone twice announces once.
func TestConflict_FlaggingTwiceIsIdempotent(t *testing.T) {
	h, c, _ := newRig(t)
	u := watchUpdates(t, h)

	flag(h, "Agent-alice", true)
	flag(h, "Agent-alice", true)
	eventually(t, "flagged", func() bool { return flaggedIs(c, "Agent-alice") })
	time.Sleep(150 * time.Millisecond)
	if n := u.count(); n != 1 {
		t.Errorf("%d updates, want 1", n)
	}
}

// A flagged participant who resubmits is no longer flagged.
func TestConflict_ResubmitClearsOnlyThatFlag(t *testing.T) {
	h, c, _ := newRig(t)
	flag(h, "Agent-alice", true)
	flag(h, "Agent-bob", true)
	eventually(t, "both flagged", func() bool { return flaggedIs(c, "Agent-alice", "Agent-bob") })

	respond(h, "Agent-alice", "Alice", "revised answer", 1)

	eventually(t, "only bob still flagged", func() bool { return flaggedIs(c, "Agent-bob") })
}

func TestConflict_ResolveClearsAllFlags(t *testing.T) {
	h, c, _ := newRig(t)
	u := watchUpdates(t, h)
	flag(h, "Agent-alice", true)
	flag(h, "Agent-bob", true)
	eventually(t, "both flagged", func() bool { return flaggedIs(c, "Agent-alice", "Agent-bob") })

	h.Send(context.Background(), hub.NewResolve(1))

	eventually(t, "no flags", func() bool { return flaggedIs(c) })
	eventually(t, "empty update published", func() bool { return len(u.last()) == 0 && u.count() >= 3 })
}

func TestConflict_NewRoundClearsFlags(t *testing.T) {
	h, c, _ := newRig(t)
	flag(h, "Agent-alice", true)
	eventually(t, "flagged", func() bool { return flaggedIs(c, "Agent-alice") })

	h.StartRound(2)

	eventually(t, "no flags in the new round", func() bool { return flaggedIs(c) })
}

// Only each participant's latest submission in the round is synthesized.
func TestSynthesis_UsesLatestSubmissionPerParticipant(t *testing.T) {
	h, c, _, synth := newRigWithSynth(t)
	ctx := context.Background()

	h.Send(ctx, hub.NewMessage(hub.MessageTypeDirective, "Host", "Which database?", 1))
	respond(h, "Agent-alice", "Alice", "use postgres", 1)
	respond(h, "Agent-bob", "Bob", "use sqlite", 1)
	respond(h, "Agent-alice", "Alice", "actually use mysql", 1)
	eventually(t, "all responses recorded", func() bool {
		return countType(c.Transcript(), hub.MessageTypeResponse, "Alice") == 2
	})

	h.RequestAggregate(1)
	eventually(t, "synthesis requested", func() bool { return synth.calls.Load() == 1 })

	synth.mu.Lock()
	prompt := synth.prompts[0]
	synth.mu.Unlock()
	if strings.Contains(prompt, "use postgres") {
		t.Errorf("superseded answer is in the prompt: %s", prompt)
	}
	for _, want := range []string{"actually use mysql", "use sqlite"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q: %s", want, prompt)
		}
	}
	// Both of Alice's submissions remain in the transcript as history.
	if got := countType(c.Transcript(), hub.MessageTypeResponse, "Alice"); got != 2 {
		t.Errorf("transcript has %d of Alice's responses, want 2", got)
	}
}

// The host decides what counts as a conflict; the model is not asked to.
func TestSynthesis_PromptDoesNotAskTheModelToFindDisagreements(t *testing.T) {
	h, c, _, synth := newRigWithSynth(t)
	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeDirective, "Host", "q", 1))
	respond(h, "Agent-alice", "Alice", "a", 1)
	eventually(t, "recorded", func() bool { return countType(c.Transcript(), hub.MessageTypeResponse, "Alice") == 1 })

	h.RequestAggregate(1)
	eventually(t, "synthesis requested", func() bool { return synth.calls.Load() == 1 })

	synth.mu.Lock()
	prompt := strings.ToLower(synth.prompts[0])
	synth.mu.Unlock()
	if strings.Contains(prompt, "disagree") {
		t.Errorf("prompt still asks about disagreements: %s", prompt)
	}
}
