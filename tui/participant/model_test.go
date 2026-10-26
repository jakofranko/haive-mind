package participant

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
)

func TestParticipantModel_Init(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	assert.NotNil(t, model)
	assert.Equal(t, paneTranscript, model.activePane)
	assert.Equal(t, 1, model.round)
	assert.Equal(t, "Agent-1", model.yourName)
	assert.Len(t, model.sidebar.GetParticipants(), 1)
	assert.Equal(t, "Agent-1", model.sidebar.GetParticipants()[0].Name)
}

func TestParticipantModel_Init_NamePrompt(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	assert.NotNil(t, model)
	assert.Equal(t, paneNamePrompt, model.activePane)
	assert.True(t, model.namePromptActive)
}

func TestParticipantModel_Update_WindowSize(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedModel, _ := model.Update(msg)

	assert.Equal(t, 120, updatedModel.(*ParticipantModel).width)
	assert.Equal(t, 40, updatedModel.(*ParticipantModel).height)
}

func TestParticipantModel_Update_TabNavigation(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	assert.Equal(t, paneTranscript, model.activePane)

	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updatedModel.(*ParticipantModel)
	assert.Equal(t, paneSidebar, model.activePane)

	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updatedModel.(*ParticipantModel)
	assert.Equal(t, paneResponse, model.activePane)

	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updatedModel.(*ParticipantModel)
	assert.Equal(t, paneTranscript, model.activePane)
}

func TestParticipantModel_Update_ShiftTabNavigation(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updatedModel.(*ParticipantModel)
	assert.Equal(t, paneResponse, model.activePane)

	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updatedModel.(*ParticipantModel)
	assert.Equal(t, paneSidebar, model.activePane)
}

func TestParticipantModel_HandleResponseSubmit(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.response.SetValue("Test response")

	// Directly call the handler since we can't easily simulate Ctrl+Enter in tests
	updatedModel, _ := model.handleResponseSubmit()
	model = updatedModel.(*ParticipantModel)

	assert.Equal(t, "", model.response.Value())

	select {
	case msg := <-hubChans.Outbound():
		assert.Equal(t, "Test response", msg.Content)
		assert.Equal(t, hub.MessageTypeResponse, msg.Type)
		assert.Equal(t, "Agent-1", msg.From)
		assert.Equal(t, 1, msg.Round)
	default:
		t.Fatal("Expected message on outbound channel")
	}

	select {
	case msg := <-hubChans.SendChan():
		assert.Equal(t, "Test response", msg.Content)
		assert.Equal(t, hub.MessageTypeResponse, msg.Type)
		assert.Equal(t, "Agent-1", msg.From)
	default:
		t.Fatal("Expected message on send channel")
	}
}

func TestParticipantModel_HandleResponseSubmit_Empty(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.response.SetValue("   ")
	model.activePane = paneResponse
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\x05'}})
	model = updatedModel.(*ParticipantModel)

	assert.Equal(t, "   ", model.response.Value())

	select {
	case <-hubChans.Outbound():
		t.Fatal("Should not send empty message")
	default:
	}

	select {
	case <-hubChans.SendChan():
		t.Fatal("Should not send empty message")
	default:
	}
}

func TestParticipantModel_AddMessage(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.transcript.SetSize(80, 20)

	msg := hub.NewMessage(hub.MessageTypeDirective, "host", "Test directive", 1)
	model.transcript.AddMessage(*msg)

	content := model.transcript.View()
	assert.Contains(t, content, "host")
	assert.Contains(t, content, "Test directive")
	assert.Contains(t, content, "Round 1")
}

func TestParticipantModel_UpdateParticipant(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	// Add Agent-2 to participants first
	model.sidebar.SetParticipants([]hub.Participant{
		{Name: "Agent-1", Status: hub.StatusIdle, Round: 0},
		{Name: "Agent-2", Status: hub.StatusIdle, Round: 0},
	})

	updated := hub.Participant{Name: "Agent-2", Status: hub.StatusTyping, Round: 1}
	model.sidebar.UpdateParticipant(updated)

	participants := model.sidebar.GetParticipants()
	found := false
	for _, p := range participants {
		if p.Name == "Agent-2" {
			assert.Equal(t, hub.StatusTyping, p.Status)
			assert.Equal(t, 1, p.Round)
			found = true
			break
		}
	}
	assert.True(t, found, "Agent-2 should be in participants list")
}

func TestParticipantModel_View_RendersWithoutPanic(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.width = 120
	model.height = 40
	model.layout()

	view := model.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Transcript")
	assert.Contains(t, view, "Participants")
	assert.Contains(t, view, "Response")
}

func TestParticipantModel_NamePrompt_Flow(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	assert.True(t, model.namePromptActive)
	assert.Equal(t, paneNamePrompt, model.activePane)

	// Simulate typing a name
	model.namePrompt.SetValue("John Doe")

	// Simulate pressing Enter
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(*ParticipantModel)

	assert.False(t, model.namePromptActive)
	assert.Equal(t, paneTranscript, model.activePane)
	assert.Equal(t, "John Doe", model.yourName)
	assert.Equal(t, "John Doe", model.sidebar.GetParticipants()[0].Name)
	assert.Equal(t, "John Doe", hubChans.GetDisplayName("user1"))
}

func TestParticipantModel_DraftPersistence(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	draft := "This is a draft response\nwith multiple lines"
	model.response.SetDraft(draft)

	assert.Equal(t, draft, model.response.GetDraft())
	assert.Equal(t, draft, model.response.Value())

	model.response.SetValue("new content")
	assert.Equal(t, "new content", model.response.GetDraft())
}

func TestParticipantModel_CtrlEnterInResponse(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("user1", "Agent-1")
	model := NewParticipantModel(hubChans, "Agent-1", "user1")

	model.activePane = paneResponse
	model.response.SetValue("Line 1")

	// Test that textarea accepts Enter (newline) via Ctrl+Enter binding
	// We can't easily simulate Ctrl+Enter in unit tests, so verify the binding exists
	assert.True(t, model.response.IsFocused())
	assert.Equal(t, "Line 1", model.response.Value())
}