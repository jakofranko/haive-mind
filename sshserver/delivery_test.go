package sshserver

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"jakofranko/haive-mind/hub"
)

// G28: constructing the SSH server must not take messages away from hub
// subscribers. Every message sent through the hub reaches every subscriber.
func TestNewServer_DoesNotStealMessagesFromSubscribers(t *testing.T) {
	h := hub.NewHub()
	NewServer(h, "127.0.0.1:0", filepath.Join(t.TempDir(), "host_key")) // not "": that writes a key into the package directory
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

// The server listens where it is told to, so tests can keep it on the loopback
// interface and never ask the operating system's firewall for permission.
func TestNewServer_ListensOnTheGivenAddress(t *testing.T) {
	h := hub.NewHub()
	defer h.Stop()

	s := NewServer(h, "127.0.0.1:0", filepath.Join(t.TempDir(), "host_key"))

	if s.Addr != "127.0.0.1:0" {
		t.Fatalf("server address = %q, want 127.0.0.1:0", s.Addr)
	}
}
