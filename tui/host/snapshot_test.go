package host

import (
	"testing"
	"time"

	"github.com/sebdah/goldie/v2"
	"jakofranko/haive-mind/hub"
)

func TestSnapshot_FullTUI_TranscriptFocused(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.width = 120
	model.height = 40
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "full_tui_transcript_focused", []byte(model.View()))
}

func TestSnapshot_FullTUI_ParticipantsFocused(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.width = 120
	model.height = 40
	model.activePane = paneParticipants
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "full_tui_participants_focused", []byte(model.View()))
}

func TestSnapshot_FullTUI_DirectiveFocused(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.width = 120
	model.height = 40
	model.activePane = paneDirective
	model.directiveInput.SetValue("Test directive content")
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "full_tui_directive_focused", []byte(model.View()))
}

func TestSnapshot_FullTUI_ButtonsFocused(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.width = 120
	model.height = 40
	model.activePane = paneButtons
	model.buttonIndex = 0
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "full_tui_buttons_focused_aggregate", []byte(model.View()))

	model.buttonIndex = 1
	g.Assert(t, "full_tui_buttons_focused_send", []byte(model.View()))

	model.buttonIndex = 2
	g.Assert(t, "full_tui_buttons_focused_newround", []byte(model.View()))

	model.buttonIndex = 3
	g.Assert(t, "full_tui_buttons_focused_export", []byte(model.View()))
}

func TestSnapshot_Transcript_WithMessages(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.width = 120
	model.height = 40
	model.transcript.Width = 80
	model.transcript.Height = 20

	// Fixed timestamp for reproducible snapshots
	fixedTime := time.Date(2026, 1, 15, 10, 30, 45, 0, time.UTC)

	// Add multiple messages with fixed timestamp
	msg1 := &hub.Message{
		ID:        "test-1",
		Type:      hub.MessageTypeDirective,
		From:      "host",
		Content:   "**Bold** and *italic* text",
		Timestamp: fixedTime,
		Round:     1,
	}
	model.addMessage(*msg1)

	msg2 := &hub.Message{
		ID:        "test-2",
		Type:      hub.MessageTypeResponse,
		From:      "Agent-1",
		Content:   "Response with `code`",
		Timestamp: fixedTime,
		Round:     1,
	}
	model.addMessage(*msg2)

	msg3 := &hub.Message{
		ID:        "test-3",
		Type:      hub.MessageTypeSystem,
		From:      "system",
		Content:   "System message",
		Timestamp: fixedTime,
		Round:     1,
	}
	model.addMessage(*msg3)

	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "transcript_with_messages", []byte(model.View()))
}

func TestSnapshot_Participants_AllStatuses(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.width = 120
	model.height = 40
	model.activePane = paneParticipants
	model.participants = []hub.Participant{
		{Name: "Agent-1", Status: hub.StatusIdle, Round: 1},
		{Name: "Agent-2", Status: hub.StatusTyping, Round: 1},
		{Name: "Agent-3", Status: hub.StatusSubmitted, Round: 1},
		{Name: "Agent-4", Status: hub.StatusReady, Round: 1},
	}
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "participants_all_statuses", []byte(model.View()))
}

func TestSnapshot_SmallTerminal(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.width = 80
	model.height = 24
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "small_terminal", []byte(model.View()))
}
