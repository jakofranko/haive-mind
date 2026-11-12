package participant

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
)

func enterName(t *testing.T, m *ParticipantModel, name string) *ParticipantModel {
	t.Helper()
	m.namePrompt.SetValue(name)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(*ParticipantModel)
}

// G13: a participant is one entry in the hub, keyed by the stable id the SSH
// server registered it under; choosing a display name changes what is shown,
// not who they are. Otherwise a second entry is created that disconnect
// never removes.
func TestParticipant_ChoosingDisplayName_DoesNotCreateSecondHubEntry(t *testing.T) {
	h := hub.NewHub()
	// What the SSH server does on connect.
	h.RegisterParticipant(hub.NewParticipantEvent("Agent-alice", "Agent-alice", hub.StatusIdle, hub.ParticipantEventConnect))

	m := NewParticipantModel(h, "Agent-alice", "alice")
	enterName(t, m, "Alice")

	ps := h.GetParticipants()
	if len(ps) != 1 {
		t.Fatalf("hub has %d entries for one person: %+v", len(ps), ps)
	}
	if ps[0].ID != "Agent-alice" || ps[0].DisplayName != "Alice" {
		t.Errorf("entry = {ID:%q DisplayName:%q}, want {Agent-alice, Alice}", ps[0].ID, ps[0].DisplayName)
	}

	// What the SSH server does when the session ends.
	h.UnregisterParticipant("Agent-alice")
	if left := h.GetParticipants(); len(left) != 0 {
		t.Errorf("after disconnect %d entries remain: %+v", len(left), left)
	}
}

// The participant's own sidebar row uses the same id, so hub updates about
// them replace it instead of adding a duplicate.
func TestParticipant_SidebarRowMatchesHubIdentity(t *testing.T) {
	h := hub.NewHub()
	m := NewParticipantModel(h, "Agent-alice", "alice")
	m = enterName(t, m, "Alice")

	row := m.sidebar.GetParticipants()
	if len(row) != 1 || row[0].ID != "Agent-alice" || row[0].DisplayName != "Alice" {
		t.Errorf("sidebar = %+v, want one row {Agent-alice, Alice}", row)
	}
}

// #29: when someone else disconnects, a participant's sidebar drops them.
func TestParticipant_OtherDisconnecting_RemovesThemFromSidebar(t *testing.T) {
	h := serverHub(t)
	m := join(t, h, "Agent-bob", "Bob")

	h.RegisterParticipant(hub.NewParticipantEvent("Agent-alice", "Alice", hub.StatusIdle, hub.ParticipantEventConnect))
	m = receiveUntil(t, m, shows("Alice"))
	h.UnregisterParticipant("Agent-alice")
	m = receiveUntil(t, m, func(s string) bool { return !strings.Contains(s, "Alice") })

	for _, p := range m.sidebar.GetParticipants() {
		if p.ID == "Agent-alice" {
			t.Errorf("Alice is still in the sidebar after disconnecting: %+v", p)
		}
	}
}
