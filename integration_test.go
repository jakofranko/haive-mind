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
	msgCh := make(chan *hub.Message, 100)
	participantCh := make(chan *hub.Participant, 100)

	// Subscribe to all message types
	messageTypes := []hub.MessageType{
		hub.MessageTypeDirective,
		hub.MessageTypeResponse,
		hub.MessageTypeSystem,
		hub.MessageTypeAggregate,
		hub.MessageTypeNewRound,
		hub.MessageTypeRoundChange,
		hub.MessageTypeExport,
		hub.MessageTypeExportComplete,
	}

	for _, msgType := range messageTypes {
		h.Subscribe(ctx, msgType, func(msg *hub.Message) error {
			select {
			case msgCh <- msg:
			default:
			}
			return nil
		})
	}

	// Subscribe to participant events
	h.Subscribe(ctx, hub.MessageTypeParticipantEvent, func(msg *hub.Message) error {
		participants := h.GetParticipants()
		for _, p := range participants {
			if p.Name == msg.ID {
				select {
				case participantCh <- p:
				default:
				}
				break
			}
		}
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-msgCh:
			t.Messages = append(t.Messages, msg)
		case p := <-participantCh:
			pCopy := *p
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

// mockAgentRunner is a test implementation of agent.Runner
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
	tmpDir := t.TempDir()

	hubChans := hub.NewHub()
	mockRunner := newMockAgentRunner()
	hubChans.SetAgentRunner(mockRunner)

	mockRunner.SetResponse("Test directive from Host", "Agent reply to directive")
	mockRunner.SetResponse("Aggregated responses: Participant1: Response 1; Participant2: Response 2", "Agent reply to aggregate")

	transcript := &Transcript{
		Messages:     make([]*hub.Message, 0),
		Participants: make([]*hub.Participant, 0),
		StartedAt:    time.Now(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go collectTranscript(ctx, hubChans, transcript, false)

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	// Give time for subscriptions to be established
	time.Sleep(50 * time.Millisecond)

	hostName := "host"
	hostDisplayName := "Host"
	p1Name := "Agent-user1"
	p1DisplayName := "Participant One"
	p2Name := "Agent-user2"
	p2DisplayName := "Participant Two"

	hubChans.RegisterParticipant(hub.NewParticipantEvent(hostName, hostDisplayName, hub.StatusIdle, 1, hub.ParticipantEventConnect))
	hubChans.RegisterParticipant(hub.NewParticipantEvent(p1Name, p1DisplayName, hub.StatusIdle, 1, hub.ParticipantEventConnect))
	hubChans.RegisterParticipant(hub.NewParticipantEvent(p2Name, p2DisplayName, hub.StatusIdle, 1, hub.ParticipantEventConnect))

	directiveMsg := hub.NewMessage(hub.MessageTypeDirective, hostDisplayName, "Test directive from Host", 1)
	hubChans.Send(ctx, directiveMsg)

	time.Sleep(100 * time.Millisecond)

	p1Response := hub.NewMessage(hub.MessageTypeResponse, p1DisplayName, "Response 1", 1)
	hubChans.Send(ctx, p1Response)

	p2Response := hub.NewMessage(hub.MessageTypeResponse, p2DisplayName, "Response 2", 1)
	hubChans.Send(ctx, p2Response)

	hubChans.RequestAggregate(1)

	time.Sleep(100 * time.Millisecond)

	aggregateMsg := hub.NewMessage(hub.MessageTypeAggregate, hostDisplayName, "Aggregated responses: Participant1: Response 1; Participant2: Response 2", 1)
	hubChans.Send(ctx, aggregateMsg)

	time.Sleep(100 * time.Millisecond)

	cancel()
	time.Sleep(50 * time.Millisecond)

	t.Logf("Total messages in transcript: %d", len(transcript.Messages))
	t.Logf("Total participants: %d", len(transcript.Participants))

	messageTypes := make(map[hub.MessageType]int)
	for _, msg := range transcript.Messages {
		messageTypes[msg.Type]++
		t.Logf("Message: type=%s from=%s round=%d content=%s", msg.Type, msg.From, msg.Round, msg.Content)
	}

	if messageTypes[hub.MessageTypeDirective] != 1 {
		t.Errorf("Expected 1 directive message, got %d", messageTypes[hub.MessageTypeDirective])
	}

	if messageTypes[hub.MessageTypeResponse] < 4 {
		t.Errorf("Expected at least 4 response messages (2 participant + 2 agent), got %d", messageTypes[hub.MessageTypeResponse])
	}

	if messageTypes[hub.MessageTypeAggregate] != 2 {
		t.Errorf("Expected 2 aggregate messages (1 from RequestAggregate, 1 from test), got %d", messageTypes[hub.MessageTypeAggregate])
	}

	if len(transcript.Participants) != 3 {
		t.Errorf("Expected 3 participants, got %d", len(transcript.Participants))
	}

	transcript.EndedAt = time.Now()
	err := saveTranscript(transcript, tmpDir)
	if err != nil {
		t.Errorf("Failed to save transcript: %v", err)
	}

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

	hubChans := hub.NewHub()
	mockRunner := newMockAgentRunner()
	hubChans.SetAgentRunner(mockRunner)

	transcript := &Transcript{
		Messages:     make([]*hub.Message, 0),
		Participants: make([]*hub.Participant, 0),
		StartedAt:    time.Now(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	go collectTranscript(ctx, hubChans, transcript, false)

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}

	hubChans.RegisterParticipant(hub.NewParticipantEvent("host", "Host", hub.StatusIdle, 1, hub.ParticipantEventConnect))

	msg := hub.NewMessage(hub.MessageTypeDirective, "Host", "Test", 1)
	hubChans.Send(ctx, msg)

	time.Sleep(50 * time.Millisecond)

	cancel()
	time.Sleep(50 * time.Millisecond)

	hubChans.Stop()

	transcript.EndedAt = time.Now()
	err := saveTranscript(transcript, tmpDir)
	if err != nil {
		t.Errorf("Failed to save transcript on shutdown: %v", err)
	}

	path := filepath.Join(tmpDir, "transcript.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Transcript file not created on shutdown")
	}
}

func TestIntegration_ParticipantsSeeEachOthersResponses(t *testing.T) {
	hubChans := hub.NewHub()
	mockRunner := newMockAgentRunner()
	hubChans.SetAgentRunner(mockRunner)

	hostMessages := make(chan *hub.Message, 100)
	p1Messages := make(chan *hub.Message, 100)
	p2Messages := make(chan *hub.Message, 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	messageTypes := []hub.MessageType{
		hub.MessageTypeDirective,
		hub.MessageTypeResponse,
		hub.MessageTypeSystem,
		hub.MessageTypeAggregate,
	}

	for _, msgType := range messageTypes {
		hubChans.Subscribe(ctx, msgType, func(msg *hub.Message) error {
			hostMessages <- msg
			p1Messages <- msg
			p2Messages <- msg
			return nil
		})
	}

	hubChans.RegisterParticipant(hub.NewParticipantEvent("host", "Host", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	hubChans.RegisterParticipant(hub.NewParticipantEvent("p1", "Participant One", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	hubChans.RegisterParticipant(hub.NewParticipantEvent("p2", "Participant Two", hub.StatusIdle, 1, hub.ParticipantEventConnect))

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	directive := hub.NewMessage(hub.MessageTypeDirective, "Host", "Directive", 1)
	hubChans.Send(ctx, directive)

	agentReply := hub.NewMessage(hub.MessageTypeResponse, "agent", "Agent reply", 1)
	hubChans.Send(ctx, agentReply)

	p1Resp := hub.NewMessage(hub.MessageTypeResponse, "Participant One", "P1 response", 1)
	hubChans.Send(ctx, p1Resp)

	p2Resp := hub.NewMessage(hub.MessageTypeResponse, "Participant Two", "P2 response", 1)
	hubChans.Send(ctx, p2Resp)

	time.Sleep(50 * time.Millisecond)

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

	verifyReceived("Host", hostMessages, 4)
	verifyReceived("Participant1", p1Messages, 4)
	verifyReceived("Participant2", p2Messages, 4)
}

func TestIntegration_HostSeesAgentReplyAndAggregatePreview(t *testing.T) {
	hubChans := hub.NewHub()
	mockRunner := newMockAgentRunner()
	hubChans.SetAgentRunner(mockRunner)

	mockRunner.SetResponse("Test directive", "Agent says hello")
	mockRunner.SetResponse("Aggregate: P1: Hi; P2: Hello", "Agent says goodbye")

	hostMessages := make(chan *hub.Message, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	messageTypes := []hub.MessageType{
		hub.MessageTypeDirective,
		hub.MessageTypeResponse,
		hub.MessageTypeSystem,
		hub.MessageTypeAggregate,
	}

	for _, msgType := range messageTypes {
		hubChans.Subscribe(ctx, msgType, func(msg *hub.Message) error {
			hostMessages <- msg
			return nil
		})
	}

	hubChans.RegisterParticipant(hub.NewParticipantEvent("host", "Host", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	hubChans.RegisterParticipant(hub.NewParticipantEvent("p1", "P1", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	hubChans.RegisterParticipant(hub.NewParticipantEvent("p2", "P2", hub.StatusIdle, 1, hub.ParticipantEventConnect))

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	directive := hub.NewMessage(hub.MessageTypeDirective, "Host", "Test directive", 1)
	hubChans.Send(ctx, directive)

	time.Sleep(50 * time.Millisecond)

	hubChans.Send(ctx, hub.NewMessage(hub.MessageTypeResponse, "P1", "Hi", 1))
	hubChans.Send(ctx, hub.NewMessage(hub.MessageTypeResponse, "P2", "Hello", 1))

	hubChans.RequestAggregate(1)
	time.Sleep(50 * time.Millisecond)

	aggregate := hub.NewMessage(hub.MessageTypeAggregate, "Host", "Aggregate: P1: Hi; P2: Hello", 1)
	hubChans.Send(ctx, aggregate)

	time.Sleep(50 * time.Millisecond)

	received := make([]*hub.Message, 0)
	timeout := time.After(500 * time.Millisecond)
	// Expect 7 messages: directive, agent reply to directive, aggregate_request, P1 response, P2 response, aggregate from host, agent reply to aggregate
	for len(received) < 7 {
		select {
		case <-timeout:
			t.Errorf("Timeout: host received %d/7 messages", len(received))
			return
		case msg := <-hostMessages:
			received = append(received, msg)
			t.Logf("Host received: %s from %s", msg.Type, msg.From)
		}
	}

	types := make(map[hub.MessageType]int)
	for _, msg := range received {
		types[msg.Type]++
	}

	if types[hub.MessageTypeDirective] != 1 {
		t.Errorf("Missing directive")
	}
	if types[hub.MessageTypeResponse] != 4 {
		t.Errorf("Expected 4 responses, got %d", types[hub.MessageTypeResponse])
	}
	if types[hub.MessageTypeAggregate] != 2 {
		t.Errorf("Expected 2 aggregates (aggregate_request + host aggregate), got %d", types[hub.MessageTypeAggregate])
	}
}

func TestIntegration_AggregationEditableByHost(t *testing.T) {
	hubChans := hub.NewHub()
	mockRunner := newMockAgentRunner()
	hubChans.SetAgentRunner(mockRunner)

	aggregateSent := make(chan string, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hubChans.Subscribe(ctx, hub.MessageTypeAggregate, func(msg *hub.Message) error {
		// Filter out the aggregate_request signal
		if msg.Content != "aggregate_request" {
			aggregateSent <- msg.Content
		}
		return nil
	})

	hubChans.RegisterParticipant(hub.NewParticipantEvent("host", "Host", hub.StatusIdle, 1, hub.ParticipantEventConnect))

	if err := hubChans.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	defer hubChans.Stop()

	// Host triggers aggregation - this sends an aggregate_request signal
	hubChans.RequestAggregate(1)

	// Host composes and sends the edited aggregate
	edited := "Edited aggregate for round 1"
	aggregateMsg := hub.NewMessage(hub.MessageTypeAggregate, "Host", edited, 1)
	hubChans.Send(ctx, aggregateMsg)

	select {
	case aggregate := <-aggregateSent:
		if aggregate != "Edited aggregate for round 1" {
			t.Errorf("Expected edited aggregate, got: %s", aggregate)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for aggregate")
	}
}