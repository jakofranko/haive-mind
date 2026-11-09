package participant

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
)

func namedModel(h hub.Hub) *ParticipantModel {
	h.SetDisplayName("alice", "Alice")
	m := NewParticipantModel(h, "Agent-alice", "alice")
	m.activePane = paneResponse
	m.updateActiveStates()
	return m
}

// G14: unsent text survives a disconnect. A new session for the same person
// opens with what they had typed.
func TestDraft_SurvivesReconnect(t *testing.T) {
	h := hub.NewHub()
	first := namedModel(h)
	first = typeRunes(first, "use postgres")

	// The session ends; a new one starts for the same SSH user.
	second := namedModel(h)

	if got := second.response.Value(); got != "use postgres" {
		t.Errorf("reconnected box holds %q, want the earlier draft", got)
	}
}

// Sending a response uses the draft up: the next session starts empty.
func TestDraft_IsClearedBySubmitting(t *testing.T) {
	h := hub.NewHub()
	m := namedModel(h)
	m = typeRunes(m, "use postgres")
	step(m, tea.KeyMsg{Type: tea.KeyEnter})

	if got := namedModel(h).response.Value(); got != "" {
		t.Errorf("after submitting, a new session starts with %q, want empty", got)
	}
}

// Emptying the box by hand clears the saved draft too.
func TestDraft_IsClearedWhenTheBoxIsEmptied(t *testing.T) {
	h := hub.NewHub()
	m := namedModel(h)
	m = typeRunes(m, "a")
	step(m, tea.KeyMsg{Type: tea.KeyBackspace})

	if got := namedModel(h).response.Value(); got != "" {
		t.Errorf("a new session starts with %q, want empty", got)
	}
}
