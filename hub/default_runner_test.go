package hub

import (
	"context"
	"testing"
	"time"
)

// A hub nobody gave a runner to never launches a real agent: tests that build
// a bare hub and send directives must not spawn `opencode`.
func TestNewHub_DoesNotInvokeARealAgentByDefault(t *testing.T) {
	t.Setenv("PATH", "") // if the hub did try `opencode`, it would fail at once and say so
	h := NewHub()
	errs := collect(t, h, MessageTypeAgentError, MessageTypeSystem, MessageTypeResponse)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	h.Send(context.Background(), NewMessage(MessageTypeDirective, "Host", "hello", 1))
	h.Send(context.Background(), NewMessage(MessageTypeSynthesisRequest, "hub", "combine", 1))

	select {
	case m := <-errs:
		if m.Type != MessageTypeDirective { // the hub's own messages only
			t.Errorf("hub tried to run an agent nobody configured: %s %q", m.Type, m.Content)
		}
	case <-time.After(500 * time.Millisecond):
	}
}
