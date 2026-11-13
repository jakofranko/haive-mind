package host

import (
	"context"
	"strings"
	"testing"

	"github.com/jakofranko/haive-mind/coordinator"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/jakofranko/haive-mind/tui/session"
	"github.com/sebdah/goldie/v2"
)

func pendingSnapshotModel(pending []hub.PendingCall) *AppModel {
	h := hub.NewHub()
	h.SetDisplayName(hub.HostID, "TestHost")
	m := NewAppModel(h)
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
	goldie.New(t, goldie.WithFixtureDir("testdata")).Assert(t, "host_pending_calls", []byte(m.View()))
}

func TestPendingCalls_NothingShownWhenNonePending(t *testing.T) {
	m := pendingSnapshotModel(nil)
	if screen := visible(m.View()); strings.Contains(screen, "Working") || strings.Contains(screen, "Waiting") || strings.Contains(screen, "Synthesizing") {
		t.Errorf("screen shows a pending call with none pending:\n%s", screen)
	}
}

func TestPendingCalls_ClearedByTheNextRoundEvent(t *testing.T) {
	m := pendingSnapshotModel([]hub.PendingCall{{ID: "a", Kind: hub.CallSynthesis, Round: 1}})
	if !strings.Contains(visible(m.View()), "Synthesizing the round 1 aggregate…") {
		t.Fatal("pending call not shown")
	}
	m.reduce(session.RoundEvent{Round: 2})
	if strings.Contains(visible(m.View()), "Synthesizing") {
		t.Error("pending call still shown after it ended")
	}
}

// gate holds every call until released, standing in for a slow agent.
type gate struct{ release chan struct{} }

func (g gate) Run(ctx context.Context, prompt string) (string, error) {
	select {
	case <-g.release:
	case <-ctx.Done():
	}
	return "done", nil
}

func TestPendingCalls_HostSeesSynthesisWhileItRunsAndNotAfter(t *testing.T) {
	g := gate{release: make(chan struct{})}
	h := hub.NewHub()
	h.SetAgentRunner(noopRunner{})
	h.SetSynthesizer(g)
	h.SetDisplayName(hub.HostID, "TestHost")
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })
	if err := coordinator.New(h).Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	drafts := recordSent(t, h, hub.MessageTypeAggregateDraft)
	m := joinHost(t, h)
	m.client.SendDirective("Question?")
	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeResponse, "Ann", "My answer", 1))
	m = receiveUntil(t, m, shows("My answer"))

	m.client.RequestAggregate()
	m = receiveUntil(t, m, shows("Synthesizing the round 1 aggregate…"))

	// A second press while it runs starts nothing new.
	m.client.RequestAggregate()

	close(g.release)
	m = receiveUntil(t, m, func(s string) bool { return !strings.Contains(s, "Synthesizing") })

	if got := drafts(); len(got) != 1 {
		t.Errorf("%d drafts arrived, want one", len(got))
	}
}
