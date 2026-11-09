package hub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"jakofranko/haive-mind/agent"
)

// ErrHubStopped is returned by Send once the hub has been stopped.
var ErrHubStopped = errors.New("hub stopped")

// handlerEntry is one subscriber. Messages for it queue in order and are
// delivered by a single drain goroutine that exists only while there is work,
// so a handler sees messages in send order and a slow handler cannot hold up
// the hub or other subscribers.
type handlerEntry struct {
	msgTypes []MessageType
	handler  func(*Message) error

	mu      sync.Mutex
	queue   []*Message
	running bool
	removed bool
}

func (e *handlerEntry) deliver(msg *Message) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.removed {
		return
	}
	e.queue = append(e.queue, msg)
	if !e.running {
		e.running = true
		go e.drain()
	}
}

func (e *handlerEntry) drain() {
	for {
		e.mu.Lock()
		if e.removed || len(e.queue) == 0 {
			e.queue = nil
			e.running = false
			e.mu.Unlock()
			return
		}
		msg := e.queue[0]
		e.queue = e.queue[1:]
		e.mu.Unlock()

		_ = e.handler(msg) // handler errors are not actionable here
	}
}

func (e *handlerEntry) remove() {
	e.mu.Lock()
	e.removed = true
	e.queue = nil
	e.mu.Unlock()
}

type subscription struct {
	hub   *hubImpl
	entry *handlerEntry
	once  sync.Once
	done  chan struct{}
}

// Unsubscribe removes the handler. It is safe to call more than once.
func (s *subscription) Unsubscribe() {
	s.once.Do(func() {
		close(s.done)
		s.hub.removeHandler(s.entry)
	})
}

type hubImpl struct {
	mu           sync.RWMutex
	handlers     map[MessageType][]*handlerEntry
	participants map[string]*Participant
	joined       map[string]int // join sequence per registered participant, for a stable order
	joinSeq      int
	displayNames map[string]string
	drafts       map[string]string
	agentRunner  agent.Runner
	synthesizer  agent.Runner
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	started      bool
	outbound     chan *Message

	// closeMu guards closed and the close of outbound: Send holds the read
	// lock so Stop cannot close the channel underneath an in-progress send.
	closeMu sync.RWMutex
	closed  bool
}

func NewHub() *hubImpl {
	ctx, cancel := context.WithCancel(context.Background())
	return &hubImpl{
		handlers:     make(map[MessageType][]*handlerEntry),
		participants: make(map[string]*Participant),
		joined:       make(map[string]int),
		displayNames: make(map[string]string),
		drafts:       make(map[string]string),
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
	return h.SubscribeTypes(ctx, []MessageType{msgType}, handler)
}

// SubscribeTypes subscribes one handler to several message types. Messages of
// all those types reach the handler in the order they were sent, which
// separate per-type subscriptions cannot guarantee.
func (h *hubImpl) SubscribeTypes(ctx context.Context, msgTypes []MessageType, handler func(*Message) error) (Subscription, error) {
	entry := &handlerEntry{msgTypes: append([]MessageType(nil), msgTypes...), handler: handler}

	h.mu.Lock()
	for _, t := range entry.msgTypes {
		h.handlers[t] = append(h.handlers[t], entry)
	}
	h.mu.Unlock()

	sub := &subscription{hub: h, entry: entry, done: make(chan struct{})}
	if ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				sub.Unsubscribe()
			case <-sub.done:
			}
		}()
	}
	return sub, nil
}

// removeHandler deletes entry, copying the slice so in-flight dispatches that
// already took a snapshot are unaffected.
func (h *hubImpl) removeHandler(entry *handlerEntry) {
	entry.remove()
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, t := range entry.msgTypes {
		old := h.handlers[t]
		kept := make([]*handlerEntry, 0, len(old))
		for _, e := range old {
			if e != entry {
				kept = append(kept, e)
			}
		}
		h.handlers[t] = kept
	}
}

func (h *hubImpl) RequestAggregate(round int) error {
	// Send an aggregate request event - the host/server will compose and send the actual aggregate
	msg := NewMessage(MessageTypeAggregateRequest, "hub", "", round)
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

	if _, known := h.joined[p.Name]; !known {
		h.joinSeq++
		h.joined[p.Name] = h.joinSeq
	}
	h.participants[p.Name] = p

	event := NewParticipantEvent(p.Name, p.DisplayName, p.Status, p.Round, ParticipantEventConnect)
	h.broadcastLocked(MessageTypeParticipantEvent, event)
	return nil
}

