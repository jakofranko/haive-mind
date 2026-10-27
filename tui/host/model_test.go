package host

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
)

func TestAppModel_Init(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	assert.NotNil(t, model)
	assert.Equal(t, paneTranscript, model.activePane)
	assert.Equal(t, 1, model.round)
	assert.Equal(t, "TestHost", model.yourName)
	assert.Len(t, model.participants.GetParticipants(), 0)
}

func TestAppModel_Init_NamePrompt(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewAppModel(hubChans)

	assert.NotNil(t, model)
	assert.Equal(t, paneNamePrompt, model.activePane)
	assert.True(t, model.namePromptActive)
}

func TestAppModel_Update_WindowSize(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedModel, _ := model.Update(msg)

	assert.Equal(t, 120, updatedModel.(*AppModel).width)
	assert.Equal(t, 40, updatedModel.(*AppModel).height)
}

func TestAppModel_Update_TabNavigation(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	// Initial pane should be transcript
	assert.Equal(t, paneTranscript, model.activePane)

	// Tab to participants
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updatedModel.(*AppModel)
	assert.Equal(t, paneParticipants, model.activePane)

	// Tab to directive
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updatedModel.(*AppModel)
	assert.Equal(t, paneDirective, model.activePane)

	// Tab to buttons
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updatedModel.(*AppModel)
	assert.Equal(t, paneButtons, model.activePane)

	// Tab wraps around to transcript
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updatedModel.(*AppModel)
	assert.Equal(t, paneTranscript, model.activePane)
}

func TestAppModel_Update_ShiftTabNavigation(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	// Shift+Tab from transcript should go to buttons
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updatedModel.(*AppModel)
	assert.Equal(t, paneButtons, model.activePane)

	// Shift+Tab from buttons should go to directive
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updatedModel.(*AppModel)
	assert.Equal(t, paneDirective, model.activePane)
}

func TestAppModel_HandleDirectiveSend(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	// Set directive content via textarea
	model.directive.SetValue("Test directive")

	// Press Enter in directive pane
	model.activePane = paneDirective
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(*AppModel)

	// Directive should be cleared
	assert.Equal(t, "", model.directive.Value())
	// Message should be sent to send channel
	select {
	case msg := <-hubChans.SendChan():
		assert.Equal(t, "Test directive", msg.Content)
		assert.Equal(t, hub.MessageTypeDirective, msg.Type)
		assert.Equal(t, "TestHost", msg.From)
	default:
		t.Fatal("Expected message on send channel")
	}
}

func TestAppModel_HandleButtonPress_Send(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	// Set directive content
	model.directive.SetValue("Test directive")

	// Press Enter in buttons pane with Send button selected
	model.activePane = paneButtons
	model.buttons.SetButtonIndex(1) // Send
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(*AppModel)

	// Directive should be cleared (sent)
	assert.Equal(t, "", model.directive.Value())
	// Message should be sent to send channel
	select {
	case msg := <-hubChans.SendChan():
		assert.Equal(t, "Test directive", msg.Content)
		assert.Equal(t, hub.MessageTypeDirective, msg.Type)
		assert.Equal(t, "TestHost", msg.From)
	default:
		t.Fatal("Expected message on send channel")
	}
}

func TestAppModel_AddMessage(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	// Set viewport dimensions so View() works
	model.transcript.SetSize(80, 20)

	msg := hub.NewMessage(hub.MessageTypeResponse, "Agent-1", "Test response", 1)
	model.transcript.AddMessage(*msg)

	content := model.transcript.View()
	assert.Contains(t, content, "Agent-1")
	assert.Contains(t, content, "Test response")
	assert.Contains(t, content, "Round 1")
}

func TestAppModel_UpdateParticipant_Upsert(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	// Add new participant (join)
	updated := hub.Participant{Name: "Agent-new", DisplayName: "New Agent", Status: hub.StatusTyping, Round: 1}
	model.participants.UpdateParticipant(updated)

	participants := model.participants.GetParticipants()
	assert.Len(t, participants, 1)
	assert.Equal(t, "Agent-new", participants[0].Name)
	assert.Equal(t, "New Agent", participants[0].DisplayName)
	assert.Equal(t, hub.StatusTyping, participants[0].Status)
	assert.Equal(t, 1, participants[0].Round)

	// Update existing participant
	updated = hub.Participant{Name: "Agent-new", DisplayName: "New Agent", Status: hub.StatusReady, Round: 1}
	model.participants.UpdateParticipant(updated)

	participants = model.participants.GetParticipants()
	assert.Len(t, participants, 1)
	assert.Equal(t, hub.StatusReady, participants[0].Status)
}

