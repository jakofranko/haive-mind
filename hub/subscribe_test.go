package hub

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// G5: after Unsubscribe, the handler is no longer called, while other
// subscribers keep receiving.
func TestHub_Unsubscribe_StopsDelivery(t *testing.T) {
	h := NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	ctx := context.Background()

	var gone, kept atomic.Int32
	sub, err := h.Subscribe(ctx, MessageTypeSystem, func(*Message) error { gone.Add(1); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Subscribe(ctx, MessageTypeSystem, func(*Message) error { kept.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}

	h.Send(ctx, NewMessage(MessageTypeSystem, "x", "one", 1))
	waitFor(t, func() bool { return gone.Load() == 1 && kept.Load() == 1 })

	sub.Unsubscribe()

	h.Send(ctx, NewMessage(MessageTypeSystem, "x", "two", 1))
	waitFor(t, func() bool { return kept.Load() == 2 })
	time.Sleep(50 * time.Millisecond)

	if got := gone.Load(); got != 1 {
		t.Errorf("unsubscribed handler called %d times, want 1", got)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}
