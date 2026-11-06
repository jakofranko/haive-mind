package hub

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ctxRunner behaves like the real runner: it blocks until the run is
// cancelled, then returns the context error.
type ctxRunner struct {
	started chan struct{}
}

func (r *ctxRunner) Run(ctx context.Context, prompt string) (string, error) {
	close(r.started)
	<-ctx.Done()
	return "", ctx.Err()
}

// Stopping the hub while an agent run is in flight must return promptly and
// must not panic when the cancelled run reports its error back through Send.
func TestHub_StopWhileAgentRunInFlight_ReturnsAndDoesNotPanic(t *testing.T) {
	// The original failure was a random select choice, so repeat.
	for i := 0; i < 200; i++ {
		h := NewHub()
		r := &ctxRunner{started: make(chan struct{})}
		h.SetAgentRunner(r)
		if err := h.Start(); err != nil {
			t.Fatal(err)
		}
		if err := h.Send(context.Background(), NewMessage(MessageTypeDirective, "Host", "hi", 1)); err != nil {
			t.Fatal(err)
		}
		select {
		case <-r.started:
		case <-time.After(time.Second):
			t.Fatal("agent run never started")
		}

		stopped := make(chan struct{})
		go func() { h.Stop(); close(stopped) }()
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Fatal("Stop did not return while an agent run was in flight")
		}
	}
}

// Anything that sends after Stop (e.g. a subscriber handler) gets an error,
// not a panic from the closed outbound channel.
func TestHub_SendAfterStop_ReturnsErrHubStopped(t *testing.T) {
	for i := 0; i < 200; i++ {
		h := NewHub()
		if err := h.Start(); err != nil {
			t.Fatal(err)
		}
		h.Stop()
		err := h.Send(context.Background(), NewMessage(MessageTypeSystem, "x", "late", 1))
		if !errors.Is(err, ErrHubStopped) {
			t.Fatalf("Send after Stop = %v, want ErrHubStopped", err)
		}
	}
}
