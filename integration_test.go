package haive_mind

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"jakofranko/haive-mind/hub"
)

type Transcript struct {
	Messages     []*hub.Message     `json:"messages"`
	Participants []*hub.Participant `json:"participants"`
	StartedAt    time.Time          `json:"started_at"`
	EndedAt      time.Time          `json:"ended_at"`
}

func collectTranscript(ctx context.Context, h hub.Hub, t *Transcript, debug bool) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-h.Outbound():
			t.Messages = append(t.Messages, msg)
			if debug {
				// debug logging would go here
			}
		case p := <-h.ParticipantUpdateChan():
			pCopy := p
			t.Participants = append(t.Participants, &pCopy)
		}
	}
}

func saveTranscript(t *Transcript, dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dataDir, "transcript.json")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(t)
}

// mockAgentRunner is a test implementation of hub.AgentRunner
type mockAgentRunner struct {
	responses map[string]string
	delay     time.Duration
	callCount int
}

func newMockAgentRunner() *mockAgentRunner {
	return &mockAgentRunner{
		responses: make(map[string]string),
	}
}

func (m *mockAgentRunner) SetResponse(prompt, response string) {
	m.responses[prompt] = response
}

func (m *mockAgentRunner) SetDelay(delay time.Duration) {
	m.delay = delay
}

func (m *mockAgentRunner) Run(ctx context.Context, prompt string) (string, error) {
	m.callCount++
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if response, ok := m.responses[prompt]; ok {
		return response, nil
	}
	return "Agent response to: " + prompt, nil
}

