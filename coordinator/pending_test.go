package coordinator_test

import (
	"testing"

	"github.com/jakofranko/haive-mind/coordinator"
	"github.com/jakofranko/haive-mind/hub"
)

func callStarted(kind hub.CallKind, round int) *hub.Message { return hub.NewCallStarted(kind, round) }

func outcome(t hub.MessageType, started *hub.Message) *hub.Message {
	m := hub.NewMessage(t, "agent", "out", started.Round)
	m.CallID = started.CallID
	return m
}

func pendingOf(t *testing.T, m *hub.Message) []hub.PendingCall {
	t.Helper()
	_, _, pending, err := m.RoundState()
	if err != nil {
		t.Fatal(err)
	}
	return pending
}

func TestState_CallStartedIsPendingUntilItsOutcome(t *testing.T) {
	for _, tc := range []struct {
		kind hub.CallKind
		end  hub.MessageType
	}{
		{hub.CallDirective, hub.MessageTypeAgentReply},
		{hub.CallAggregate, hub.MessageTypeAgentReply},
		{hub.CallSynthesis, hub.MessageTypeAggregateDraft},
		{hub.CallDirective, hub.MessageTypeAgentError},
	} {
		s := coordinator.NewState()
		started := callStarted(tc.kind, 1)
		out := s.Apply(started)
		if len(out) != 1 || len(pendingOf(t, out[0])) != 1 {
			t.Fatalf("%s: starting published %d messages, want one round state with one pending call", tc.kind, len(out))
		}
		out = s.Apply(outcome(tc.end, started))
		if len(out) != 1 || len(pendingOf(t, out[0])) != 0 {
			t.Errorf("%s: %s did not clear the pending call", tc.kind, tc.end)
		}
	}
}

func TestState_OverlappingCallsEndOneAtATime(t *testing.T) {
	s := coordinator.NewState()
	first, second := callStarted(hub.CallDirective, 1), callStarted(hub.CallDirective, 1)
	applyAll(s, first, second)

	out := s.Apply(outcome(hub.MessageTypeAgentReply, second))
	left := pendingOf(t, out[0])
	if len(left) != 1 || left[0].ID != first.CallID {
		t.Errorf("pending after the second ended = %+v, want only the first", left)
	}
}

func TestState_OutcomeOfNoPendingCallAnnouncesNothing(t *testing.T) {
	s := coordinator.NewState()
	wantNothing(t, s.Apply(hub.NewMessage(hub.MessageTypeAgentReply, "agent", "x", 1)))
}

func TestState_PendingCallsSurviveNewRoundAndJoinEveryRoundState(t *testing.T) {
	s := coordinator.NewState()
	s.Apply(callStarted(hub.CallDirective, 1))

	out := s.Apply(newRound(1))
	if got := pendingOf(t, ofType(out, hub.MessageTypeRoundState)[0]); len(got) != 1 {
		t.Errorf("round state after New Round has %d pending calls, want 1", len(got))
	}
	out = s.Apply(participantEvent("Agent-bob", hub.ParticipantEventConnect))
	if got := pendingOf(t, ofType(out, hub.MessageTypeRoundState)[0]); len(got) != 1 {
		t.Errorf("round state on connect has %d pending calls, want 1", len(got))
	}
}

func TestState_PendingCallsAreNotInTheTranscript(t *testing.T) {
	s := coordinator.NewState()
	started := callStarted(hub.CallDirective, 1)
	s.Apply(started)
	s.Apply(outcome(hub.MessageTypeAgentReply, started))

	for _, m := range s.Transcript() {
		if m.Type == hub.MessageTypeCallStarted || m.CallID != "" {
			t.Errorf("transcript holds %s with call id %q", m.Type, m.CallID)
		}
	}
	if len(s.Transcript()) != 1 {
		t.Errorf("transcript has %d entries, want just the reply", len(s.Transcript()))
	}
}

func TestState_RepeatAggregateRequestWhileSynthesisPendingIsDropped(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s, directive("q", 1), response("Agent-a", "A", "ans", 1))
	if len(ofType(s.Apply(aggregateRequest(1)), hub.MessageTypeSynthesisRequest)) != 1 {
		t.Fatal("first request should start a synthesis")
	}
	started := callStarted(hub.CallSynthesis, 1)
	s.Apply(started)

	wantNothing(t, s.Apply(aggregateRequest(1)))

	s.Apply(outcome(hub.MessageTypeAggregateDraft, started))
	if len(ofType(s.Apply(aggregateRequest(1)), hub.MessageTypeSynthesisRequest)) != 1 {
		t.Error("a request after the draft arrived should synthesize again")
	}
}

func TestState_AggregateRequestForAnotherRoundIsNotDropped(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s, directive("q", 1), response("Agent-a", "A", "ans", 1))
	s.Apply(callStarted(hub.CallSynthesis, 1))
	applyAll(s, newRound(1), directive("q2", 2), response("Agent-a", "A", "ans2", 2))

	if len(ofType(s.Apply(aggregateRequest(2)), hub.MessageTypeSynthesisRequest)) != 1 {
		t.Error("round 2's request was dropped because round 1's synthesis is pending")
	}
}

func TestState_AgentCallPendingDoesNotDropAggregateRequest(t *testing.T) {
	s := coordinator.NewState()
	applyAll(s, directive("q", 1), response("Agent-a", "A", "ans", 1), callStarted(hub.CallDirective, 1))

	if len(ofType(s.Apply(aggregateRequest(1)), hub.MessageTypeSynthesisRequest)) != 1 {
		t.Error("a pending agent call blocked the synthesis")
	}
}
