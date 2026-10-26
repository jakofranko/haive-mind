package participant

import (
	"testing"
	"time"

	"github.com/sebdah/goldie/v2"
	"jakofranko/haive-mind/hub"
)

func TestSnapshot_ParticipantTUI_TranscriptFocused(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.width = 120
	model.height = 40
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "participant_tui_transcript_focused", []byte(model.View()))
}

func TestSnapshot_ParticipantTUI_SidebarFocused(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.width = 120
	model.height = 40
	model.activePane = paneSidebar
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "participant_tui_sidebar_focused", []byte(model.View()))
}

func TestSnapshot_ParticipantTUI_ResponseFocused(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.width = 120
	model.height = 40
	model.activePane = paneResponse
	model.response.SetValue("Test response content")
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "participant_tui_response_focused", []byte(model.View()))
}

func TestSnapshot_ParticipantTUI_ResponseWithDraft(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.width = 120
	model.height = 40
	model.activePane = paneResponse
	model.response.SetValue("Line 1\nLine 2\nLine 3")
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "participant_tui_response_with_draft", []byte(model.View()))
}

func TestSnapshot_ParticipantTUI_TranscriptWithMessages(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.width = 120
	model.height = 40
	model.transcript.SetSize(80, 20)

	fixedTime := time.Date(2026, 1, 15, 10, 30, 45, 0, time.UTC)

	msg1 := &hub.Message{
		ID:        "test-1",
		Type:      hub.MessageTypeDirective,
		From:      "host",
		Content:   "**Bold** and *italic* text",
		Timestamp: fixedTime,
		Round:     1,
	}
	model.transcript.AddMessage(*msg1)

	msg2 := &hub.Message{
		ID:        "test-2",
		Type:      hub.MessageTypeResponse,
		From:      "Agent-2",
		Content:   "Response with `code`",
		Timestamp: fixedTime,
		Round:     1,
	}
	model.transcript.AddMessage(*msg2)

	msg3 := &hub.Message{
		ID:        "test-3",
		Type:      hub.MessageTypeSystem,
		From:      "system",
		Content:   "System message",
		Timestamp: fixedTime,
		Round:     1,
	}
	model.transcript.AddMessage(*msg3)

	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "participant_transcript_with_messages", []byte(model.View()))
}

func TestSnapshot_ParticipantTUI_SidebarAllStatuses(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.width = 120
	model.height = 40
	model.activePane = paneSidebar
	model.sidebar.SetParticipants([]hub.Participant{
		{Name: "Agent-1", DisplayName: "Agent-1", Status: hub.StatusIdle, Round: 1},
		{Name: "Agent-2", DisplayName: "Agent-2", Status: hub.StatusTyping, Round: 1},
		{Name: "Agent-3", DisplayName: "Agent-3", Status: hub.StatusSubmitted, Round: 1},
		{Name: "Agent-4", DisplayName: "Agent-4", Status: hub.StatusReady, Round: 1},
	})
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "participant_sidebar_all_statuses", []byte(model.View()))
}

func TestSnapshot_ParticipantTUI_SmallTerminal(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.width = 80
	model.height = 24
	model.layout()

	g := goldie.New(t, goldie.WithFixtureDir("testdata"))
	g.Assert(t, "participant_small_terminal", []byte(model.View()))
}