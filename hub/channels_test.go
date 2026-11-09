package hub

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/agent"
)

type mockAgentRunner struct {
	response string
	err      error
	delay    time.Duration
}

func (m *mockAgentRunner) Run(ctx context.Context, prompt string) (string, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return m.response, m.err
}

func TestHub_Send_Success(t *testing.T) {
	hubChans := NewHub()
	hubChans.SetAgentRunner(&mockAgentRunner{response: "Agent response"})

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	ctx := context.Background()
	msg := NewMessage(MessageTypeDirective, "Host", "test prompt", 1)

	err := hubChans.Send(ctx, msg)
	assert.NoError(t, err)
}

func TestHub_SubscribeAndSend(t *testing.T) {
	hubChans := NewHub()

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	ctx := context.Background()
	messagesReceived := make(chan *Message, 10)

	sub, err := hubChans.Subscribe(ctx, MessageTypeDirective, func(msg *Message) error {
		messagesReceived <- msg
		return nil
	})
	assert.NoError(t, err)
	defer sub.Unsubscribe()

	msg := NewMessage(MessageTypeDirective, "Host", "test prompt", 1)
	err = hubChans.Send(ctx, msg)
	assert.NoError(t, err)

	select {
	case received := <-messagesReceived:
		assert.Equal(t, "test prompt", received.Content)
		assert.Equal(t, MessageTypeDirective, received.Type)
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for message")
	}
}

func TestHub_RequestAggregate(t *testing.T) {
	hubChans := NewHub()

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	ctx := context.Background()
	aggregatesReceived := make(chan *Message, 10)

	sub, err := hubChans.Subscribe(ctx, MessageTypeAggregateRequest, func(msg *Message) error {
		aggregatesReceived <- msg
		return nil
	})
	assert.NoError(t, err)
	defer sub.Unsubscribe()

	err = hubChans.RequestAggregate(1)
	assert.NoError(t, err)

	select {
	case received := <-aggregatesReceived:
		assert.Equal(t, 1, received.Round)
		assert.Equal(t, MessageTypeAggregateRequest, received.Type)
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for aggregate")
	}
}

func TestHub_StartRound(t *testing.T) {
	hubChans := NewHub()

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	ctx := context.Background()
	roundsReceived := make(chan *Message, 10)

	sub, err := hubChans.Subscribe(ctx, MessageTypeNewRound, func(msg *Message) error {
		roundsReceived <- msg
		return nil
	})
	assert.NoError(t, err)
	defer sub.Unsubscribe()

	err = hubChans.StartRound(1)
	assert.NoError(t, err)

	select {
	case received := <-roundsReceived:
		assert.Equal(t, 1, received.Round) // the round being ended
		assert.Equal(t, MessageTypeNewRound, received.Type)
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for new round")
	}
}

func TestHub_Export(t *testing.T) {
	hubChans := NewHub()

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	ctx := context.Background()
	exportsReceived := make(chan *Message, 10)

	sub, err := hubChans.Subscribe(ctx, MessageTypeExport, func(msg *Message) error {
		exportsReceived <- msg
		return nil
	})
	assert.NoError(t, err)
	defer sub.Unsubscribe()

	err = hubChans.Export("test.json")
	assert.NoError(t, err)

	select {
	case received := <-exportsReceived:
		assert.Equal(t, "test.json", received.Content)
		assert.Equal(t, MessageTypeExport, received.Type)
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for export")
	}
}

func TestHub_RegisterUnregisterParticipant(t *testing.T) {
	hubChans := NewHub()

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	ctx := context.Background()
	participantEvents := make(chan *Message, 10)

	sub, err := hubChans.Subscribe(ctx, MessageTypeParticipantEvent, func(msg *Message) error {
		participantEvents <- msg
		return nil
	})
	assert.NoError(t, err)
	defer sub.Unsubscribe()

	// Register
	p := NewParticipantEvent("host", "Host", StatusIdle, 1, ParticipantEventConnect)
	err = hubChans.RegisterParticipant(p)
	assert.NoError(t, err)

	participants := hubChans.GetParticipants()
	assert.Len(t, participants, 1)

	// Wait for event
	select {
	case event := <-participantEvents:
		assert.Equal(t, "host", event.ID)
		assert.Equal(t, ParticipantEventConnect, event.Content)
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for participant event")
	}

	// Unregister
	err = hubChans.UnregisterParticipant("host")
	assert.NoError(t, err)

	participants = hubChans.GetParticipants()
	assert.Len(t, participants, 0)

	// Wait for event
	select {
	case event := <-participantEvents:
		assert.Equal(t, "host", event.ID)
		assert.Equal(t, ParticipantEventDisconnect, event.Content)
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for participant disconnect event")
	}
}

func TestHub_SetDisplayName(t *testing.T) {
	hubChans := NewHub()

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	hubChans.SetDisplayName("host", "TestHost")
	assert.Equal(t, "TestHost", hubChans.GetDisplayName("host"))
	assert.Equal(t, "", hubChans.GetDisplayName("unknown"))
}

func TestHub_SetAgentRunner(t *testing.T) {
	hubChans := NewHub()

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	ctx := context.Background()
	mockRunner := &mockAgentRunner{response: "Custom agent response"}
	hubChans.SetAgentRunner(mockRunner)

	agentReplies := make(chan *Message, 10)
	sub, err := hubChans.Subscribe(ctx, MessageTypeAgentReply, func(msg *Message) error {
		agentReplies <- msg
		return nil
	})
	assert.NoError(t, err)
	defer sub.Unsubscribe()

	// Send a directive - hub should invoke agent internally
	msg := NewMessage(MessageTypeDirective, "Host", "test prompt", 1)
	err = hubChans.Send(ctx, msg)
	assert.NoError(t, err)

	select {
	case reply := <-agentReplies:
		assert.Equal(t, "Custom agent response", reply.Content)
		assert.Equal(t, "agent", reply.From)
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for agent reply")
	}
}

func TestAgentError_Code(t *testing.T) {
	err := agent.NewAgentError("test_code", "test message")
	assert.Equal(t, "test_code", err.Code())
	assert.Equal(t, "test message", err.Error())
}

func TestErrAgentTimeout(t *testing.T) {
	assert.Equal(t, "agent timed out", agent.ErrAgentTimeout.Error())
	assert.Equal(t, "timeout", agent.ErrAgentTimeout.Code())
}
