package hub

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// G23: a subscriber sees messages in the order they were sent, so the
// transcript is linear.
func TestHub_DeliversToSubscribersInSendOrder(t *testing.T) {
	h := NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	ctx := context.Background()

	const n = 200
	var mu sync.Mutex
	var got []string
	done := make(chan struct{})
	h.Subscribe(ctx, MessageTypeSystem, func(m *Message) error {
		mu.Lock()
		got = append(got, m.Content)
		if len(got) == n {
			close(done)
		}
		mu.Unlock()
		return nil
	})

	for i := 0; i < n; i++ {
		h.Send(ctx, NewMessage(MessageTypeSystem, "x", fmt.Sprintf("%03d", i), 1))
	}
	<-done

	mu.Lock()
	defer mu.Unlock()
	for i, c := range got {
		if want := fmt.Sprintf("%03d", i); c != want {
			t.Fatalf("message %d delivered as %s, want %s (out of order)", i, c, want)
		}
	}
}
