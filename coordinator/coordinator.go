// Package coordinator holds the session-level state that sits beside the hub:
// the transcript, the current round, the participant responses collected
// for each round, and which participants the host has flagged for revision.
// It reacts to hub messages; it never invokes the agent (the hub owns that).
package coordinator

import (
	"context"
	"sort"
	"strings"
	"sync"

	"jakofranko/haive-mind/hub"
)

type Coordinator struct {
	hub hub.Hub

	mu         sync.Mutex
	transcript []*hub.Message
	round      int
	responses  map[int][]*hub.Message
	directives map[int]string
	flagged    map[string]bool // participant ids the host asked to revise
}

func New(h hub.Hub) *Coordinator {
	return &Coordinator{
		hub:        h,
		round:      1,
		responses:  make(map[int][]*hub.Message),
		directives: make(map[int]string),
		flagged:    make(map[string]bool),
	}
}

// transcriptTypes are the message types recorded in the transcript.
var transcriptTypes = []hub.MessageType{
	hub.MessageTypeDirective,
	hub.MessageTypeResponse,
	hub.MessageTypeAgentReply,
	hub.MessageTypeSystem,
	hub.MessageTypeAggregate,
	hub.MessageTypeAggregateDraft,
	hub.MessageTypeNewRound,
	hub.MessageTypeRoundChange,
	hub.MessageTypeExport,
	hub.MessageTypeExportComplete,
}

// Start subscribes the coordinator to the hub.
func (c *Coordinator) Start(ctx context.Context) error {
	// One subscription for every recorded type, so the transcript keeps the
	// order messages were sent in.
	if _, err := c.hub.SubscribeTypes(ctx, transcriptTypes, c.record); err != nil {
		return err
	}
	for _, s := range []struct {
		typ     hub.MessageType
		handler func(*hub.Message) error
	}{
		{hub.MessageTypeResponse, c.trackResponse},
		{hub.MessageTypeDirective, c.trackDirective},
		{hub.MessageTypeAggregateRequest, c.handleAggregateRequest},
		{hub.MessageTypeNewRound, c.handleNewRound},
		{hub.MessageTypeFlag, c.handleFlag},
		{hub.MessageTypeUnflag, c.handleUnflag},
		{hub.MessageTypeResolve, c.handleResolve},
		{hub.MessageTypeParticipantEvent, c.handleParticipantEvent},
	} {
		if _, err := c.hub.Subscribe(ctx, s.typ, s.handler); err != nil {
			return err
		}
	}
	return nil
}

// handleNewRound advances the round and announces the new number. The request
// names the round being ended; one naming any other round is stale or a repeat
// and is ignored. Flags belong to a round, so they end with it.
func (c *Coordinator) handleNewRound(msg *hub.Message) error {
	c.mu.Lock()
	if msg.Round != c.round {
		c.mu.Unlock()
		return nil
	}
	c.round++
	round := c.round
	c.responses[round] = nil
	c.flagged = make(map[string]bool)
	c.mu.Unlock()
	c.publishRoundState(round)
	return c.hub.Send(context.Background(), hub.NewMessage(hub.MessageTypeRoundChange, "hub", "", round))
}

// Round returns the current round number.
func (c *Coordinator) Round() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.round
}

// Flagged returns the ids of participants the host has asked to revise, sorted.
func (c *Coordinator) Flagged() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flaggedLocked()
}

