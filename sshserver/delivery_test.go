package sshserver

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"jakofranko/haive-mind/hub"
)

// G28: constructing the SSH server must not take messages away from hub
// subscribers. Every message sent through the hub reaches every subscriber.
func TestNewServer_DoesNotStealMessagesFromSubscribers(t *testing.T) {
	h := hub.NewHub()
	NewServer(h, "0", "")
	defer h.Stop()

	var got atomic.Int32
	if _, err := h.Subscribe(context.Background(), hub.MessageTypeSystem, func(*hub.Message) error {
		got.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	const n = 300
	for i := 0; i < n; i++ {
		h.Send(context.Background(), hub.NewMessage(hub.MessageTypeSystem, "x", "m", 1))
	}

	deadline := time.Now().Add(3 * time.Second)
	for got.Load() < n && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if g := got.Load(); g != n {
		t.Errorf("subscriber received %d of %d messages", g, n)
	}
}
