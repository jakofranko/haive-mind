package hub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"jakofranko/haive-mind/agent"
)

// ErrHubStopped is returned by Send once the hub has been stopped.
var ErrHubStopped = errors.New("hub stopped")

type handlerEntry struct {
	msgType MessageType
	handler func(*Message) error
	cancel  context.CancelFunc
}

type subscription struct {
	entries []*handlerEntry
	mu      sync.Mutex
}

func (s *subscription) Unsubscribe() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.cancel != nil {
			e.cancel()
		}
	}
	s.entries = nil
}

type hubImpl struct {
	mu              sync.RWMutex
	handlers        map[MessageType][]func(*Message) error
	participants    map[string]*Participant
	displayNames    map[string]string
	agentRunner     agent.Runner
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	started         bool
	outbound        chan *Message

	// closeMu guards closed and the close of outbound: Send holds the read
	// lock so Stop cannot close the channel underneath an in-progress send.
	closeMu sync.RWMutex
	closed  bool
}

func NewHub() *hubImpl {
	ctx, cancel := context.WithCancel(context.Background())
	return &hubImpl{
		handlers:     make(map[MessageType][]func(*Message) error),
		participants: make(map[string]*Participant),
		displayNames: make(map[string]string),
		agentRunner:  agent.NewDefaultRunner(),
		ctx:          ctx,
		cancel:       cancel,
		outbound:     make(chan *Message, 100),
	}
}

func (h *hubImpl) Send(ctx context.Context, msg *Message) error {
	h.closeMu.RLock()
	defer h.closeMu.RUnlock()
	if h.closed {
		return ErrHubStopped
	}
	select {
	case h.outbound <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-h.ctx.Done():
		return h.ctx.Err()
	}
}

func (h *hubImpl) Subscribe(ctx context.Context, msgType MessageType, handler func(*Message) error) (Subscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.handlers[msgType] == nil {
		h.handlers[msgType] = make([]func(*Message) error, 0)
	}

	_, cancel := context.WithCancel(ctx)
	entry := &handlerEntry{
		msgType: msgType,
		handler: handler,
		cancel:  cancel,
	}

	h.handlers[msgType] = append(h.handlers[msgType], handler)

	sub := &subscription{entries: []*handlerEntry{entry}}
	return sub, nil
}

func (h *hubImpl) RequestAggregate(round int) error {
	// Send an aggregate request event - the host/server will compose and send the actual aggregate
	msg := NewMessage(MessageTypeAggregate, "hub", "aggregate_request", round)
	return h.Send(h.ctx, msg)
}

func (h *hubImpl) StartRound(round int) error {
	msg := NewMessage(MessageTypeNewRound, "hub", "", round)
	return h.Send(h.ctx, msg)
}

func (h *hubImpl) Export(path string) error {
	msg := NewMessage(MessageTypeExport, "hub", path, 0)
	return h.Send(h.ctx, msg)
}

func (h *hubImpl) RegisterParticipant(p *Participant) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.participants[p.Name] = p

	event := NewParticipantEvent(p.Name, p.DisplayName, p.Status, p.Round, ParticipantEventConnect)
	h.broadcastLocked(MessageTypeParticipantEvent, event)
	return nil
}

func (h *hubImpl) UnregisterParticipant(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if p, ok := h.participants[name]; ok {
		delete(h.participants, name)

		event := NewParticipantEvent(p.Name, p.DisplayName, StatusDisconnected, p.Round, ParticipantEventDisconnect)
		h.broadcastLocked(MessageTypeParticipantEvent, event)
	}
	return nil
}

func (h *hubImpl) GetDisplayName(name string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if dn, ok := h.displayNames[name]; ok {
		return dn
	}
	return ""
}

func (h *hubImpl) SetDisplayName(name, displayName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.displayNames[name] = displayName
}

func (h *hubImpl) SetAgentRunner(runner agent.Runner) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agentRunner = runner
}

func (h *hubImpl) SendToAgent(ctx context.Context, prompt string) (string, error) {
	h.mu.RLock()
	runner := h.agentRunner
	h.mu.RUnlock()

	if runner == nil {
		return "", fmt.Errorf("no agent runner configured")
	}

	return runner.Run(ctx, prompt)
}

func (h *hubImpl) Start() error {
	h.mu.Lock()
	if h.started {
		h.mu.Unlock()
		return nil
	}
	h.started = true
	h.mu.Unlock()

	h.wg.Add(1)
	go h.dispatchLoop()
	return nil
}

func (h *hubImpl) Stop() error {
	h.cancel()
	h.wg.Wait()
	h.closeMu.Lock()
	defer h.closeMu.Unlock()
	if !h.closed {
		h.closed = true
		close(h.outbound)
	}
	return nil
}

func (h *hubImpl) Outbound() <-chan *Message {
	return h.outbound
}

func (h *hubImpl) dispatchLoop() {
	defer h.wg.Done()

	for {
		select {
		case <-h.ctx.Done():
			return
		case msg := <-h.outbound:
			h.dispatch(msg)
		}
	}
}

func (h *hubImpl) dispatch(msg *Message) {
	h.mu.RLock()
	handlers := h.handlers[msg.Type]
	h.mu.RUnlock()

	for _, handler := range handlers {
		go func(h func(*Message) error) {
			if err := h(msg); err != nil {
				// Log error but continue
			}
		}(handler)
	}

	// Handle internal agent invocation for Directive and Aggregate
	if msg.Type == MessageTypeDirective || msg.Type == MessageTypeAggregate {
		h.invokeAgent(msg)
	}
}

func (h *hubImpl) invokeAgent(msg *Message) {
	h.mu.RLock()
	runner := h.agentRunner
	h.mu.RUnlock()

	if runner == nil {
		return
	}

	// Skip agent invocation for aggregate_request signals
	if msg.Type == MessageTypeAggregate && msg.Content == "aggregate_request" {
		return
	}

	go func() {
		response, err := runner.Run(h.ctx, msg.Content)
		if err != nil {
			errMsg := NewMessage(MessageTypeSystem, "hub", "Agent error: "+err.Error(), msg.Round)
			h.Send(h.ctx, errMsg)
			return
		}

		reply := NewMessage(MessageTypeResponse, "agent", response, msg.Round)
		h.Send(h.ctx, reply)
	}()
}

func (h *hubImpl) broadcastLocked(msgType MessageType, payload interface{}) {
	var msg *Message
	switch v := payload.(type) {
	case *Message:
		msg = v
	case *Participant:
		msg = &Message{
			ID:        v.Name,
			Type:      msgType,
			From:      "hub",
			Content:   v.Event,
			Timestamp: time.Now(),
			Round:     v.Round,
		}
	default:
		return
	}

	handlers := h.handlers[msgType]
	for _, handler := range handlers {
		go func(h func(*Message) error) {
			if err := h(msg); err != nil {
			}
		}(handler)
	}
}

func (h *hubImpl) GetParticipants() []*Participant {
	h.mu.RLock()
	defer h.mu.RUnlock()
	participants := make([]*Participant, 0, len(h.participants))
	for _, p := range h.participants {
		participants = append(participants, p)
	}
	return participants
}