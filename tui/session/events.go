// Package session provides the Session client: a session's end of the
// conversation with the hub, in both directions. There is one per role:
// Participant and Host. Each knows where its session stands in the round,
// hands its TUI events that say what happened (never a raw message to
// inspect), and sends on the session's behalf, stamping the round and the
// sender's identity. Nothing reaches the TUI before Catch-up. A client does
// not depend on Bubble Tea; each model wraps Next in its own thin command.
package session

import "jakofranko/haive-mind/hub"

// Event is something that happened, as a session's TUI needs to know it.
// Every role gets CatchUpEvent, MessageEvent, ParticipantEvent and
// RoundEvent; a participant also gets RevisionEvent, and the host gets
// DraftEvent, AgentFailedEvent and ExportEvent.
type Event interface{ event() }

// MessageEvent is a transcript entry to show as it is.
type MessageEvent struct{ Message hub.Message }

// ParticipantEvent is a participant connecting, changing status or leaving.
// A disconnect carries Status hub.StatusDisconnected.
type ParticipantEvent struct{ Participant *hub.Participant }

// RoundEvent says which round it is and who is flagged. It comes for both a
// Round change and a Round state; after a Round change no one is flagged.
type RoundEvent struct {
	Round   int
	Flagged []string
}

// ExportEvent reports a finished transcript export, with the file it went to.
// Only the host receives it.
type ExportEvent struct{ Path string }

func (MessageEvent) event()     {}
func (ParticipantEvent) event() {}
func (RoundEvent) event()       {}
func (ExportEvent) event()      {}
