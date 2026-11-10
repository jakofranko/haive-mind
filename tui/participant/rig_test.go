package participant

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/coordinator"
	"jakofranko/haive-mind/hub"
)

// serverHub is a started hub with the Coordinator beside it, as the server
// runs them, so a session is caught up the way a real one is.
func serverHub(t *testing.T) hub.Hub {
	t.Helper()
	h := hub.NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })
	if err := coordinator.New(h).Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return h
}

// join is what the SSH server does for a participant: it makes the model
// (whose Session client subscribes), then registers the participant. It
// returns the model with a screen, once it has caught up, with the response
// box focused.
func join(t *testing.T, h hub.Hub, id, name string) *ParticipantModel {
	t.Helper()
	h.SetDisplayName(id, name)
	m := NewParticipantModel(h, id, "ssh-"+name)
	t.Cleanup(m.Close)
	h.RegisterParticipant(hub.NewParticipantEvent(id, name, hub.StatusIdle, hub.ParticipantEventConnect))
	m = receive(t, m) // Catch-up is always the first event
	m = sized(m)
	m.activePane = paneResponse
	m.updateActiveStates()
	return m
}

// statusOf reads what the hub says the participant is doing.
func statusOf(t *testing.T, h hub.Hub, id string) func() hub.ParticipantStatus {
	return func() hub.ParticipantStatus {
		for _, p := range h.GetParticipants() {
			if p.ID == id {
				return p.Status
			}
		}
		t.Fatalf("%s is not registered", id)
		return ""
	}
}

// receive takes the next event the model is given and applies it through
// Update, as the program loop does.
func receive(t *testing.T, m *ParticipantModel) *ParticipantModel {
	t.Helper()
	res := make(chan tea.Msg, 1)
	go func() { res <- m.waitForEvent()() }()
	select {
	case msg := <-res:
		return step(m, msg)
	case <-time.After(5 * time.Second):
		t.Fatal("model was given no event")
		return m
	}
}

// receiveUntil applies events until the screen shows want.
func receiveUntil(t *testing.T, m *ParticipantModel, want func(screen string) bool) *ParticipantModel {
	t.Helper()
	for !want(visible(m.View())) {
		m = receive(t, m)
	}
	return m
}

func shows(text string) func(string) bool {
	return func(screen string) bool { return strings.Contains(screen, text) }
}

func step(m *ParticipantModel, msg tea.Msg) *ParticipantModel {
	updated, _ := m.Update(msg)
	return updated.(*ParticipantModel)
}

func typeRunes(m *ParticipantModel, s string) *ParticipantModel {
	return step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
}

// sized gives the model a screen so View() shows the panes.
func sized(m *ParticipantModel) *ParticipantModel {
	return step(m, tea.WindowSizeMsg{Width: 140, Height: 40})
}

// submitted is alice, who has already sent an answer.
func submitted(t *testing.T, answer string) (*ParticipantModel, hub.Hub) {
	t.Helper()
	h := serverHub(t)
	m := join(t, h, "Agent-alice", "Alice")
	m = typeRunes(m, answer)
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})
	return m, h
}

func flag(h hub.Hub, round int, id string) {
	h.Send(context.Background(), hub.NewFlag(round, id))
}