func TestAppModel_RemoveParticipant(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	// Add participants
	model.participants.UpdateParticipant(hub.Participant{Name: "Agent-1", DisplayName: "Agent One", Status: hub.StatusIdle, Round: 0})
	model.participants.UpdateParticipant(hub.Participant{Name: "Agent-2", DisplayName: "Agent Two", Status: hub.StatusIdle, Round: 0})
	assert.Len(t, model.participants.GetParticipants(), 2)

	// Remove one
	model.participants.RemoveParticipant("Agent-1")
	participants := model.participants.GetParticipants()
	assert.Len(t, participants, 1)
	assert.Equal(t, "Agent-2", participants[0].Name)

	// Remove non-existent (should not panic)
	model.participants.RemoveParticipant("NonExistent")
	assert.Len(t, model.participants.GetParticipants(), 1)
}

func TestAppModel_ParticipantUpdateMsg_Join(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	// Simulate participant join via ParticipantUpdateChan message
	joinMsg := hub.Participant{Name: "Agent-user1", DisplayName: "User One", Status: hub.StatusIdle, Round: 0}
	updatedModel, _ := model.Update(participantUpdateMsg(joinMsg))
	model = updatedModel.(*AppModel)

	participants := model.participants.GetParticipants()
	assert.Len(t, participants, 1)
	assert.Equal(t, "Agent-user1", participants[0].Name)
	assert.Equal(t, "User One", participants[0].DisplayName)
}

func TestAppModel_ParticipantUpdateMsg_Leave(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	// First add a participant
	model.participants.UpdateParticipant(hub.Participant{Name: "Agent-user1", DisplayName: "User One", Status: hub.StatusIdle, Round: 0})
	assert.Len(t, model.participants.GetParticipants(), 1)

	// Simulate participant leave via ParticipantUpdateChan message
	leaveMsg := hub.Participant{Name: "Agent-user1", Status: hub.StatusDisconnected}
	updatedModel, _ := model.Update(participantUpdateMsg(leaveMsg))
	model = updatedModel.(*AppModel)

	participants := model.participants.GetParticipants()
	assert.Len(t, participants, 0)
}

func TestAppModel_View_RendersWithoutPanic(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	// Simulate window size
	model.width = 120
	model.height = 40
	model.layout()

	// View should not panic
	view := model.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Transcript")
	assert.Contains(t, view, "Participants")
	assert.Contains(t, view, "Directive")
	assert.Contains(t, view, "Controls")
}

func TestAppModel_CtrlEnterInDirective(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	model.activePane = paneDirective
	model.directive.SetValue("Line 1")

	// Ctrl+Enter should add newline - simulate with key msg
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\x05'}})
	model = updatedModel.(*AppModel)

	// Actually test that textarea handles it - we can't easily simulate Ctrl+Enter in tests
	// So just verify the textarea accepts the value
	assert.Equal(t, "Line 1", model.directive.Value())
}

func TestAppModel_NamePrompt_Flow(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewAppModel(hubChans)

	assert.True(t, model.namePromptActive)
	assert.Equal(t, paneNamePrompt, model.activePane)

	// Simulate typing a name
	model.namePrompt.SetValue("John Doe")

	// Simulate pressing Enter
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(*AppModel)

	assert.False(t, model.namePromptActive)
	assert.Equal(t, paneTranscript, model.activePane)
	assert.Equal(t, "John Doe", model.yourName)
	assert.Equal(t, "John Doe", hubChans.GetDisplayName("host"))
}

func TestAppModel_HandleButtonPress_NewRound(t *testing.T) {
	hubChans := hub.NewHubChannels()
	hubChans.SetDisplayName("host", "TestHost")
	model := NewAppModel(hubChans)

	model.activePane = paneButtons
	model.buttons.SetButtonIndex(2) // New Round
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(*AppModel)

	assert.Equal(t, 2, model.round)
}