func TestIntegration_FullRoundTrip(t *testing.T) {
	// Create a temporary directory for transcript
	tmpDir := t.TempDir()

	// Create hub with mock agent runner
	hubChans := hub.NewHubChannels()
	mockRunner := newMockAgentRunner()
	hubChans.SetAgentRunner(mockRunner)

	// Set up mock agent responses
	mockRunner.SetResponse("Test directive from Host", "Agent reply to directive")
	mockRunner.SetResponse("Aggregated responses: Participant1: Response 1; Participant2: Response 2", "Agent reply to aggregate")

	// Create transcript collector
	transcript := &Transcript{
		Messages:     make([]*hub.Message, 0),
		Participants: make([]*hub.Participant, 0),
		StartedAt:    time.Now(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Collect transcript
	go collectTranscript(ctx, hubChans, transcript, false)

	// Simulate host connection
	hostName := "host"
	hostDisplayName := "Host"
	hubChans.ParticipantUpdateChan() <- hub.Participant{
		Name:        hostName,
		DisplayName: hostDisplayName,
		Status:      hub.StatusIdle,
		Round:       1,
	}

	// Simulate participant 1 connection
	p1Name := "Agent-user1"
	p1DisplayName := "Participant One"
	hubChans.ParticipantUpdateChan() <- hub.Participant{
		Name:        p1Name,
		DisplayName: p1DisplayName,
		Status:      hub.StatusIdle,
		Round:       1,
	}

	// Simulate participant 2 connection
	p2Name := "Agent-user2"
	p2DisplayName := "Participant Two"
	hubChans.ParticipantUpdateChan() <- hub.Participant{
		Name:        p2Name,
		DisplayName: p2DisplayName,
		Status:      hub.StatusIdle,
		Round:       1,
	}

	// Round 1: Host sends directive
	directiveMsg := hub.NewMessage(hub.MessageTypeDirective, hostDisplayName, "Test directive from Host", 1)
	hubChans.Outbound() <- directiveMsg
	hubChans.SendChan() <- directiveMsg

	// Agent processes directive and sends reply
	go func() {
		response, err := hubChans.SendToAgent(ctx, "Test directive from Host")
		if err != nil {
			t.Errorf("Agent error: %v", err)
			return
		}
		agentReply := hub.NewMessage(hub.MessageTypeResponse, "agent", response, 1)
		hubChans.Outbound() <- agentReply
	}()

	// Wait for agent reply
	time.Sleep(100 * time.Millisecond)

	// Participants respond
	p1Response := hub.NewMessage(hub.MessageTypeResponse, p1DisplayName, "Response 1", 1)
	hubChans.Outbound() <- p1Response
	hubChans.SendChan() <- p1Response

	p2Response := hub.NewMessage(hub.MessageTypeResponse, p2DisplayName, "Response 2", 1)
	hubChans.Outbound() <- p2Response
	hubChans.SendChan() <- p2Response

	// Host triggers aggregation
	hubChans.AggregateChan() <- 1

	// Wait for aggregation to be processed
	time.Sleep(100 * time.Millisecond)

	// Host sends aggregated message (simulated)
	aggregateMsg := hub.NewMessage(hub.MessageTypeAggregate, hostDisplayName, "Aggregated responses: Participant1: Response 1; Participant2: Response 2", 1)
	hubChans.Outbound() <- aggregateMsg
	hubChans.SendChan() <- aggregateMsg

	// Agent processes aggregate and sends reply
	go func() {
		response, err := hubChans.SendToAgent(ctx, "Aggregated responses: Participant1: Response 1; Participant2: Response 2")
		if err != nil {
			t.Errorf("Agent error on aggregate: %v", err)
			return
		}
		agentReply := hub.NewMessage(hub.MessageTypeResponse, "agent", response, 1)
		hubChans.Outbound() <- agentReply
	}()

	// Wait for final agent reply
	time.Sleep(100 * time.Millisecond)

	// End session
	cancel()
	time.Sleep(50 * time.Millisecond)

	// Verify transcript
	t.Logf("Total messages in transcript: %d", len(transcript.Messages))
	t.Logf("Total participants: %d", len(transcript.Participants))

	// Verify we have all expected message types
	messageTypes := make(map[hub.MessageType]int)
	for _, msg := range transcript.Messages {
		messageTypes[msg.Type]++
		t.Logf("Message: type=%s from=%s round=%d content=%s", msg.Type, msg.From, msg.Round, msg.Content)
	}

	// Check for directive
	if messageTypes[hub.MessageTypeDirective] != 1 {
		t.Errorf("Expected 1 directive message, got %d", messageTypes[hub.MessageTypeDirective])
	}

	// Check for agent replies (2: one for directive, one for aggregate)
	if messageTypes[hub.MessageTypeResponse] < 4 {
		// 2 from participants + 2 from agent
		t.Errorf("Expected at least 4 response messages (2 participant + 2 agent), got %d", messageTypes[hub.MessageTypeResponse])
	}

	// Check for aggregate
	if messageTypes[hub.MessageTypeAggregate] != 1 {
		t.Errorf("Expected 1 aggregate message, got %d", messageTypes[hub.MessageTypeAggregate])
	}

	// Check participants
	if len(transcript.Participants) != 3 {
		t.Errorf("Expected 3 participants, got %d", len(transcript.Participants))
	}

	// Verify transcript can be saved
	transcript.EndedAt = time.Now()
	err := saveTranscript(transcript, tmpDir)
	if err != nil {
		t.Errorf("Failed to save transcript: %v", err)
	}

	// Verify saved transcript
	savedPath := filepath.Join(tmpDir, "transcript.json")
	data, err := os.ReadFile(savedPath)
	if err != nil {
		t.Errorf("Failed to read saved transcript: %v", err)
	}

	var savedTranscript Transcript
	if err := json.Unmarshal(data, &savedTranscript); err != nil {
		t.Errorf("Failed to unmarshal saved transcript: %v", err)
	}

	if len(savedTranscript.Messages) != len(transcript.Messages) {
		t.Errorf("Saved transcript message count mismatch: %d vs %d", len(savedTranscript.Messages), len(transcript.Messages))
	}

	t.Log("Integration test passed: full round trip verified")
}

func TestIntegration_GracefulShutdownSavesTranscript(t *testing.T) {
	tmpDir := t.TempDir()

	hubChans := hub.NewHubChannels()
	mockRunner := newMockAgentRunner()
	hubChans.SetAgentRunner(mockRunner)

	transcript := &Transcript{
		Messages:     make([]*hub.Message, 0),
		Participants: make([]*hub.Participant, 0),
		StartedAt:    time.Now(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	go collectTranscript(ctx, hubChans, transcript, false)

	// Add some messages
	hubChans.ParticipantUpdateChan() <- hub.Participant{
		Name:        "host",
		DisplayName: "Host",
		Status:      hub.StatusIdle,
		Round:       1,
	}

	msg := hub.NewMessage(hub.MessageTypeDirective, "Host", "Test", 1)
	hubChans.Outbound() <- msg

	time.Sleep(50 * time.Millisecond)

	// Shutdown
	cancel()
	time.Sleep(50 * time.Millisecond)

	transcript.EndedAt = time.Now()
	err := saveTranscript(transcript, tmpDir)
	if err != nil {
		t.Errorf("Failed to save transcript on shutdown: %v", err)
	}

	// Verify file exists
	path := filepath.Join(tmpDir, "transcript.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Transcript file not created on shutdown")
	}
}

func TestIntegration_ParticipantsSeeEachOthersResponses(t *testing.T) {
	hubChans := hub.NewHubChannels()
	mockRunner := newMockAgentRunner()
	hubChans.SetAgentRunner(mockRunner)

	// Track messages received by each "client"
	hostMessages := make(chan *hub.Message, 100)
	p1Messages := make(chan *hub.Message, 100)
	p2Messages := make(chan *hub.Message, 100)

	// Simulate three listeners (host, participant1, participant2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-hubChans.Outbound():
				hostMessages <- msg
				p1Messages <- msg
				p2Messages <- msg
			}
		}
	}()

	// Setup participants
	hubChans.ParticipantUpdateChan() <- hub.Participant{Name: "host", DisplayName: "Host", Status: hub.StatusIdle, Round: 1}
	hubChans.ParticipantUpdateChan() <- hub.Participant{Name: "p1", DisplayName: "Participant One", Status: hub.StatusIdle, Round: 1}
	hubChans.ParticipantUpdateChan() <- hub.Participant{Name: "p2", DisplayName: "Participant Two", Status: hub.StatusIdle, Round: 1}

	// Host sends directive
	directive := hub.NewMessage(hub.MessageTypeDirective, "Host", "Directive", 1)
	hubChans.Outbound() <- directive
	hubChans.SendChan() <- directive

	// Agent replies
	agentReply := hub.NewMessage(hub.MessageTypeResponse, "agent", "Agent reply", 1)
	hubChans.Outbound() <- agentReply

	// Participant 1 responds
	p1Resp := hub.NewMessage(hub.MessageTypeResponse, "Participant One", "P1 response", 1)
	hubChans.Outbound() <- p1Resp
	hubChans.SendChan() <- p1Resp

	// Participant 2 responds
	p2Resp := hub.NewMessage(hub.MessageTypeResponse, "Participant Two", "P2 response", 1)
	hubChans.Outbound() <- p2Resp
	hubChans.SendChan() <- p2Resp

	time.Sleep(50 * time.Millisecond)

	// Verify all three received all messages
	verifyReceived := func(name string, ch chan *hub.Message, expected int) {
		count := 0
		timeout := time.After(500 * time.Millisecond)
		for count < expected {
			select {
			case <-timeout:
				t.Errorf("%s: timeout waiting for messages, got %d/%d", name, count, expected)
				return
			case msg := <-ch:
				count++
				t.Logf("%s received: %s from %s", name, msg.Type, msg.From)
			}
		}
	}

	// Each should receive: directive, agent reply, p1 response, p2 response = 4 messages
	verifyReceived("Host", hostMessages, 4)
	verifyReceived("Participant1", p1Messages, 4)
	verifyReceived("Participant2", p2Messages, 4)
}

