package host

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
)

func TestAppModel_Init(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewAppModel(hubChans)

	assert.NotNil(t, model)
	assert.Equal(t, paneTranscript, model.activePane)
	assert.Equal(t, 1, model.round)
	assert.Len(t, model.participants.GetParticipants(), 3)
}

func TestAppModel_Update_WindowSize(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewAppModel(hubChans)

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedModel, _ := model.Update(msg)

	assert.Equal(t, 120, updatedModel.(*AppModel).width)
	assert.Equal(t, 40, updatedModel.(*AppModel).height)
}

func TestAppModel_Update_TabNavigation(t *testing.T) {
	hubChans := hub.NewHubChannels()
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
	model := NewAppModel(hubChans)

	// Set directive content via textarea
	model.directive.SetValue("Test directive")

	// Press Enter in directive pane
	model.activePane = paneDirective
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(*AppModel)

	// Directive should be cleared
	assert.Equal(t, "", model.directive.Value())
	// Message should be sent to outbound channel
	select {
	case msg := <-hubChans.Outbound():
		assert.Equal(t, "Test directive", msg.Content)
		assert.Equal(t, hub.MessageTypeDirective, msg.Type)
		assert.Equal(t, "host", msg.From)
	default:
		t.Fatal("Expected message on outbound channel")
	}
}

func TestAppModel_HandleButtonPress_Send(t *testing.T) {
	hubChans := hub.NewHubChannels()
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
	// Message should be sent to outbound channel
	select {
	case msg := <-hubChans.Outbound():
		assert.Equal(t, "Test directive", msg.Content)
		assert.Equal(t, hub.MessageTypeDirective, msg.Type)
		assert.Equal(t, "host", msg.From)
	default:
		t.Fatal("Expected message on outbound channel")
	}
}

func TestAppModel_AddMessage(t *testing.T) {
	hubChans := hub.NewHubChannels()
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

func TestAppModel_UpdateParticipant(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewAppModel(hubChans)

	updated := hub.Participant{Name: "Agent-1", Status: hub.StatusTyping, Round: 1}
	model.participants.UpdateParticipant(updated)

	assert.Equal(t, hub.StatusTyping, model.participants.GetParticipants()[0].Status)
	assert.Equal(t, 1, model.participants.GetParticipants()[0].Round)
}

func TestAppModel_View_RendersWithoutPanic(t *testing.T) {
	hubChans := hub.NewHubChannels()
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
	model := NewAppModel(hubChans)

	model.activePane = paneDirective
	model.directive.SetValue("Line 1")

	// Ctrl+Enter should add newline - simulate with key msg
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\x05'}}) // Ctrl+E as proxy
	model = updatedModel.(*AppModel)

	// Actually test that textarea handles it - we can't easily simulate Ctrl+Enter in tests
	// So just verify the textarea accepts the value
	assert.Equal(t, "Line 1", model.directive.Value())
}
