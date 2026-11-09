package host

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
)

// popUpdate waits for the next participant update the model's subscription
// delivers, the way its Update loop would.
func popUpdate(t *testing.T, m *AppModel) tea.Msg {
	t.Helper()
	res := make(chan tea.Msg, 1)
	go func() { res <- m.waitForEvent()() }()
	select {
	case msg := <-res:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("no participant update arrived")
		return nil
	}
}

// #29: when a participant disconnects the host's list must drop them. The hub
// has already forgotten them by the time the event is handled, so the event
// itself has to carry enough to remove the row.
func TestHost_ParticipantDisconnect_RemovesThemFromTheList(t *testing.T) {
	h := hub.NewHub()
	h.SetDisplayName(hub.HostID, "TestHost")
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	m := NewAppModel(h)

	h.RegisterParticipant(hub.NewParticipantEvent("Agent-alice", "Alice", hub.StatusIdle, hub.ParticipantEventConnect))
	updated, _ := m.Update(popUpdate(t, m))
	m = updated.(*AppModel)
	if got := len(m.participants.GetParticipants()); got != 1 {
		t.Fatalf("after connect the list has %d rows, want 1", got)
	}

	h.UnregisterParticipant("Agent-alice")
	updated, _ = m.Update(popUpdate(t, m))
	m = updated.(*AppModel)
	if got := m.participants.GetParticipants(); len(got) != 0 {
		t.Errorf("after disconnect the list still has %+v", got)
	}
}