func (c *Coordinator) flaggedLocked() []string {
	ids := make([]string, 0, len(c.flagged))
	for id := range c.flagged {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// handleFlag flags the participant named in To.
func (c *Coordinator) handleFlag(msg *hub.Message) error {
	c.mu.Lock()
	changed := false
	if msg.To != "" && !c.flagged[msg.To] {
		c.flagged[msg.To] = true
		changed = true
	}
	round := c.round
	c.mu.Unlock()
	if changed {
		c.publishRoundState(round)
	}
	return nil
}

// handleUnflag withdraws the flag on the participant named in To.
func (c *Coordinator) handleUnflag(msg *hub.Message) error {
	c.mu.Lock()
	changed := false
	if c.flagged[msg.To] {
		delete(c.flagged, msg.To)
		changed = true
	}
	round := c.round
	c.mu.Unlock()
	if changed {
		c.publishRoundState(round)
	}
	return nil
}

// handleResolve clears every flag.
func (c *Coordinator) handleResolve(msg *hub.Message) error {
	c.mu.Lock()
	had := len(c.flagged) > 0
	c.flagged = make(map[string]bool)
	round := c.round
	c.mu.Unlock()
	if had {
		c.publishRoundState(round)
	}
	return nil
}

// handleParticipantEvent announces the Round state when someone connects, so a
// session that joins or reconnects mid-session shows where things stand. The
// state is broadcast (ADR 0001) and idempotent, so everyone receiving it again
// is harmless; it never changes anyone's status.
func (c *Coordinator) handleParticipantEvent(msg *hub.Message) error {
	if msg.Content != hub.ParticipantEventConnect {
		return nil
	}
	c.mu.Lock()
	round := c.round
	c.mu.Unlock()
	c.publishRoundState(round)
	return nil
}

// publishRoundState tells everyone the current round and who is flagged.
func (c *Coordinator) publishRoundState(round int) {
	c.mu.Lock()
	ids := c.flaggedLocked()
	c.mu.Unlock()
	c.hub.Send(context.Background(), hub.NewRoundState(round, ids))
}

// trackDirective remembers the question the host asked in each round.
func (c *Coordinator) trackDirective(msg *hub.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.directives[msg.Round] = msg.Content
	return nil
}

// senderID identifies who sent a response: the stable id when there is one.
func senderID(msg *hub.Message) string {
	if msg.FromID != "" {
		return msg.FromID
	}
	return msg.From
}

// trackResponse remembers each participant's latest response by
// round, and lifts that participant's revision flag.
func (c *Coordinator) trackResponse(msg *hub.Message) error {
	id := senderID(msg)
	c.mu.Lock()
	replaced := false
	for i, r := range c.responses[msg.Round] {
		if senderID(r) == id {
			c.responses[msg.Round][i] = msg
			replaced = true
			break
		}
	}
	if !replaced {
		c.responses[msg.Round] = append(c.responses[msg.Round], msg)
	}
	wasFlagged := c.flagged[id]
	delete(c.flagged, id)
	round := c.round
	c.mu.Unlock()
	if wasFlagged {
		c.publishRoundState(round)
	}
	return nil
}

// handleAggregateRequest asks the hub to synthesize this round's responses.
// The host's confirmed Aggregate is a different message type, so it never
// lands here: that is what stops the loop.
func (c *Coordinator) handleAggregateRequest(msg *hub.Message) error {
	c.mu.Lock()
	round := c.round
	question := c.directives[round]
	var lines []string
	for _, r := range c.responses[round] {
		lines = append(lines, "- "+r.From+": "+r.Content)
	}
	c.mu.Unlock()

	if len(lines) == 0 {
		return nil
	}
	prompt := synthesisPrompt(question, lines)
	return c.hub.Send(context.Background(), hub.NewMessage(hub.MessageTypeSynthesisRequest, "hub", prompt, round))
}

// synthesisPrompt asks for one combined answer to the question. Deciding what
// counts as a conflict is the host's job, not the model's.
func synthesisPrompt(question string, responses []string) string {
	var b strings.Builder
	b.WriteString("Several participants answered the question below. Write one concise answer ")
	b.WriteString("that represents the group.")
	b.WriteString("\n\nQuestion: ")
	b.WriteString(question)
	b.WriteString("\n\nResponses:\n")
	b.WriteString(strings.Join(responses, "\n"))
	return b.String()
}

func (c *Coordinator) record(msg *hub.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.transcript = append(c.transcript, msg)
	return nil
}

// Transcript returns a copy of the recorded messages.
func (c *Coordinator) Transcript() []*hub.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*hub.Message, len(c.transcript))
	copy(out, c.transcript)
	return out
}
