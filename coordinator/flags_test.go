package coordinator_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"jakofranko/haive-mind/coordinator"
	"jakofranko/haive-mind/hub"
)

// updates records every round_state the coordinator publishes.
type updates struct {
	mu   sync.Mutex
	list [][]string
	rnds []int
}

func watchUpdates(t *testing.T, h hub.Hub) *updates {
	t.Helper()
	u := &updates{}
	if _, err := h.Subscribe(context.Background(), hub.MessageTypeRoundState, func(m *hub.Message) error {
		round, ids, err := m.RoundState()
		if err != nil {
			t.Errorf("round_state content %q is not a JSON list: %v", m.Content, err)
		}
		u.mu.Lock()
		u.list = append(u.list, ids)
		u.rnds = append(u.rnds, round)
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

func (u *updates) lastRound() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.rnds) == 0 {
		return 0
	}
	return u.rnds[len(u.rnds)-1]
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
func TestFlag_HostFlagsAndUnflags(t *testing.T) {
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
func TestFlag_FlaggingTwiceIsIdempotent(t *testing.T) {
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
func TestFlag_ResubmitClearsOnlyThatFlag(t *testing.T) {
	h, c, _ := newRig(t)
	flag(h, "Agent-alice", true)
	flag(h, "Agent-bob", true)
	eventually(t, "both flagged", func() bool { return flaggedIs(c, "Agent-alice", "Agent-bob") })

	respond(h, "Agent-alice", "Alice", "revised answer", 1)

	eventually(t, "only bob still flagged", func() bool { return flaggedIs(c, "Agent-bob") })
}

func TestFlag_ResolveClearsAllFlags(t *testing.T) {
	h, c, _ := newRig(t)
	u := watchUpdates(t, h)
	flag(h, "Agent-alice", true)
	flag(h, "Agent-bob", true)
	eventually(t, "both flagged", func() bool { return flaggedIs(c, "Agent-alice", "Agent-bob") })

	h.Send(context.Background(), hub.NewResolve(1))

	eventually(t, "no flags", func() bool { return flaggedIs(c) })
	eventually(t, "empty update published", func() bool { return len(u.last()) == 0 && u.count() >= 3 })
}

func TestFlag_NewRoundClearsFlags(t *testing.T) {
	h, c, _ := newRig(t)
	flag(h, "Agent-alice", true)
	eventually(t, "flagged", func() bool { return flaggedIs(c, "Agent-alice") })

	h.StartRound(1)

	eventually(t, "no flags in the new round", func() bool { return flaggedIs(c) })
}

// A new round is announced as a Round state even when nobody was flagged.
func TestRoundState_PublishedOnNewRound(t *testing.T) {
	h, _, _ := newRig(t)
	u := watchUpdates(t, h)

	h.StartRound(1)

	eventually(t, "round state for round 2", func() bool { return u.count() == 1 && u.lastRound() == 2 && len(u.last()) == 0 })
}

// Flags set in round 1 are cleared by the new round, and the announcement says so.
func TestRoundState_NewRoundAnnouncesClearedFlags(t *testing.T) {
	h, _, _ := newRig(t)
	u := watchUpdates(t, h)
	flag(h, "Agent-alice", true)
	eventually(t, "flag announced", func() bool { return u.count() == 1 })

	h.StartRound(1)

	eventually(t, "round 2 with no flags", func() bool { return u.count() == 2 && u.lastRound() == 2 && len(u.last()) == 0 })
}

// A stale new-round request announces nothing.
func TestRoundState_StaleNewRoundPublishesNothing(t *testing.T) {
	h, c, _ := newRig(t)
	h.StartRound(1)
	eventually(t, "round 2", func() bool { return c.Round() == 2 })
	u := watchUpdates(t, h)

	h.StartRound(1)
	time.Sleep(300 * time.Millisecond) // negative check: waiting to confirm nothing else happens

	if n := u.count(); n != 0 {
		t.Errorf("%d round states for a stale request, want 0", n)
	}
}

// A flagged participant's resubmit lifts the flag and the round state says so.
func TestRoundState_PublishedOnResubmitWithCurrentRound(t *testing.T) {
	h, c, _ := newRig(t)
	u := watchUpdates(t, h)
	flag(h, "Agent-alice", true)
	eventually(t, "flagged", func() bool { return flaggedIs(c, "Agent-alice") })

	respond(h, "Agent-alice", "Alice", "revised", 1)

	eventually(t, "empty round state for round 1", func() bool { return u.count() == 2 && u.lastRound() == 1 && len(u.last()) == 0 })
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

// connect registers a participant the way the SSH server does.
func connect(t *testing.T, h hub.Hub, id string) {
	t.Helper()
	p := hub.NewParticipantEvent(id, id, hub.StatusIdle, 0, hub.ParticipantEventConnect)
	if err := h.RegisterParticipant(p); err != nil {
		t.Fatal(err)
	}
}

// Someone connecting mid-session is told the current round and who is flagged.
func TestRoundState_PublishedOnParticipantConnect(t *testing.T) {
	h, c, _ := newRig(t)
	h.StartRound(1)
	eventually(t, "round 2", func() bool { return c.Round() == 2 })
	flag(h, "Agent-alice", true)
	eventually(t, "flagged", func() bool { return flaggedIs(c, "Agent-alice") })
	u := watchUpdates(t, h)

	connect(t, h, "Agent-bob")

	eventually(t, "round state on connect", func() bool {
		return u.count() == 1 && u.lastRound() == 2 && len(u.last()) == 1 && u.last()[0] == "Agent-alice"
	})
}

// Announcing again is harmless: each connect repeats the same state.
func TestRoundState_RepeatedConnectRepeatsSameState(t *testing.T) {
	h, c, _ := newRig(t)
	flag(h, "Agent-alice", true)
	eventually(t, "flagged", func() bool { return flaggedIs(c, "Agent-alice") })
	u := watchUpdates(t, h)

	connect(t, h, "Agent-bob")
	connect(t, h, "Agent-bob")

	eventually(t, "two announcements", func() bool { return u.count() == 2 })
	if u.lastRound() != 1 || len(u.last()) != 1 || u.last()[0] != "Agent-alice" || !flaggedIs(c, "Agent-alice") || c.Round() != 1 {
		t.Errorf("state changed by a repeat: round %d, flagged %v", u.lastRound(), c.Flagged())
	}
}

// A connect announces state only; it is not a Round change, and it does not
// disturb anyone's status.
func TestRoundState_ConnectDoesNotResetStatuses(t *testing.T) {
	h, _, _ := newRig(t)
	var mu sync.Mutex
	changes := 0
	if _, err := h.Subscribe(context.Background(), hub.MessageTypeRoundChange, func(*hub.Message) error {
		mu.Lock()
		changes++
		mu.Unlock()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	u := watchUpdates(t, h)
	connect(t, h, "Agent-alice")
	if err := h.UpdateParticipantStatus("Agent-alice", hub.StatusSubmitted); err != nil {
		t.Fatal(err)
	}
	before := u.count()

	connect(t, h, "Agent-bob")

	eventually(t, "round state for bob", func() bool { return u.count() > before })
	time.Sleep(300 * time.Millisecond) // negative check: waiting to confirm nothing else happens
	for _, p := range h.GetParticipants() {
		if p.ID == "Agent-alice" && p.Status != hub.StatusSubmitted {
			t.Errorf("alice's status = %q after bob connected, want submitted", p.Status)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if changes != 0 {
		t.Errorf("%d round changes on connect, want 0", changes)
	}
}
