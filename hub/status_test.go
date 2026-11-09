package hub

import "testing"

// G20: a participant's status can change, the hub remembers it, and
// subscribers hear about it.
func TestUpdateParticipantStatus_IsStoredAndAnnounced(t *testing.T) {
	h := NewHub()
	h.RegisterParticipant(NewParticipantEvent("Agent-alice", "Alice", StatusIdle, ParticipantEventConnect))
	events := collect(t, h, MessageTypeParticipantEvent)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	if err := h.UpdateParticipantStatus("Agent-alice", StatusTyping); err != nil {
		t.Fatal(err)
	}

	ps := h.GetParticipants()
	if len(ps) != 1 || ps[0].Status != StatusTyping || ps[0].DisplayName != "Alice" {
		t.Errorf("participants = %+v, want Alice typing", ps)
	}
	if ev := next(t, events); ev.ID != "Agent-alice" || ev.Content != ParticipantEventStatusChange {
		t.Errorf("event = id %q content %q, want a status_change for Agent-alice", ev.ID, ev.Content)
	}
}

func TestUpdateParticipantStatus_UnknownParticipant_IsAnError(t *testing.T) {
	h := NewHub()
	if err := h.UpdateParticipantStatus("nobody", StatusTyping); err == nil {
		t.Error("expected an error for an unregistered participant")
	}
}
