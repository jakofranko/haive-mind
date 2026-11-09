package participant

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/session"
)

// statusRig is a participant already named and registered the way the SSH
// server does it, focused on the response box.
func statusRig(t *testing.T) (*ParticipantModel, func() hub.ParticipantStatus) {
	t.Helper()
	h := hub.NewHub()
	h.SetDisplayName("Agent-alice", "Alice")
	h.RegisterParticipant(hub.NewParticipantEvent("Agent-alice", "Alice", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	m := NewParticipantModel(h, "Agent-alice", "alice")
	m.activePane = paneResponse
	m.updateActiveStates()
	return m, func() hub.ParticipantStatus {
		for _, p := range h.GetParticipants() {
			if p.ID == "Agent-alice" {
				return p.Status
			}
		}
		t.Fatal("participant missing from hub")
		return ""
	}
}

func step(m *ParticipantModel, msg tea.Msg) *ParticipantModel {
	updated, _ := m.Update(msg)
	return updated.(*ParticipantModel)
}

func typeRunes(m *ParticipantModel, s string) *ParticipantModel {
	return step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
}

// G20: the host's list can only be accurate if the hub is told what each
// participant is doing.
func TestParticipantStatus_FollowsTypingSubmittingAndNewRound(t *testing.T) {
	m, status := statusRig(t)

	if got := status(); got != hub.StatusIdle {
		t.Fatalf("starts as %q, want idle", got)
	}

	m = typeRunes(m, "use postgres")
	if got := status(); got != hub.StatusTyping {
		t.Errorf("after typing: %q, want typing", got)
	}

	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := status(); got != hub.StatusSubmitted {
		t.Errorf("after submitting: %q, want submitted", got)
	}

	m = step(m, eventMsg{session.RoundEvent{Round: 2}})
	if got := status(); got != hub.StatusIdle {
		t.Errorf("after a new round: %q, want idle", got)
	}
}

// Deleting everything they typed puts them back to idle.
func TestParticipantStatus_EmptyingTheBox_ReturnsToIdle(t *testing.T) {
	m, status := statusRig(t)

	m = typeRunes(m, "a")
	if got := status(); got != hub.StatusTyping {
		t.Fatalf("after typing: %q, want typing", got)
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if got := status(); got != hub.StatusIdle {
		t.Errorf("after emptying the box: %q, want idle", got)
	}
}
