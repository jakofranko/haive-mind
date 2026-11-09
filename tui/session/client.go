// Package session provides the Session client: a session's end of the
// conversation with the hub. It subscribes to the hub, keeps the inbox, and
// hands the host or participant TUI typed events, so the TUI never inspects a
// message's Content or ID to learn what happened. It does not depend on
// Bubble Tea; each model wraps Next in its own thin command.
package session

import (
	"context"
	"log/slog"
	"sync"

	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/inbox"
)

// Event is something the hub told this session. It is one of MessageEvent,
// ParticipantEvent, RoundEvent, RoundStateEvent or ExportEvent.
type Event interface{ event() }

// MessageEvent is a hub message passed through unchanged.
type MessageEvent struct{ Message hub.Message }

// ParticipantEvent is a participant connecting, changing status or leaving.
// A disconnect carries Status hub.StatusDisconnected.
type ParticipantEvent struct{ Participant *hub.Participant }

// RoundEvent is a change of round.
type RoundEvent struct{ Round int }

// RoundStateEvent is the Coordinator's statement of where things stand: the
// current round and the flagged Participant ids. It is not a round change.
type RoundStateEvent struct {
	Round int
	IDs   []string
}

// ExportEvent reports a finished transcript export, with the file it went to.
// Only a client created with hub.MessageTypeExportComplete receives it.
type ExportEvent struct{ Path string }

func (ExportEvent) event()      {}
func (MessageEvent) event()     {}
func (ParticipantEvent) event() {}
func (RoundEvent) event()       {}
func (RoundStateEvent) event()  {}

// transcriptTypes are the message types every session shows in its transcript.
// Adding one here makes it reach every TUI.
var transcriptTypes = []hub.MessageType{
	hub.MessageTypeDirective,
	hub.MessageTypeResponse,
	hub.MessageTypeAgentReply,
	hub.MessageTypeSystem,
	hub.MessageTypeAggregate,
	hub.MessageTypeAggregateDraft,
}

// Client is one session's subscription to the hub. Create it with New.
type Client struct {
	hub   hub.Hub
	inbox *inbox.Queue[Event]
	ready chan struct{}

	mu   sync.Mutex
	subs []hub.Subscription
}

// New subscribes to the hub and returns a client. extra lists message types
// beyond the common transcript ones that this role wants delivered (as
// MessageEvents). Everything, in every type, arrives in the order the hub sent
// it, because it all goes through one subscription.
func New(h hub.Hub, extra ...hub.MessageType) *Client {
	c := &Client{hub: h, inbox: inbox.New[Event](), ready: make(chan struct{})}

	types := append([]hub.MessageType{}, transcriptTypes...)
	types = append(types, hub.MessageTypeRoundState, hub.MessageTypeParticipantEvent, hub.MessageTypeRoundChange)
	types = append(types, extra...)

	sub, err := h.SubscribeTypes(context.Background(), types, c.receive)
	if err != nil {
		slog.Error("session client could not subscribe to the hub", "error", err)
		c.inbox.Close()
	} else {
		c.subs = append(c.subs, sub)
	}
	close(c.ready)
	return c
}

// receive decodes one hub message into an event and queues it.
func (c *Client) receive(msg *hub.Message) error {
	switch msg.Type {
	case hub.MessageTypeParticipantEvent:
		if p := c.participantFor(msg); p != nil {
			c.inbox.Push(ParticipantEvent{Participant: p})
		}
	case hub.MessageTypeRoundChange:
		c.inbox.Push(RoundEvent{Round: msg.Round})
	case hub.MessageTypeRoundState:
		if round, ids, err := msg.RoundState(); err == nil {
			c.inbox.Push(RoundStateEvent{Round: round, IDs: ids})
		}
	case hub.MessageTypeExportComplete:
		c.inbox.Push(ExportEvent{Path: msg.Content})
	default:
		c.inbox.Push(MessageEvent{Message: *msg})
	}
	return nil
}

func (c *Client) participantFor(msg *hub.Message) *hub.Participant {
	// The hub has already forgotten a participant who disconnected, so there is
	// nothing to look up: the event itself says who to remove.
	if msg.Content == hub.ParticipantEventDisconnect {
		return &hub.Participant{ID: msg.ID, Status: hub.StatusDisconnected, Round: msg.Round, Event: msg.Content}
	}
	for _, p := range c.hub.GetParticipants() {
		if p.ID == msg.ID {
			return p
		}
	}
	return nil
}

// Next returns the next event, blocking until one arrives. Once the client is
// closed and what was already queued is drained, it returns ok=false.
func (c *Client) Next() (Event, bool) {
	return c.inbox.Next()
}

// Participants is the hub's current participant list, for a TUI that starts
// with someone already connected.
func (c *Client) Participants() []hub.Participant {
	ps := c.hub.GetParticipants()
	out := make([]hub.Participant, len(ps))
	for i, p := range ps {
		out[i] = *p
	}
	return out
}

// Ready is closed once the client's subscriptions are in place, so messages
// sent after it closes will be delivered. New closes it before it returns. If
// the subscription failed, the error is logged and the client is already
// closed: Next reports ok=false immediately.
func (c *Client) Ready() <-chan struct{} { return c.ready }

// Close unsubscribes from the hub and releases a pending Next. It may be
// called more than once.
func (c *Client) Close() {
	c.mu.Lock()
	subs := c.subs
	c.subs = nil
	c.mu.Unlock()
	for _, s := range subs {
		s.Unsubscribe()
	}
	c.inbox.Close()
}
