package session

import (
	"context"
	"strings"
	"time"

	"jakofranko/haive-mind/hub"
)

// DraftEvent is a synthesized draft of the aggregate, for the host to edit.
// It is also a transcript entry.
type DraftEvent struct{ Message hub.Message }

// AgentFailedEvent is a failed call to the agent or synthesizer. Text says
// what went wrong; when CanRetry is set, Host.Retry sends the failed message
// again.
type AgentFailedEvent struct {
	Text     string
	CanRetry bool
	failed   hub.Message
}

func (DraftEvent) event()       {}
func (AgentFailedEvent) event() {}

// Host is the host session's Session client. It gives the host TUI Catch-up
// and then MessageEvent, DraftEvent, AgentFailedEvent, ExportEvent,
// ParticipantEvent and RoundEvent, and sends the host's directives,
// aggregates and requests stamped with the current round and the host's
// display name.
//
// It needs a Coordinator on the hub: until the Transcript snapshot addressed
// to the host arrives, it gives the TUI nothing.
type Host struct {
	*core

	displayName string // guarded by core.mu; what the host's messages are sent under
}

// NewHost subscribes to the hub for the host and returns their Session
// client. Create it before the host is registered, so the Transcript snapshot
// for that registration reaches it.
func NewHost(h hub.Hub) *Host {
	c := &Host{displayName: h.GetDisplayName(hub.HostID)}
	if c.displayName == "" {
		c.displayName = "Host"
	}
	c.core = newCore(h, hub.HostID, c)
	types := append(hub.ShownTypes(),
		hub.MessageTypeRoundState,
		hub.MessageTypeRoundChange,
		hub.MessageTypeParticipantEvent,
		hub.MessageTypeTranscriptSnapshot,
		hub.MessageTypeAgentError,
		hub.MessageTypeExportComplete,
	)
	c.subscribe(types)
	return c
}

// DisplayName is the display name the host chose, in this session or an
// earlier one, or "" if they have not chosen one yet.
func (c *Host) DisplayName() string { return c.hub.GetDisplayName(hub.HostID) }

// Rename sets the host's display name.
func (c *Host) Rename(name string) {
	c.mu.Lock()
	c.displayName = name
	c.mu.Unlock()
	c.hub.SetDisplayName(hub.HostID, name)
}

// SendDirective sends text to the agent as a directive in the current round,
// and reports whether it sent anything: blank text is not sent.
func (c *Host) SendDirective(text string) bool {
	return c.send(hub.MessageTypeDirective, text)
}

// SendAggregate sends text to the agent as the round's aggregate, and reports
// whether it sent anything: blank text is not sent.
func (c *Host) SendAggregate(text string) bool {
	return c.send(hub.MessageTypeAggregate, text)
}

func (c *Host) send(t hub.MessageType, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	c.mu.Lock()
	msg := c.fromHost(t, text, c.round)
	c.mu.Unlock()
	c.hub.Send(context.Background(), msg)
	return true
}

// fromHost is a message from the host. Call it with mu held.
func (c *Host) fromHost(t hub.MessageType, text string, round int) *hub.Message {
	msg := hub.NewMessage(t, c.displayName, text, round)
	msg.FromID = hub.HostID
	return msg
}

// Retry sends the message whose agent call failed again, for the round it
// was sent in. It does nothing for a failure that cannot be retried.
func (c *Host) Retry(f AgentFailedEvent) error {
	if !f.CanRetry {
		return nil
	}
	c.mu.Lock()
	msg := c.fromHost(f.failed.Retry.Type, f.failed.Retry.Content, f.failed.Round)
	c.mu.Unlock()
	return c.hub.Send(context.Background(), msg)
}

// RequestAggregate asks for the current round's responses to be synthesized
// into a draft.
func (c *Host) RequestAggregate() { c.hub.RequestAggregate(c.currentRound()) }

// NewRound asks for the current round to end. It names the round being
// ended, so asking twice before the new round arrives starts one round.
func (c *Host) NewRound() { c.hub.StartRound(c.currentRound()) }

// Resolve clears every flag.
func (c *Host) Resolve() {
	c.hub.Send(context.Background(), hub.NewResolve(c.currentRound()))
}

// Flag asks the participant with id to revise. The host cannot flag
// themselves.
func (c *Host) Flag(id string) {
	if id == hub.HostID {
		return
	}
	c.hub.Send(context.Background(), hub.NewFlag(c.currentRound(), id))
}

// Unflag withdraws the request for the participant with id to revise.
func (c *Host) Unflag(id string) {
	c.hub.Send(context.Background(), hub.NewUnflag(c.currentRound(), id))
}

// Export writes the transcript to a new timestamped file. An ExportEvent says
// where it went.
func (c *Host) Export() {
	c.hub.Export("transcript_" + time.Now().Format("20060102_150405") + ".json")
}

func (c *Host) currentRound() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.round
}

func (c *Host) event(msg *hub.Message) Event {
	switch msg.Type {
	case hub.MessageTypeAggregateDraft:
		return DraftEvent{Message: *msg}
	case hub.MessageTypeAgentError:
		return AgentFailedEvent{Text: msg.Content, CanRetry: msg.Retry != nil, failed: *msg}
	case hub.MessageTypeExportComplete:
		return ExportEvent{Path: msg.Content}
	}
	return transcriptEntry(msg)
}

// The host has no status to report and no answer to revise.
func (c *Host) caughtUp(*hub.Message) []Event { return nil }
func (c *Host) flagsSet() []Event             { return nil }
func (c *Host) roundChanged()                 {}
