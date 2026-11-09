package hub

import "testing"

// G14: the hub keeps each participant's unsent text, so a new session for the
// same person can pick it up where the last one left off.
func TestDrafts_AreKeptPerParticipantAndSurviveDisconnect(t *testing.T) {
	h := NewHub()
	h.RegisterParticipant(NewParticipantEvent("Agent-alice", "Alice", StatusIdle, ParticipantEventConnect))

	if got := h.GetDraft("Agent-alice"); got != "" {
		t.Errorf("a new participant has draft %q, want none", got)
	}

	h.SetDraft("Agent-alice", "use post")
	h.SetDraft("Agent-bob", "sqlite")
	h.SetDraft("Agent-alice", "use postgres\nfor prod") // later text replaces earlier

	h.UnregisterParticipant("Agent-alice") // the session ends

	if got := h.GetDraft("Agent-alice"); got != "use postgres\nfor prod" {
		t.Errorf("alice's draft after disconnect = %q", got)
	}
	if got := h.GetDraft("Agent-bob"); got != "sqlite" {
		t.Errorf("bob's draft = %q, want it untouched by alice's", got)
	}

	h.SetDraft("Agent-alice", "") // clearing
	if got := h.GetDraft("Agent-alice"); got != "" {
		t.Errorf("cleared draft = %q, want none", got)
	}
}
