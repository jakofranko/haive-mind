package participant

import (
	"context"
	"strings"
	"testing"

	"github.com/jakofranko/haive-mind/coordinator"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/jakofranko/haive-mind/tui/session"
	"github.com/sebdah/goldie/v2"
)

func pendingSnapshotModel(pending []hub.PendingCall) *ParticipantModel {
	h := hub.NewHub()
	h.SetDisplayName("Agent-1", "Agent-1")
	m := NewParticipantModel(h, "Agent-1", "user1")
	m.width, m.height = 120, 40
	m.reduce(session.CatchUpEvent{
		Round:        2,
		Participants: []hub.Participant{{ID: "Agent-1", DisplayName: "Agent-1", Status: hub.StatusIdle}},
		Pending:      pending,
	})
	m.layout()
	return m
}

func TestSnapshot_PendingCalls_OneLineEach(t *testing.T) {
	m := pendingSnapshotModel([]hub.PendingCall{
		{ID: "a", Kind: hub.CallSynthesis, Round: 1},
		{ID: "b", Kind: hub.CallDirective, Round: 2},
	})
	goldie.New(t, goldie.WithFixtureDir("testdata")).Assert(t, "participant_pending_calls", []byte(m.View()))
}

func TestPendingCalls_NothingShownWhenNonePending(t *testing.T) {
	m := pendingSnapshotModel(nil)
	if screen := m.View(); strings.Contains(screen, "Waiting for the agent") || strings.Contains(screen, "Synthesizing") {
		t.Errorf("screen shows a pending call with none pending:\n%s", screen)
	}
}

func TestPendingCalls_ClearedByTheNextRoundEvent(t *testing.T) {
	m := pendingSnapshotModel([]hub.PendingCall{{ID: "a", Kind: hub.CallDirective, Round: 2}})
	if !strings.Contains(m.View(), "Waiting for the agent (round 2)…") {
		t.Fatal("pending call not shown")
	}
	m.reduce(session.RoundEvent{Round: 2})
	if strings.Contains(m.View(), "Waiting for the agent") {
		t.Error("pending call still shown after it ended")
	}
}

type gate struct{ release chan struct{} }

func (g gate) Run(ctx context.Context, prompt string) (string, error) {
	select {
	case <-g.release:
	case <-ctx.Done():
	}
	return "done", nil
}

// A participant who joins while the agent is working is shown it at once, and
// sees it end.
func TestPendingCalls_LateJoinerSeesTheCallAndItsEnd(t *testing.T) {
	g := gate{release: make(chan struct{})}
	h := hub.NewHub()
	h.SetAgentRunner(g)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })
	if err := coordinator.New(h).Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeDirective, "Host", "Question?", 1))
	// Let the call start and be recorded before the participant joins.
	probe := join(t, h, "Agent-probe", "Probe")
	probe = receiveUntil(t, probe, shows("Waiting for the agent (round 1)…"))
	_ = probe

	late := join(t, h, "Agent-ann", "Ann")
	if !strings.Contains(late.View(), "Waiting for the agent (round 1)…") {
		t.Errorf("a participant joining mid-wait is not shown the call:\n%s", late.View())
	}

	close(g.release)
	late = receiveUntil(t, late, func(s string) bool { return !strings.Contains(s, "Waiting for the agent") })
	_ = late
}