// UpdateParticipantStatus records a new status for a registered participant
// and announces the change. The stored entry is replaced, not mutated, so
// copies already handed out stay unchanged.
func (h *hubImpl) UpdateParticipantStatus(name string, status ParticipantStatus) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	p, ok := h.participants[name]
	if !ok {
		return fmt.Errorf("participant %q is not registered", name)
	}
	updated := *p
	updated.Status = status
	updated.Event = ParticipantEventStatusChange
	h.participants[name] = &updated

	h.broadcastLocked(MessageTypeParticipantEvent, &updated)
	return nil
}

func (h *hubImpl) UnregisterParticipant(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if p, ok := h.participants[name]; ok {
		delete(h.participants, name)
		delete(h.joined, name)

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

// SetDraft stores a participant's unsent text, keyed by their stable id so it
// outlives any one session. Empty text clears it.
func (h *hubImpl) SetDraft(name, text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if text == "" {
		delete(h.drafts, name)
		return
	}
	h.drafts[name] = text
}

func (h *hubImpl) GetDraft(name string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.drafts[name]
}

func (h *hubImpl) SetAgentRunner(runner agent.Runner) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agentRunner = runner
}

// SetSynthesizer sets the runner used for one-off aggregate synthesis. It is
// separate from the agent runner because synthesis needs no session context.
func (h *hubImpl) SetSynthesizer(runner agent.Runner) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.synthesizer = runner
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

	for _, e := range handlers {
		e.deliver(msg)
	}

	// Handle internal agent invocation for Directive and Aggregate
	if msg.Type == MessageTypeDirective || msg.Type == MessageTypeAggregate {
		h.invokeAgent(msg)
	}
	if msg.Type == MessageTypeSynthesisRequest {
		h.invokeSynthesizer(msg)
	}
}

// invokeSynthesizer runs the one-off synthesis and publishes the result as a
// draft, or a system message if it fails.
func (h *hubImpl) invokeSynthesizer(msg *Message) {
	h.mu.RLock()
	runner := h.synthesizer
	h.mu.RUnlock()

	h.runCall("synthesizer", runner, "Synthesis failed: ", msg, func(out string) {
		h.Send(h.ctx, NewMessage(MessageTypeAggregateDraft, "synthesizer", out, msg.Round))
	})
}

func (h *hubImpl) invokeAgent(msg *Message) {
	h.mu.RLock()
	runner := h.agentRunner
	h.mu.RUnlock()

	h.runCall("agent", runner, "Agent error: ", msg, func(out string) {
		h.Send(h.ctx, NewMessage(MessageTypeAgentReply, "agent", out, msg.Round))
	})
}

// runCall is the one path for calling a runner: it runs msg.Content off the
// dispatch loop, tracked so Stop waits for it, and on failure tells everyone
// (a system message) and the host (an agent error with a retry). On success it
// hands the output to publish. Sends after Stop fail with ErrHubStopped, which
// is why their results are not checked here. A missing runner skips the call.
func (h *hubImpl) runCall(name string, runner agent.Runner, failurePrefix string, msg *Message, publish func(out string)) {
	if runner == nil {
		slog.Warn("no runner set; skipping call", "call", name, "type", msg.Type)
		return
	}

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		out, err := runner.Run(h.ctx, msg.Content)
		if err != nil {
			text := failurePrefix + err.Error()
			h.Send(h.ctx, NewMessage(MessageTypeSystem, "hub", text, msg.Round))
			h.reportFailure(text, msg)
			return
		}
		publish(out)
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
	for _, e := range handlers {
		e.deliver(msg)
	}
}

func (h *hubImpl) GetParticipants() []*Participant {
	h.mu.RLock()
	defer h.mu.RUnlock()
	participants := make([]*Participant, 0, len(h.participants))
	for _, p := range h.participants {
		participants = append(participants, p)
	}
	sort.Slice(participants, func(i, j int) bool { return h.joined[participants[i].Name] < h.joined[participants[j].Name] })
	return participants
}

// reportFailure tells the host a call failed and how to try it again.
func (h *hubImpl) reportFailure(text string, failed *Message) {
	m := NewMessage(MessageTypeAgentError, "hub", text, failed.Round)
	m.Retry = &Retry{Type: failed.Type, Content: failed.Content}
	h.Send(h.ctx, m)
}
