package participant

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
)

// catchupRig is a participant on a started hub, with a screen.
func catchupRig(t *testing.T) (*ParticipantModel, hub.Hub) {
	t.Helper()
	h := hub.NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })
	h.SetDisplayName("Agent-alice", "Alice")
	h.RegisterParticipant(hub.NewParticipantEvent("Agent-alice", "Alice", hub.StatusIdle, hub.ParticipantEventConnect))
	m := NewParticipantModel(h, "Agent-alice", "alice")
	m.Init()
	return sized(m), h
}

// receive takes the next event the model is given by the hub and applies it
// through Update, as the program loop does.
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

func recorded(from, content string) *hub.Message {
	return hub.NewMessage(hub.MessageTypeDirective, from, content, 1)
}

func TestSnapshot_AddressedToMe_ShowsTheRecordedConversation(t *testing.T) {
	m, h := catchupRig(t)

	h.Send(context.Background(), hub.NewTranscriptSnapshot(1, "Agent-alice",
		[]*hub.Message{recorded("host", "pick a database")}, nil))
	m = receive(t, m)

	assert.Contains(t, visible(m.View()), "pick a database")
}

// The snapshot is older than what the participant already saw live, so it
// goes before it.
func TestSnapshot_GoesBeforeWhatIsAlreadyShown(t *testing.T) {
	m, h := catchupRig(t)
	ctx := context.Background()

	h.Send(ctx, recorded("host", "live directive"))
	m = receive(t, m)
	h.Send(ctx, hub.NewTranscriptSnapshot(1, "Agent-alice",
		[]*hub.Message{recorded("host", "first directive"), recorded("host", "second directive")}, nil))
	m = receive(t, m)

	screen := visible(m.View())
	assert.Contains(t, screen, "first directive")
	assert.Less(t, strings.Index(screen, "first directive"), strings.Index(screen, "second directive"))
	assert.Less(t, strings.Index(screen, "second directive"), strings.Index(screen, "live directive"))
}

// A live message that arrived before the snapshot, and is also in it, is shown
// once, after the snapshot's older messages.
func TestSnapshot_LiveMessageFirst_OlderMessagesGoBeforeIt(t *testing.T) {
	m, h := catchupRig(t)
	ctx := context.Background()
	older, live := recorded("host", "older-entry"), recorded("host", "live-entry")

	h.Send(ctx, live)
	m = receive(t, m)
	h.Send(ctx, hub.NewTranscriptSnapshot(1, "Agent-alice", []*hub.Message{older, live}, nil))
	m = receive(t, m)

	screen := visible(m.View())
	assert.Equal(t, 1, strings.Count(screen, "live-entry"))
	assert.Less(t, strings.Index(screen, "older-entry"), strings.Index(screen, "live-entry"))
}

// A snapshot for someone else changes nothing.
func TestSnapshot_ForAnotherParticipant_ChangesNothing(t *testing.T) {
	m, h := catchupRig(t)
	ctx := context.Background()
	other := recorded("host", "bobs directive")
	last := hub.NewMessage(hub.MessageTypeResponse, "Agent-bob", "bobs secret answer", 1)

	h.Send(ctx, hub.NewTranscriptSnapshot(1, "Agent-bob", []*hub.Message{other}, last))
	h.Send(ctx, recorded("host", "marker"))
	m = receive(t, m) // the marker is the first thing the model is given

	screen := visible(m.View())
	assert.Contains(t, screen, "marker")
	assert.NotContains(t, screen, "bobs directive")
	m = step(m, flagUpdate("Agent-alice"))
	assert.Equal(t, "", m.response.Value())
}

// After a snapshot carrying their last submission, being flagged puts that
// answer back for editing.
func TestSnapshot_LastSubmission_IsOfferedWhenFlaggedToRevise(t *testing.T) {
	m, h := catchupRig(t)
	last := hub.NewMessage(hub.MessageTypeResponse, "Agent-alice", "use postgres", 1)

	h.Send(context.Background(), hub.NewTranscriptSnapshot(1, "Agent-alice", nil, last))
	m = receive(t, m)
	m = step(m, flagUpdate("Agent-alice"))

	assert.Equal(t, "use postgres", m.response.Value())
}

// On a reconnect the coordinator sends Round state before the snapshot, so a
// participant who is already flagged gets their answer when the snapshot lands.
func TestSnapshot_LastSubmission_IsOfferedWhenAlreadyFlagged(t *testing.T) {
	m, h := catchupRig(t)
	last := hub.NewMessage(hub.MessageTypeResponse, "Agent-alice", "use postgres", 1)

	m = step(m, flagUpdate("Agent-alice"))
	h.Send(context.Background(), hub.NewTranscriptSnapshot(1, "Agent-alice", nil, last))
	m = receive(t, m)

	assert.Equal(t, "use postgres", m.response.Value())
}

// Text already being typed is left alone, as for any revise.
func TestSnapshot_LastSubmission_DoesNotOverwriteTyping(t *testing.T) {
	m, h := catchupRig(t)
	last := hub.NewMessage(hub.MessageTypeResponse, "Agent-alice", "use postgres", 1)

	h.Send(context.Background(), hub.NewTranscriptSnapshot(1, "Agent-alice", nil, last))
	m = receive(t, m)
	m.activePane = paneResponse
	m.updateActiveStates()
	m = typeRunes(m, "sqlite")
	m = step(m, flagUpdate("Agent-alice"))

	assert.Equal(t, "sqlite", m.response.Value())
}
