package hub

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// G16: when the agent fails, the hub publishes an agent_error carrying the
// reason and what to resend to try again.
func TestAgentFailure_PublishesErrorWithRetry(t *testing.T) {
	h := NewHub()
	h.SetAgentRunner(&stubRunner{err: errors.New("boom")})
	errs := collect(t, h, MessageTypeAgentError)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	h.Send(context.Background(), NewMessage(MessageTypeDirective, "Host", "do the thing", 2))

	m := next(t, errs)
	if !strings.Contains(m.Content, "boom") {
		t.Errorf("error content %q does not say why", m.Content)
	}
	if m.Retry == nil || m.Retry.Type != MessageTypeDirective || m.Retry.Content != "do the thing" {
		t.Errorf("retry = %+v, want the original directive", m.Retry)
	}
	if m.Round != 2 {
		t.Errorf("round = %d, want 2", m.Round)
	}
}

func TestSynthesisFailure_PublishesErrorWithRetry(t *testing.T) {
	h := NewHub()
	h.SetSynthesizer(&stubRunner{err: errors.New("no model")})
	errs := collect(t, h, MessageTypeAgentError)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	h.Send(context.Background(), NewMessage(MessageTypeSynthesisRequest, "hub", "combine these", 1))

	m := next(t, errs)
	if m.Retry == nil || m.Retry.Type != MessageTypeSynthesisRequest || m.Retry.Content != "combine these" {
		t.Errorf("retry = %+v, want the original synthesis request", m.Retry)
	}
}

func TestAgentSuccess_PublishesNoError(t *testing.T) {
	h := NewHub()
	h.SetAgentRunner(&stubRunner{reply: "fine"})
	errs := collect(t, h, MessageTypeAgentError)
	replies := collect(t, h, MessageTypeResponse)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	h.Send(context.Background(), NewMessage(MessageTypeDirective, "Host", "x", 1))
	next(t, replies)
	select {
	case m := <-errs:
		t.Errorf("unexpected agent_error: %+v", m)
	default:
	}
}