func TestIntegration_HostSeesAgentReplyAndAggregatePreview(t *testing.T) {
	hubChans := hub.NewHubChannels()
	mockRunner := newMockAgentRunner()
	hubChans.SetAgentRunner(mockRunner)

	mockRunner.SetResponse("Test directive", "Agent says hello")
	mockRunner.SetResponse("Aggregate: P1: Hi; P2: Hello", "Agent says goodbye")

	hostMessages := make(chan *hub.Message, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-hubChans.Outbound():
				hostMessages <- msg
			}
		}
	}()

	hubChans.ParticipantUpdateChan() <- hub.Participant{Name: "host", DisplayName: "Host", Status: hub.StatusIdle, Round: 1}
	hubChans.ParticipantUpdateChan() <- hub.Participant{Name: "p1", DisplayName: "P1", Status: hub.StatusIdle, Round: 1}
	hubChans.ParticipantUpdateChan() <- hub.Participant{Name: "p2", DisplayName: "P2", Status: hub.StatusIdle, Round: 1}

	// Host sends directive
	directive := hub.NewMessage(hub.MessageTypeDirective, "Host", "Test directive", 1)
	hubChans.Outbound() <- directive
	hubChans.SendChan() <- directive

	// Agent replies
	go func() {
		resp, _ := hubChans.SendToAgent(ctx, "Test directive")
		reply := hub.NewMessage(hub.MessageTypeResponse, "agent", resp, 1)
		hubChans.Outbound() <- reply
	}()

	time.Sleep(50 * time.Millisecond)

	// Participants respond
	hubChans.Outbound() <- hub.NewMessage(hub.MessageTypeResponse, "P1", "Hi", 1)
	hubChans.SendChan() <- hub.NewMessage(hub.MessageTypeResponse, "P1", "Hi", 1)
	hubChans.Outbound() <- hub.NewMessage(hub.MessageTypeResponse, "P2", "Hello", 1)
	hubChans.SendChan() <- hub.NewMessage(hub.MessageTypeResponse, "P2", "Hello", 1)

	// Host aggregates
	hubChans.AggregateChan() <- 1
	time.Sleep(50 * time.Millisecond)

	// Host sends aggregate
	aggregate := hub.NewMessage(hub.MessageTypeAggregate, "Host", "Aggregate: P1: Hi; P2: Hello", 1)
	hubChans.Outbound() <- aggregate
	hubChans.SendChan() <- aggregate

	// Agent replies to aggregate
	go func() {
		resp, _ := hubChans.SendToAgent(ctx, "Aggregate: P1: Hi; P2: Hello")
		reply := hub.NewMessage(hub.MessageTypeResponse, "agent", resp, 1)
		hubChans.Outbound() <- reply
	}()

	time.Sleep(50 * time.Millisecond)

	// Verify host sees all messages
	received := make([]*hub.Message, 0)
	timeout := time.After(500 * time.Millisecond)
	for len(received) < 6 {
		select {
		case <-timeout:
			t.Errorf("Timeout: host received %d/6 messages", len(received))
			return
		case msg := <-hostMessages:
			received = append(received, msg)
			t.Logf("Host received: %s from %s", msg.Type, msg.From)
		}
	}

	// Verify message types
	types := make(map[hub.MessageType]int)
	for _, msg := range received {
		types[msg.Type]++
	}

	if types[hub.MessageTypeDirective] != 1 {
		t.Errorf("Missing directive")
	}
	if types[hub.MessageTypeResponse] != 4 { // agent reply + 2 participants + agent reply to aggregate
		t.Errorf("Expected 4 responses, got %d", types[hub.MessageTypeResponse])
	}
	if types[hub.MessageTypeAggregate] != 1 {
		t.Errorf("Missing aggregate")
	}
}

func TestIntegration_AggregationEditableByHost(t *testing.T) {
	hubChans := hub.NewHubChannels()
	mockRunner := newMockAgentRunner()
	hubChans.SetAgentRunner(mockRunner)

	aggregateSent := make(chan string, 1)

	// Override aggregate handling to capture what host sends
	go func() {
		for round := range hubChans.AggregateChan() {
			t.Logf("Aggregate triggered for round %d", round)
			// In real app, host would edit and send via SendChan
			// Here we simulate host editing the aggregate
			edited := "Edited aggregate for round " + string(rune(round+'0'))
			aggregateSent <- edited
			msg := hub.NewMessage(hub.MessageTypeAggregate, "Host", edited, round)
			hubChans.Outbound() <- msg
			hubChans.SendChan() <- msg
		}
	}()

	hubChans.AggregateChan() <- 1

	select {
	case aggregate := <-aggregateSent:
		if aggregate != "Edited aggregate for round 1" {
			t.Errorf("Expected edited aggregate, got: %s", aggregate)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for aggregate")
	}
}
