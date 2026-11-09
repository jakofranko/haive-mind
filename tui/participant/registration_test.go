package participant

import (
	"context"
	"sync"
	"testing"
	"time"

	"jakofranko/haive-mind/hub"
)

// A session registers once, on connect; choosing a name renames that entry
// and announces a status change. The TUI itself announces no connect.
func TestParticipant_ChoosingDisplayName_RenamesWithoutASecondConnect(t *testing.T) {
	h := hub.NewHub()
	var kinds []string
	var mu sync.Mutex
	if _, err := h.Subscribe(context.Background(), hub.MessageTypeParticipantEvent, func(msg *hub.Message) error {
		mu.Lock()
		kinds = append(kinds, msg.Content)
		mu.Unlock()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	// What the SSH server does on connect.
	h.RegisterParticipant(hub.NewParticipantEvent("Agent-alice", "Agent-alice", hub.StatusIdle, hub.ParticipantEventConnect))
	m := NewParticipantModel(h, "Agent-alice", "alice")
	enterName(t, m, "Alice")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(kinds)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // room for an unwanted third event

	mu.Lock()
	defer mu.Unlock()
	if len(kinds) != 2 || kinds[0] != hub.ParticipantEventConnect || kinds[1] != hub.ParticipantEventStatusChange {
		t.Errorf("events = %v, want [connect status_change]", kinds)
	}
}

// Without the server's registration the TUI does not register itself.
func TestParticipant_ChoosingDisplayName_DoesNotRegister(t *testing.T) {
	h := hub.NewHub()
	m := NewParticipantModel(h, "Agent-alice", "alice")
	enterName(t, m, "Alice")

	if ps := h.GetParticipants(); len(ps) != 0 {
		t.Errorf("TUI registered itself: %+v", ps)
	}
	if got := h.GetDisplayName("Agent-alice"); got != "Alice" {
		t.Errorf("stored display name = %q, want Alice", got)
	}
}
