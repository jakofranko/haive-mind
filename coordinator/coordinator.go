// Package coordinator holds the session-level state that sits beside the hub:
// the transcript, the current round, and the participant responses collected
// for each round. It reacts to hub messages; it never invokes the agent (the
// hub owns that).
package coordinator

import (
	"context"
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
}

// aggregateRequest is the Content of the signal message that asks the
// coordinator to compose an aggregate (see hub.RequestAggregate).
const aggregateRequest = "aggregate_request"

func New(h hub.Hub) *Coordinator {
	return &Coordinator{
		hub:       h,
		round:     1,
		responses: make(map[int][]*hub.Message),
	}
}

// transcriptTypes are the message types recorded in the transcript.
var transcriptTypes = []hub.MessageType{
	hub.MessageTypeDirective,
	hub.MessageTypeResponse,
	hub.MessageTypeSystem,
	hub.MessageTypeAggregate,
	hub.MessageTypeNewRound,
	hub.MessageTypeRoundChange,
	hub.MessageTypeExport,
	hub.MessageTypeExportComplete,
}

// Start subscribes the coordinator to the hub.
func (c *Coordinator) Start(ctx context.Context) error {
	for _, t := range transcriptTypes {
		if _, err := c.hub.Subscribe(ctx, t, c.record); err != nil {
			return err
		}
	}
	if _, err := c.hub.Subscribe(ctx, hub.MessageTypeResponse, c.trackResponse); err != nil {
		return err
	}
	if _, err := c.hub.Subscribe(ctx, hub.MessageTypeAggregate, c.handleAggregate); err != nil {
		return err
	}
	if _, err := c.hub.Subscribe(ctx, hub.MessageTypeNewRound, c.handleNewRound); err != nil {
		return err
	}
	return nil
}

// handleNewRound advances the round and announces the new number.
func (c *Coordinator) handleNewRound(msg *hub.Message) error {
	c.mu.Lock()
	c.round++
	round := c.round
	c.responses[round] = nil
	c.mu.Unlock()
	return c.hub.Send(context.Background(), hub.NewMessage(hub.MessageTypeRoundChange, "hub", "", round))
}

// Round returns the current round number.
func (c *Coordinator) Round() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.round
}

// trackResponse remembers participant (non-agent) responses by round.
func (c *Coordinator) trackResponse(msg *hub.Message) error {
	if msg.From == "agent" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.responses[msg.Round] = append(c.responses[msg.Round], msg)
	return nil
}

// handleAggregate reacts only to aggregate requests. The aggregate it sends
// back has real content, so it is ignored here: that is what stops the loop.
func (c *Coordinator) handleAggregate(msg *hub.Message) error {
	if msg.Content != aggregateRequest {
		return nil
	}
	c.mu.Lock()
	round := c.round
	parts := make([]string, 0, len(c.responses[round]))
	for _, r := range c.responses[round] {
		parts = append(parts, r.From+": "+r.Content)
	}
	c.mu.Unlock()

	if len(parts) == 0 {
		return nil
	}
	content := "Aggregated responses: " + strings.Join(parts, "; ")
	return c.hub.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregate, "Host", content, round))
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
