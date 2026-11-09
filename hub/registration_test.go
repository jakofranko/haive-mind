package hub

import "testing"

// The hub honours the event kind a caller gives instead of announcing every
// registration as a connect.
func TestRegisterParticipant_HonoursTheGivenEventKind(t *testing.T) {
	h := NewHub()
	events := collect(t, h, MessageTypeParticipantEvent)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	h.RegisterParticipant(NewParticipantEvent("Agent-alice", "Alice", StatusIdle, ParticipantEventStatusChange))

	if ev := next(t, events); ev.Content != ParticipantEventStatusChange {
		t.Errorf("event content = %q, want %q", ev.Content, ParticipantEventStatusChange)
	}
}

// Renaming a registered participant updates it and announces a status change.
func TestSetDisplayName_RenamesARegisteredParticipantAndAnnouncesIt(t *testing.T) {
	h := NewHub()
	events := collect(t, h, MessageTypeParticipantEvent)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	h.RegisterParticipant(NewParticipantEvent("Agent-alice", "Agent-alice", StatusTyping, ParticipantEventConnect))
	if ev := next(t, events); ev.Content != ParticipantEventConnect {
		t.Fatalf("first event = %q, want connect", ev.Content)
	}

	h.SetDisplayName("Agent-alice", "Alice")

	if ev := next(t, events); ev.ID != "Agent-alice" || ev.Content != ParticipantEventStatusChange {
		t.Errorf("second event = id %q content %q, want a status_change for Agent-alice", ev.ID, ev.Content)
	}
	ps := h.GetParticipants()
	if len(ps) != 1 || ps[0].DisplayName != "Alice" || ps[0].Status != StatusTyping {
		t.Errorf("participants = %+v, want one Alice, status kept", ps)
	}
	if got := h.GetDisplayName("Agent-alice"); got != "Alice" {
		t.Errorf("stored display name = %q, want Alice", got)
	}
}

// Naming someone who has not connected stores the name and announces nothing.
func TestSetDisplayName_UnregisteredParticipantIsOnlyStored(t *testing.T) {
	h := NewHub()
	events := collect(t, h, MessageTypeParticipantEvent)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	h.SetDisplayName("Agent-alice", "Alice")

	if got := h.GetDisplayName("Agent-alice"); got != "Alice" {
		t.Errorf("stored display name = %q, want Alice", got)
	}
	if len(h.GetParticipants()) != 0 {
		t.Errorf("a participant appeared without registering: %+v", h.GetParticipants())
	}
	select {
	case ev := <-events:
		t.Errorf("unexpected event %q", ev.Content)
	default:
	}
}
