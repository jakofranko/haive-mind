package participant

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
)

func TestParticipantModel_ReceivesMessages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hubChans := hub.NewHub()
	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	// Register participants BEFORE creating model (simulating real scenario)
	hubChans.RegisterParticipant(hub.NewParticipantEvent("host", "Host", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	hubChans.RegisterParticipant(hub.NewParticipantEvent("Agent-user1", "Participant One", hub.StatusIdle, 1, hub.ParticipantEventConnect))

	// Create participant model
	model := NewParticipantModel(hubChans, "Agent-user1", "user1")

	// Run program in background
	prog := tea.NewProgram(model)
	go func() {
		if _, err := prog.Run(); err != nil {
			t.Logf("Program error: %v", err)
		}
	}()

	// Wait for subscriptions to be ready
	select {
	case <-model.SubscriptionsReady():
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for subscriptions to be ready")
	}

	// Wait for initial participants to be fetched
	for i := 0; i < 50; i++ {
		participants := model.sidebar.GetParticipants()
		if len(participants) >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Set transcript size (simulate window size)
	model.transcript.SetSize(80, 20)

	// Send a message through the hub
	msg := hub.NewMessage(hub.MessageTypeDirective, "Host", "Test directive", 1)
	hubChans.Send(ctx, msg)

	// Wait for message to be received
	time.Sleep(500 * time.Millisecond)

	// Check if transcript has the message
	transcriptContent := model.transcript.View()
	assert.Contains(t, transcriptContent, "Test directive")
	assert.Contains(t, transcriptContent, "Host")
}

func TestParticipantModel_ReceivesParticipantEvents(t *testing.T) {
	hubChans := hub.NewHub()
	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	// Register initial participants BEFORE creating model
	hubChans.RegisterParticipant(hub.NewParticipantEvent("host", "Host", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	hubChans.RegisterParticipant(hub.NewParticipantEvent("Agent-user1", "Participant One", hub.StatusIdle, 1, hub.ParticipantEventConnect))

	// Create participant model
	model := NewParticipantModel(hubChans, "Agent-user1", "user1")

	// Run program in background
	prog := tea.NewProgram(model)
	go func() {
		if _, err := prog.Run(); err != nil {
			t.Logf("Program error: %v", err)
		}
	}()

	// Wait for subscriptions to be ready
	select {
	case <-model.SubscriptionsReady():
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for subscriptions to be ready")
	}

	// Wait for initial participants to be fetched
	for i := 0; i < 50; i++ {
		participants := model.sidebar.GetParticipants()
		if len(participants) >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Register another participant (simulating someone joining)
	hubChans.RegisterParticipant(hub.NewParticipantEvent("Agent-user2", "Participant Two", hub.StatusIdle, 1, hub.ParticipantEventConnect))

	// Wait for participant event to be processed
	time.Sleep(500 * time.Millisecond)

	// Check sidebar has all three participants
	participants := model.sidebar.GetParticipants()
	assert.Len(t, participants, 3)
	
	names := make(map[string]bool)
	for _, p := range participants {
		names[p.Name] = true
	}
	assert.True(t, names["host"])
	assert.True(t, names["Agent-user1"])
	assert.True(t, names["Agent-user2"])
}