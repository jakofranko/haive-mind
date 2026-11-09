package hub

import (
	"reflect"
	"testing"
)

func names(h *hubImpl) []string {
	var out []string
	for _, p := range h.GetParticipants() {
		out = append(out, p.ID)
	}
	return out
}

func join(h *hubImpl, id string) {
	h.RegisterParticipant(NewParticipantEvent(id, id, StatusIdle, ParticipantEventConnect))
}

// #31: participants come back in the order they joined, every time.
func TestGetParticipants_IsInJoinOrder(t *testing.T) {
	h := NewHub()
	for _, n := range []string{"host", "Agent-alice", "Agent-bob", "Agent-carol", "Agent-dave"} {
		join(h, n)
	}
	want := []string{"host", "Agent-alice", "Agent-bob", "Agent-carol", "Agent-dave"}

	for i := 0; i < 50; i++ { // a map would shuffle within a few tries
		if got := names(h); !reflect.DeepEqual(got, want) {
			t.Fatalf("call %d: order = %v, want %v", i, got, want)
		}
	}
}

// Changing someone's status or name keeps their place; leaving and coming back
// puts them at the end.
func TestGetParticipants_StatusChangeKeepsPlaceAndRejoinGoesLast(t *testing.T) {
	h := NewHub()
	for _, n := range []string{"host", "Agent-alice", "Agent-bob"} {
		join(h, n)
	}

	h.UpdateParticipantStatus("Agent-alice", StatusTyping)
	join(h, "Agent-alice") // re-registered (e.g. a display-name change)
	if got, want := names(h), []string{"host", "Agent-alice", "Agent-bob"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after updates: %v, want %v", got, want)
	}

	h.UnregisterParticipant("Agent-alice")
	join(h, "Agent-alice")
	if got, want := names(h), []string{"host", "Agent-bob", "Agent-alice"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after rejoining: %v, want %v", got, want)
	}
}
