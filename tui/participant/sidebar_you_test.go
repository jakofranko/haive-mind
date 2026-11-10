package participant

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"jakofranko/haive-mind/hub"
)

// youRows returns the sidebar rows drawn in the "you" style (bold), as the
// text a user reads.
func youRows(t *testing.T, m *ParticipantModel) []string {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	var rows []string
	for _, line := range strings.Split(m.sidebar.View(), "\n") {
		if strings.Contains(line, "\x1b[1;") && !strings.Contains(line, "Participants") {
			rows = append(rows, strings.TrimSpace(visible(line)))
		}
	}
	return rows
}

func sidebarModel(t *testing.T, id string, people map[string]string) *ParticipantModel {
	t.Helper()
	h, _ := serverHub(t)
	for pid, name := range people {
		if pid != id {
			h.SetDisplayName(pid, name)
			h.RegisterParticipant(hub.NewParticipantEvent(pid, name, hub.StatusIdle, hub.ParticipantEventConnect))
		}
	}
	return join(t, h, id, people[id])
}

// The sidebar knows who "you" are by participant id. Two people who chose the
// same display name must not both be drawn as you.
func TestParticipant_SidebarHighlightsOnlyOwnID_WhenDisplayNamesCollide(t *testing.T) {
	m := sidebarModel(t, "Agent-sam2", map[string]string{"Agent-sam1": "Sam", "Agent-sam2": "Sam"})

	if got := youRows(t, m); len(got) != 1 {
		t.Fatalf("%d rows highlighted as you, want exactly 1: %q", len(got), got)
	}
}

func TestParticipant_SidebarHighlightsOwnRow_WhenDisplayNamesAreUnique(t *testing.T) {
	m := sidebarModel(t, "Agent-bob", map[string]string{"Agent-ann": "Ann", "Agent-bob": "Bob"})

	got := youRows(t, m)
	if len(got) != 1 || !strings.Contains(got[0], "Bob") {
		t.Fatalf("highlighted rows = %q, want only Bob's", got)
	}
}
