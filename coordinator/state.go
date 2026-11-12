package coordinator

import (
	"slices"
	"sort"
	"strings"

	"github.com/jakofranko/haive-mind/hub"
)

// State is the Coordinator's synchronous core: the session state, changed one
// hub message at a time. It has no hub and no goroutines; Apply returns what
// to publish and the caller sends it. It is not safe for concurrent use.
type State struct {
	transcript []*hub.Message
	round      int
	responses  map[int][]*hub.Message
	directives map[int]string
	flagged    map[string]bool // participant ids the host asked to revise
}

// NewState is the state of a session that has not started: round 1, nothing
// recorded, nobody flagged.
func NewState() *State {
	return &State{
		round:      1,
		responses:  make(map[int][]*hub.Message),
		directives: make(map[int]string),
		flagged:    make(map[string]bool),
	}
}

// handledTypes are the message types Apply acts on beyond recording them.
var handledTypes = []hub.MessageType{
	hub.MessageTypeResponse,
	hub.MessageTypeDirective,
	hub.MessageTypeAggregateRequest,
	hub.MessageTypeNewRound,
	hub.MessageTypeFlag,
	hub.MessageTypeUnflag,
	hub.MessageTypeResolve,
	hub.MessageTypeParticipantEvent,
}

// AppliedTypes returns every message type Apply needs to see: the recorded
// types and the ones it acts on.
func AppliedTypes() []hub.MessageType {
	types := hub.RecordedTypes()
	for _, t := range handledTypes {
		if !slices.Contains(types, t) {
			types = append(types, t)
		}
	}
	return types
}

// Apply applies one hub message and returns the messages to publish, in order.
func (s *State) Apply(msg *hub.Message) []*hub.Message {
	if msg.Type.Recorded() {
		s.transcript = append(s.transcript, msg)
	}
	switch msg.Type {
	case hub.MessageTypeDirective:
		s.directives[msg.Round] = msg.Content
	case hub.MessageTypeResponse:
		return s.trackResponse(msg)
	case hub.MessageTypeAggregateRequest:
		return s.aggregateRequest()
	case hub.MessageTypeNewRound:
		return s.newRound(msg)
	case hub.MessageTypeFlag:
		if msg.To != "" && !s.flagged[msg.To] {
			s.flagged[msg.To] = true
			return []*hub.Message{s.roundState()}
		}
	case hub.MessageTypeUnflag:
		if s.flagged[msg.To] {
			delete(s.flagged, msg.To)
			return []*hub.Message{s.roundState()}
		}
	case hub.MessageTypeResolve:
		if len(s.flagged) > 0 {
			s.flagged = make(map[string]bool)
			return []*hub.Message{s.roundState()}
		}
	case hub.MessageTypeParticipantEvent:
		return s.participantEvent(msg)
	}
	return nil
}

// newRound advances the round and announces the new number. The request names
// the round being ended; one naming any other round is stale or a repeat and
// is ignored. Flags belong to a round, so they end with it.
func (s *State) newRound(msg *hub.Message) []*hub.Message {
	if msg.Round != s.round {
		return nil
	}
	s.round++
	s.responses[s.round] = nil
	s.flagged = make(map[string]bool)
	return []*hub.Message{
		s.roundState(),
		hub.NewMessage(hub.MessageTypeRoundChange, "hub", "", s.round),
	}
}

// participantEvent announces the Round state when someone connects, so a
// session that joins or reconnects mid-session shows where things stand, and
// catches that session up with a Transcript snapshot. The Round state is
// broadcast (ADR 0001) and idempotent, so everyone receiving it again is
// harmless; it never changes anyone's status. The snapshot is an ordinary
// broadcast addressed by To; the hub has no targeted send.
func (s *State) participantEvent(msg *hub.Message) []*hub.Message {
	if msg.Content != hub.ParticipantEventConnect {
		return nil
	}
	return []*hub.Message{
		s.roundState(),
		hub.NewTranscriptSnapshot(s.round, msg.ID, s.Transcript(), s.LastSubmission(msg.ID)),
	}
}

// trackResponse remembers each participant's latest response by round, and
// lifts that participant's revision flag.
func (s *State) trackResponse(msg *hub.Message) []*hub.Message {
	id := senderID(msg)
	replaced := false
	for i, r := range s.responses[msg.Round] {
		if senderID(r) == id {
			s.responses[msg.Round][i] = msg
			replaced = true
			break
		}
	}
	if !replaced {
		s.responses[msg.Round] = append(s.responses[msg.Round], msg)
	}
	if s.flagged[id] {
		delete(s.flagged, id)
		return []*hub.Message{s.roundState()}
	}
	return nil
}

// aggregateRequest asks the hub to synthesize this round's responses. The
// host's confirmed Aggregate is a different message type, so it never lands
// here: that is what stops the loop.
func (s *State) aggregateRequest() []*hub.Message {
	var lines []string
	for _, r := range s.responses[s.round] {
		lines = append(lines, "- "+r.From+": "+r.Content)
	}
	if len(lines) == 0 {
		return nil
	}
	prompt := synthesisPrompt(s.directives[s.round], lines)
	return []*hub.Message{hub.NewMessage(hub.MessageTypeSynthesisRequest, "hub", prompt, s.round)}
}

// roundState tells everyone the current round and who is flagged.
func (s *State) roundState() *hub.Message {
	return hub.NewRoundState(s.round, s.Flagged())
}

// Round returns the current round number.
func (s *State) Round() int { return s.round }

// Flagged returns the ids of participants the host has asked to revise, sorted.
func (s *State) Flagged() []string {
	ids := make([]string, 0, len(s.flagged))
	for id := range s.flagged {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// LastSubmission returns the latest response from the participant with id in
// the current round, or nil when it has none.
func (s *State) LastSubmission(id string) *hub.Message {
	for _, r := range s.responses[s.round] {
		if senderID(r) == id {
			return r
		}
	}
	return nil
}

// Transcript returns a copy of the recorded messages.
func (s *State) Transcript() []*hub.Message {
	out := make([]*hub.Message, len(s.transcript))
	copy(out, s.transcript)
	return out
}

// senderID identifies who sent a response: the stable id when there is one.
func senderID(msg *hub.Message) string {
	if msg.FromID != "" {
		return msg.FromID
	}
	return msg.From
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
