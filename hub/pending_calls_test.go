package hub

import (
	"context"
	"errors"
	"testing"
	"time"
)

// gateRunner blocks each call until released, so a test can look at the hub
// while calls are running.
type gateRunner struct {
	release chan struct{}
	reply   string
}

func (r *gateRunner) Run(ctx context.Context, prompt string) (string, error) {
	select {
	case <-r.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return r.reply, nil
}

func startedHub(t *testing.T) *hubImpl {
	t.Helper()
	h := NewHub()
	t.Cleanup(func() { h.Stop() })
	return h
}

func TestCallStarted_AnnouncesEachKindOfCallBeforeItsOutcome(t *testing.T) {
	for _, tc := range []struct {
		name string
		send *Message
		kind CallKind
	}{
		{"directive", NewMessage(MessageTypeDirective, "Host", "go", 2), CallDirective},
		{"aggregate", NewMessage(MessageTypeAggregate, "Host", "go", 2), CallAggregate},
		{"synthesis", NewMessage(MessageTypeSynthesisRequest, "hub", "go", 2), CallSynthesis},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := startedHub(t)
			gate := &gateRunner{release: make(chan struct{}), reply: "ok"}
			h.SetAgentRunner(gate)
			h.SetSynthesizer(gate)
			got := collect(t, h, MessageTypeCallStarted, MessageTypeAgentReply, MessageTypeAggregateDraft)
			h.Start()

			h.Send(context.Background(), tc.send)
			started := next(t, got)
			if started.Type != MessageTypeCallStarted {
				t.Fatalf("first message = %s, want call_started", started.Type)
			}
			if c := started.PendingCall(); c.Kind != tc.kind || c.Round != 2 || c.ID == "" {
				t.Errorf("pending call = %+v, want kind %s round 2 with an id", c, tc.kind)
			}
			close(gate.release)
			outcome := next(t, got)
			if outcome.CallID != started.CallID {
				t.Errorf("outcome ends call %q, want %q", outcome.CallID, started.CallID)
			}
		})
	}
}

func TestCallStarted_FailureEndsTheCallWithItsAgentError(t *testing.T) {
	h := startedHub(t)
	h.SetAgentRunner(&stubRunner{err: errors.New("boom")})
	got := collect(t, h, MessageTypeCallStarted, MessageTypeAgentError)
	h.Start()

	h.Send(context.Background(), NewMessage(MessageTypeDirective, "Host", "go", 1))
	started := next(t, got)
	failed := next(t, got)
	if failed.Type != MessageTypeAgentError || failed.CallID != started.CallID {
		t.Errorf("got %s ending %q, want an agent_error ending %q", failed.Type, failed.CallID, started.CallID)
	}
}

func TestCallStarted_SkippedCallNeverStarts(t *testing.T) {
	h := startedHub(t) // no runners set
	got := collect(t, h, MessageTypeCallStarted)
	h.Start()

	h.Send(context.Background(), NewMessage(MessageTypeDirective, "Host", "go", 1))
	h.Send(context.Background(), NewMessage(MessageTypeSynthesisRequest, "hub", "go", 1))
	select {
	case m := <-got:
		t.Errorf("unexpected %s for a call with no runner", m.Type)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestCallStarted_OverlappingCallsHaveDistinctIDs(t *testing.T) {
	h := startedHub(t)
	gate := &gateRunner{release: make(chan struct{}), reply: "ok"}
	h.SetAgentRunner(gate)
	got := collect(t, h, MessageTypeCallStarted)
	h.Start()

	h.Send(context.Background(), NewMessage(MessageTypeDirective, "Host", "go", 1))
	h.Send(context.Background(), NewMessage(MessageTypeDirective, "Host", "go", 1))
	a, b := next(t, got), next(t, got)
	close(gate.release)
	if a.CallID == b.CallID {
		t.Errorf("two calls share id %q", a.CallID)
	}
}

func TestRoundState_PendingCallsRoundTrip(t *testing.T) {
	pending := []PendingCall{{ID: "c1", Kind: CallSynthesis, Round: 1}, {ID: "c2", Kind: CallDirective, Round: 2}}
	_, _, got, err := NewRoundState(2, nil, pending).RoundState()
	if err != nil || len(got) != 2 || got[0] != pending[0] || got[1] != pending[1] {
		t.Errorf("got %+v, %v; want %+v", got, err, pending)
	}
}

func TestPendingCall_Labels(t *testing.T) {
	for call, want := range map[PendingCall]string{
		{Kind: CallSynthesis, Round: 1}: "Synthesizing the round 1 aggregate…",
		{Kind: CallDirective, Round: 2}: "Waiting for the agent (round 2)…",
		{Kind: CallAggregate, Round: 3}: "Waiting for the agent (round 3)…",
	} {
		if got := call.Label(); got != want {
			t.Errorf("%+v label = %q, want %q", call, got, want)
		}
	}
}
