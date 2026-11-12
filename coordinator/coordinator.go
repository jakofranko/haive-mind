// Package coordinator holds the session-level state that sits beside the hub:
// the transcript, the current round, the participant responses collected
// for each round, and which participants the host has flagged for revision.
// It applies hub messages one at a time, in the order the hub sent them; it
// never invokes the agent (the hub owns that).
//
// State is the synchronous core; Coordinator is the thin adapter that feeds it
// from one hub subscription and sends what it returns.
package coordinator

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jakofranko/haive-mind/hub"
)

type Coordinator struct {
	hub hub.Hub

	mu     sync.Mutex
	state  *State
	export ExportWriter
}

// ExportWriter saves an export of the transcript under the file name the host
// asked for and returns where it went. The server provides it; the Coordinator
// only decides when the transcript is read.
type ExportWriter func(transcript []*hub.Message, name string) (path string, err error)

// SetExportWriter sets what export requests are written with. With none set,
// an export request does nothing.
func (c *Coordinator) SetExportWriter(w ExportWriter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.export = w
}

func New(h hub.Hub) *Coordinator {
	return &Coordinator{hub: h, state: NewState()}
}

// Start subscribes the coordinator to the hub. One subscription for every type
// it needs, so messages are applied in the order the hub sent them.
func (c *Coordinator) Start(ctx context.Context) error {
	_, err := c.hub.SubscribeTypes(ctx, AppliedTypes(), c.apply)
	return err
}

// apply runs on the subscription's single mailbox, so the sends go out in the
// order Apply returned them, and before the next message is applied. Sends
// fail only once the hub has stopped, so a failure does not skip the rest.
func (c *Coordinator) apply(msg *hub.Message) error {
	c.mu.Lock()
	out := c.state.Apply(msg)
	write := c.export
	exporting := msg.Type == hub.MessageTypeExport && write != nil
	var transcript []*hub.Message
	if exporting {
		transcript = c.state.Transcript()
	}
	c.mu.Unlock()
	if exporting {
		out = append(out, c.writeExport(write, transcript, msg.Content)...)
	}
	var first error
	for _, m := range out {
		if err := c.hub.Send(context.Background(), m); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// writeExport writes the transcript as it stood when the export request was
// applied, and announces where it went. A failed write is logged and announces
// nothing.
func (c *Coordinator) writeExport(write ExportWriter, transcript []*hub.Message, name string) []*hub.Message {
	path, err := write(transcript, name)
	if err != nil {
		slog.Error("Failed to export transcript", "error", err)
		return nil
	}
	return []*hub.Message{hub.NewMessage(hub.MessageTypeExportComplete, "hub", path, 0)}
}

// Round returns the current round number.
func (c *Coordinator) Round() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.Round()
}

// Flagged returns the ids of participants the host has asked to revise, sorted.
func (c *Coordinator) Flagged() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.Flagged()
}

// LastSubmission returns the latest response from the participant with id in
// the current round, or nil when it has none.
func (c *Coordinator) LastSubmission(id string) *hub.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.LastSubmission(id)
}

// Transcript returns a copy of the recorded messages.
func (c *Coordinator) Transcript() []*hub.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.Transcript()
}
