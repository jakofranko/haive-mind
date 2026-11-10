package host

import (
	"testing"

	"jakofranko/haive-mind/hub"
)

// #29: when a participant disconnects the host's list must drop them. The hub
// has already forgotten them by the time the event is handled, so the event
// itself has to carry enough to remove the row.
func TestHost_ParticipantDisconnect_RemovesThemFromTheList(t *testing.T) {
	h, _ := serverHub(t)
	m := joinHost(t, h)
	before := len(m.participants.GetParticipants())

	m = addParticipant(t, m, h, "Agent-alice", "Alice", hub.StatusIdle)
	if got := len(m.participants.GetParticipants()); got != before+1 {
		t.Fatalf("after connect the list has %d rows, want %d", got, before+1)
	}

	h.UnregisterParticipant("Agent-alice")
	for len(m.participants.GetParticipants()) != before {
		m = receive(t, m)
	}
}
