package host

import (
	"os"
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"jakofranko/haive-mind/hub"
)

func TestHostModel_Init(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	assert.NotNil(t, model)
	assert.Equal(t, paneTranscript, model.activePane)
	assert.Equal(t, 1, model.round)
	assert.Len(t, model.participants, 3)
}

func TestHostModel_Update_WindowSize(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedModel, _ := model.Update(msg)

	assert.Equal(t, 120, updatedModel.(*HostModel).width)
	assert.Equal(t, 40, updatedModel.(*HostModel).height)
}

func TestHostModel_Update_TabNavigation(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	// Initial pane should be transcript
	assert.Equal(t, paneTranscript, model.activePane)

	// Tab to participants
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updatedModel.(*HostModel)
	assert.Equal(t, paneParticipants, model.activePane)

	// Tab to directive
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updatedModel.(*HostModel)
	assert.Equal(t, paneDirective, model.activePane)

	// Tab to buttons
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updatedModel.(*HostModel)
	assert.Equal(t, paneButtons, model.activePane)

	// Tab wraps around to transcript
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updatedModel.(*HostModel)
	assert.Equal(t, paneTranscript, model.activePane)
}

func TestHostModel_Update_ShiftTabNavigation(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	// Shift+Tab from transcript should go to buttons
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updatedModel.(*HostModel)
	assert.Equal(t, paneButtons, model.activePane)

	// Shift+Tab from buttons should go to directive
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updatedModel.(*HostModel)
	assert.Equal(t, paneDirective, model.activePane)
}

func TestHostModel_HandleDirectiveSend(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	// Set directive content
	model.directive = "Test directive"
	model.directiveVP.SetContent("Test directive")

	// Press Enter in directive pane
	model.activePane = paneDirective
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(*HostModel)

	// Directive should be cleared
	assert.Equal(t, "", model.directive)
	// Message should be sent to outbound channel
	select {
	case msg := <-hubChans.Outbound:
		assert.Equal(t, "Test directive", msg.Content)
		assert.Equal(t, hub.MessageTypeDirective, msg.Type)
		assert.Equal(t, "host", msg.From)
	default:
		t.Fatal("Expected message on outbound channel")
	}
}

func TestHostModel_HandleButtonPress_Aggregate(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.activePane = paneButtons
	model.buttonIndex = 0 // Aggregate

	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(*HostModel)

	select {
	case cmd := <-hubChans.Commands:
		assert.Equal(t, hub.CmdAggregate, cmd.Type)
		assert.Equal(t, 1, cmd.Payload)
	default:
		t.Fatal("Expected aggregate command")
	}
}

func TestHostModel_HandleButtonPress_NewRound(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.activePane = paneButtons
	model.buttonIndex = 2 // New Round

	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(*HostModel)

	assert.Equal(t, 2, model.round)

	select {
	case cmd := <-hubChans.Commands:
		assert.Equal(t, hub.CmdNewRound, cmd.Type)
		assert.Equal(t, 2, cmd.Payload)
	default:
		t.Fatal("Expected new round command")
	}
}

func TestHostModel_HandleButtonPress_Export(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.activePane = paneButtons
	model.buttonIndex = 3 // Export

	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updatedModel.(*HostModel)

	select {
	case cmd := <-hubChans.Commands:
		assert.Equal(t, hub.CmdExport, cmd.Type)
	default:
		t.Fatal("Expected export command")
	}
}

func TestHostModel_ButtonNavigation(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.activePane = paneButtons
	model.buttonIndex = 0

	// Right arrow should increment button index
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updatedModel.(*HostModel)
	assert.Equal(t, 1, model.buttonIndex)

	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updatedModel.(*HostModel)
	assert.Equal(t, 2, model.buttonIndex)

	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updatedModel.(*HostModel)
	assert.Equal(t, 3, model.buttonIndex)

	// Right arrow at max should not increment
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updatedModel.(*HostModel)
	assert.Equal(t, 3, model.buttonIndex)

	// Left arrow should decrement
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updatedModel.(*HostModel)
	assert.Equal(t, 2, model.buttonIndex)

	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updatedModel.(*HostModel)
	assert.Equal(t, 1, model.buttonIndex)

	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updatedModel.(*HostModel)
	assert.Equal(t, 0, model.buttonIndex)

	// Left arrow at min should not decrement
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updatedModel.(*HostModel)
	assert.Equal(t, 0, model.buttonIndex)
}

func TestHostModel_AddMessage(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	// Set viewport dimensions so View() works
	model.transcript.Width = 80
	model.transcript.Height = 20

	msg := hub.NewMessage(hub.MessageTypeResponse, "Agent-1", "Test response", 1)
	model.addMessage(*msg)

	content := model.transcript.View()
	assert.Contains(t, content, "Agent-1")
	assert.Contains(t, content, "Test response")
	assert.Contains(t, content, "Round 1")
}

func TestHostModel_UpdateParticipant(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	updated := hub.Participant{Name: "Agent-1", Status: hub.StatusTyping, Round: 1}
	model.updateParticipant(updated)

	assert.Equal(t, hub.StatusTyping, model.participants[0].Status)
	assert.Equal(t, 1, model.participants[0].Round)
}

func TestHostModel_ExportTranscript(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	// Set viewport dimensions so View() works
	model.transcript.Width = 80
	model.transcript.Height = 20

	// Add a message
	msg := hub.NewMessage(hub.MessageTypeResponse, "Agent-1", "Test response", 1)
	model.addMessage(*msg)

	// Export to temp file
	tmpFile := t.TempDir() + "/transcript.json"
	err := model.ExportTranscript(tmpFile)
	require.NoError(t, err)

	// Verify file exists and has content
	data, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Agent-1")
	assert.Contains(t, string(data), "Test response")
}

func TestHostModel_View_RendersWithoutPanic(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	// Simulate window size
	model.width = 120
	model.height = 40
	model.resizePanes()

	// View should not panic
	view := model.View()
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Transcript")
	assert.Contains(t, view, "Participants")
	assert.Contains(t, view, "Directive")
	assert.Contains(t, view, "Controls")
}

func TestHostModel_CtrlEnterInDirective(t *testing.T) {
	hubChans := hub.NewHubChannels()
	model := NewHostModel(hubChans)

	model.activePane = paneDirective
	model.directive = "Line 1"
	model.directiveVP.SetContent("Line 1")

	// Ctrl+Enter should add newline
	model.HandleCtrlEnter()
	assert.Equal(t, "Line 1\n", model.directive)
}
