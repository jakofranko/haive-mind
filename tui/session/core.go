package session

import (
	"context"
	"log/slog"
	"sync"

	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/inbox"
)

// CatchUpEvent is the first event a session's TUI is given, once its
// Transcript snapshot has arrived: the earlier conversation (oldest first,
// including anything that arrived live before the snapshot, each once), the
// round, who is flagged and who is connected. Nothing comes before it,
// whatever order the Round state and the snapshot reach the session in.
type CatchUpEvent struct {
	Messages     []hub.Message
	Round        int
	Flagged      []string
	Participants []hub.Participant
}

// RevisionEvent is the host asking this participant to revise. It comes once
// each time they become flagged, with their last answer for the current round
// to edit, or "" when they have none.
type RevisionEvent struct{ LastAnswer string }

func (CatchUpEvent) event()  {}
func (RevisionEvent) event() {}

// roleHooks is the part of a Session client that differs between roles. The
// core calls each hook with its lock held, so a hook may read and change the
// core's state and the role's own.
type roleHooks interface {
	// caughtUp runs as Catch-up is made, with the last submission the snapshot
	// carried, and returns the events that follow Catch-up.
	caughtUp(last *hub.Message) []Event
	// flagsSet runs when Round state names who is flagged, and returns the
	// events that follow the RoundEvent.
	flagsSet() []Event
	// roundChanged runs when a new round starts.
	roundChanged()
	// event turns any other message, seen for the first time, into the
	// role's event for it, or nil to drop it.
	event(msg *hub.Message) Event
}

// core is what every role's Session client shares: one ordered subscription,
// the Catch-up gate, dropping messages already given, and where the round
// stands.
type core struct {
	hub   hub.Hub
	id    string // this session's participant id
	inbox *inbox.Queue[Event]
	ready chan struct{}
	hooks roleHooks

	// mu guards everything below and the role's own state: the hub delivers
	// on one goroutine and the TUI sends commands on another.
	mu       sync.Mutex
	sub      hub.Subscription
	seen     map[string]bool
	caughtUp bool
	held     []*hub.Message // what arrived before the snapshot
	round    int
	flagged  []string
}

func newCore(h hub.Hub, id string, hooks roleHooks) *core {
	c := &core{
		hub:   h,
		id:    id,
		inbox: inbox.New[Event](),
		ready: make(chan struct{}),
		hooks: hooks,
		seen:  map[string]bool{},
		round: 1,
	}
	return c
}

// subscribe starts delivery. Everything arrives through one subscription, so
// in the order the hub sent it.
func (c *core) subscribe(types []hub.MessageType) {
	sub, err := c.hub.SubscribeTypes(context.Background(), types, c.receive)
	if err != nil {
		slog.Error("session client could not subscribe to the hub", "error", err)
		c.inbox.Close()
	} else {
		c.mu.Lock()
		c.sub = sub
		c.mu.Unlock()
	}
	close(c.ready)
}

func (c *core) receive(msg *hub.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case msg.Type == hub.MessageTypeTranscriptSnapshot:
		if msg.To == c.id && !c.caughtUp {
			c.catchUp(msg)
		}
	case !c.caughtUp:
		c.held = append(c.held, msg)
	default:
		c.apply(msg)
	}
	return nil
}

