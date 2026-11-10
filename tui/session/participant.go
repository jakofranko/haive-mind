package session

import (
	"context"
	"strings"

	"jakofranko/haive-mind/hub"
)

// Participant is a participant session's Session client. It knows where the
// participant stands in the round, gives the participant TUI Catch-up and then
// MessageEvent, ParticipantEvent, RoundEvent and RevisionEvent, reports the
// participant's status to the hub, and sends their responses stamped with
// their id, display name and the current round.
//
// It needs a Coordinator on the hub: until the Transcript snapshot addressed
// to it arrives, it gives the TUI nothing.
type Participant struct {
	*core

	// Guarded by core.mu.
	displayName     string // what responses are sent under; the id until one is chosen
	status          hub.ParticipantStatus
	wasFlagged      bool   // flagged at the last Round state, so a repeat does not ask again
	lastAnswer      string // the last response sent, offered back when the host asks for a revision
	lastAnswerRound int    // the round lastAnswer answers; it is offered back only in that round
}

// NewParticipant subscribes to the hub for the participant with id and
// returns their Session client. Create it before the participant is
// registered, so the Transcript snapshot for that registration reaches it.
func NewParticipant(h hub.Hub, id string) *Participant {
	p := &Participant{displayName: h.GetDisplayName(id), status: hub.StatusIdle}
	if p.displayName == "" {
		p.displayName = id
	}
	p.core = newCore(h, id, p)
	types := append(hub.ShownTypes(),
		hub.MessageTypeRoundState,
		hub.MessageTypeRoundChange,
		hub.MessageTypeParticipantEvent,
		hub.MessageTypeTranscriptSnapshot,
	)
	p.subscribe(types)
	return p
}

// DisplayName is the display name this participant chose, in this session or
// an earlier one, or "" if they have not chosen one yet.
func (p *Participant) DisplayName() string { return p.hub.GetDisplayName(p.id) }

// UnsentText is what the participant had typed but not sent, kept from an
// earlier session.
func (p *Participant) UnsentText() string { return p.hub.GetDraft(p.id) }

// Rename sets the participant's display name.
func (p *Participant) Rename(name string) {
	p.mu.Lock()
	p.displayName = name
	p.mu.Unlock()
	p.hub.SetDisplayName(p.id, name)
}

// Typed reports what is in the response box after the participant changed
// it. The text is kept as unsent text, and the participant is typing while
// there is any; emptying the box makes a typing participant idle again.
func (p *Participant) Typed(text string) {
	p.hub.SetDraft(p.id, text)
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case strings.TrimSpace(text) != "":
		p.setStatus(hub.StatusTyping)
	case p.status == hub.StatusTyping:
		p.setStatus(hub.StatusIdle)
	}
}

// Submit sends text as the participant's response to the current round and
// reports whether it sent anything: blank text is not sent. The participant
// is then submitted, and their unsent text is cleared.
func (p *Participant) Submit(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	p.mu.Lock()
	msg := hub.NewMessage(hub.MessageTypeResponse, p.displayName, text, p.round)
	msg.FromID = p.id
	p.lastAnswer, p.lastAnswerRound = text, p.round
	p.setStatus(hub.StatusSubmitted)
	p.mu.Unlock()

	p.hub.Send(context.Background(), msg)
	p.hub.SetDraft(p.id, "")
	return true
}

// setStatus tells the hub what the participant is doing, once per change.
// Call it with mu held.
func (p *Participant) setStatus(status hub.ParticipantStatus) {
	if p.status == status {
		return
	}
	p.status = status
	p.hub.UpdateParticipantStatus(p.id, status)
}

func (p *Participant) caughtUp(last *hub.Message) []Event {
	// An answer sent in this session is at least as new as the snapshot's.
	if p.lastAnswer == "" && last != nil {
		p.lastAnswer, p.lastAnswerRound = last.Content, last.Round
	}
	return p.flagsSet()
}

// flagsSet asks for a revision when the participant becomes flagged, and
// makes them idle so the host sees they have not started on it.
func (p *Participant) flagsSet() []Event {
	flagged := p.flaggedSelf()
	newly := flagged && !p.wasFlagged
	p.wasFlagged = flagged
	if !newly {
		return nil
	}
	p.setStatus(hub.StatusIdle)
	answer := ""
	if p.lastAnswerRound == p.round {
		answer = p.lastAnswer
	}
	return []Event{RevisionEvent{LastAnswer: answer}}
}

func (p *Participant) roundChanged() {
	p.wasFlagged = false
	p.setStatus(hub.StatusIdle)
}
