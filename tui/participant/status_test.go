package participant

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
)

// G20: the host's list can only be accurate if the hub is told what each
// participant is doing.
func TestParticipantStatus_FollowsTypingSubmittingAndNewRound(t *testing.T) {
	h := serverHub(t)
	m := join(t, h, "Agent-alice", "Alice")
	status := statusOf(t, h, "Agent-alice")

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

	h.StartRound(1)
	deadline := time.Now().Add(5 * time.Second)
	for status() != hub.StatusIdle {
		if time.Now().After(deadline) {
			t.Fatalf("after a new round: %q, want idle", status())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Deleting everything they typed puts them back to idle.
func TestParticipantStatus_EmptyingTheBox_ReturnsToIdle(t *testing.T) {
	h := serverHub(t)
	m := join(t, h, "Agent-alice", "Alice")
	status := statusOf(t, h, "Agent-alice")

	m = typeRunes(m, "a")
	if got := status(); got != hub.StatusTyping {
		t.Fatalf("after typing: %q, want typing", got)
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if got := status(); got != hub.StatusIdle {
		t.Errorf("after emptying the box: %q, want idle", got)
	}
}
