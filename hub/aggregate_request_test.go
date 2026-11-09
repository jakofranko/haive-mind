package hub

import (
	"context"
	"testing"
	"time"
)

// An Aggregate is content: even one whose text is exactly "aggregate_request"
// goes to the agent like any other.
func TestAggregate_WithSignalLookalikeText_IsInvokedOnAgent(t *testing.T) {
	h := NewHub()
	agent := &stubRunner{reply: "ok"}
	h.SetAgentRunner(agent)
	replies := collect(t, h, MessageTypeResponse)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	h.Send(context.Background(), NewMessage(MessageTypeAggregate, "Host", "aggregate_request", 1))

	next(t, replies)
	if agent.callCount() != 1 || agent.calls[0] != "aggregate_request" {
		t.Errorf("agent calls = %q, want one call with the aggregate text", agent.calls)
	}
}

// The Aggregate request is a signal for the Coordinator and never reaches the
// agent.
func TestRequestAggregate_IsNotInvokedOnAgent(t *testing.T) {
	h := NewHub()
	agent := &stubRunner{reply: "should not run"}
	h.SetAgentRunner(agent)
	signals := collect(t, h, MessageTypeAggregateRequest)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	if err := h.RequestAggregate(2); err != nil {
		t.Fatal(err)
	}

	if m := next(t, signals); m.Round != 2 {
		t.Errorf("signal round = %d, want 2", m.Round)
	}
	time.Sleep(100 * time.Millisecond)
	if n := agent.callCount(); n != 0 {
		t.Errorf("agent invoked %d times by an aggregate request, want 0", n)
	}
}