// catchUp makes the one Catch-up event from the snapshot and what was held
// back before it, then lets later messages through.
func (c *core) catchUp(snapshot *hub.Message) {
	recorded, last, err := snapshot.TranscriptSnapshot()
	if err != nil {
		slog.Error("unreadable transcript snapshot", "error", err)
		return
	}
	var ev CatchUpEvent
	var after []Event // what arrived early that is not a transcript entry
	for _, m := range recorded {
		if m.Type.Shown() && c.firstSight(m.ID) {
			ev.Messages = append(ev.Messages, *m)
		}
	}
	for _, m := range c.held {
		switch m.Type {
		case hub.MessageTypeRoundChange:
			c.round, c.flagged = m.Round, nil
		case hub.MessageTypeRoundState:
			if round, ids, err := m.RoundState(); err == nil {
				c.round, c.flagged = round, ids
			}
		case hub.MessageTypeParticipantEvent:
			// The roster below is read after it, so it already says this.
		default:
			if !c.firstSight(m.ID) {
				continue
			}
			switch e := c.hooks.event(m).(type) {
			case nil:
			case MessageEvent:
				ev.Messages = append(ev.Messages, e.Message)
			default:
				after = append(after, e)
			}
		}
	}
	c.held = nil
	c.round = snapshot.Round // the snapshot is newer than anything held
	c.caughtUp = true

	ev.Round = c.round
	ev.Flagged = append([]string{}, c.flagged...)
	for _, p := range c.hub.GetParticipants() {
		ev.Participants = append(ev.Participants, *p)
	}
	follow := c.hooks.caughtUp(last)
	c.inbox.Push(ev)
	c.push(after)
	c.push(follow)
}

// apply turns one live message into events.
func (c *core) apply(msg *hub.Message) {
	switch msg.Type {
	case hub.MessageTypeParticipantEvent:
		if p := lookupParticipant(c.hub, msg); p != nil {
			c.inbox.Push(ParticipantEvent{Participant: p})
		}
	case hub.MessageTypeRoundChange:
		c.round, c.flagged = msg.Round, nil
		c.hooks.roundChanged()
		c.inbox.Push(RoundEvent{Round: c.round, Flagged: []string{}})
	case hub.MessageTypeRoundState:
		round, ids, err := msg.RoundState()
		if err != nil {
			return
		}
		c.round, c.flagged = round, ids
		follow := c.hooks.flagsSet()
		c.inbox.Push(RoundEvent{Round: c.round, Flagged: append([]string{}, ids...)})
		c.push(follow)
	default:
		if !c.firstSight(msg.ID) {
			return
		}
		if ev := c.hooks.event(msg); ev != nil {
			c.inbox.Push(ev)
		}
	}
}

// transcriptEntry is the event for a message every role shows as is.
func transcriptEntry(msg *hub.Message) Event {
	if msg.Type.Shown() {
		return MessageEvent{Message: *msg}
	}
	return nil
}

func (c *core) push(evs []Event) {
	for _, ev := range evs {
		c.inbox.Push(ev)
	}
}

// flaggedSelf reports whether this session's participant is flagged.
func (c *core) flaggedSelf() bool {
	for _, id := range c.flagged {
		if id == c.id {
			return true
		}
	}
	return false
}

// firstSight records a message id and reports whether it is new to this
// session. A message without an id is always new. Call it with mu held.
func (c *core) firstSight(id string) bool {
	if id == "" {
		return true
	}
	if c.seen[id] {
		return false
	}
	c.seen[id] = true
	return true
}

// Next returns the next event, blocking until one arrives. Once the client is
// closed and what was already queued is drained, it returns ok=false.
func (c *core) Next() (Event, bool) { return c.inbox.Next() }

// Ready is closed once the client's subscription is in place. If subscribing
// failed, the error is logged and Next reports ok=false immediately.
func (c *core) Ready() <-chan struct{} { return c.ready }

// Close unsubscribes from the hub and releases a pending Next. It may be
// called more than once.
func (c *core) Close() {
	c.mu.Lock()
	sub := c.sub
	c.sub = nil
	c.mu.Unlock()
	if sub != nil {
		sub.Unsubscribe()
	}
	c.inbox.Close()
}

// lookupParticipant is the participant a participant event is about. The hub
// has already forgotten one who disconnected, so the event itself says who to
// remove.
func lookupParticipant(h hub.Hub, msg *hub.Message) *hub.Participant {
	if msg.Content == hub.ParticipantEventDisconnect {
		return &hub.Participant{ID: msg.ID, Status: hub.StatusDisconnected, Event: msg.Content}
	}
	for _, p := range h.GetParticipants() {
		if p.ID == msg.ID {
			return p
		}
	}
	return nil
}
