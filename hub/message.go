package hub

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ParticipantStatus string

const (
	StatusIdle         ParticipantStatus = "idle"
	StatusTyping       ParticipantStatus = "typing"
	StatusSubmitted    ParticipantStatus = "submitted"
	StatusReady        ParticipantStatus = "ready"
	StatusDisconnected ParticipantStatus = "disconnected"
)

type MessageType string

const (
	MessageTypeDirective        MessageType = "directive"
	MessageTypeResponse         MessageType = "response"
	MessageTypeAgentReply       MessageType = "agent_reply"
	MessageTypeSystem           MessageType = "system"
	MessageTypeAggregate        MessageType = "aggregate"
	MessageTypeAggregateRequest MessageType = "aggregate_request"
	MessageTypeNewRound         MessageType = "new_round"
	MessageTypeRoundChange      MessageType = "round_change"
	MessageTypeExport           MessageType = "export"
	MessageTypeExportComplete   MessageType = "export_complete"
	MessageTypeParticipantEvent MessageType = "participant_event"
	MessageTypeSynthesisRequest MessageType = "synthesis_request"
	MessageTypeAggregateDraft   MessageType = "aggregate_draft"
	MessageTypeAgentError       MessageType = "agent_error"
	MessageTypeFlag             MessageType = "flag"
	MessageTypeUnflag           MessageType = "unflag"
	MessageTypeResolve          MessageType = "resolve"
	MessageTypeRoundState       MessageType = "round_state"

	MessageTypeTranscriptSnapshot MessageType = "transcript_snapshot"
)

// shownTypes are the message types a session shows as transcript entries.
var shownTypes = []MessageType{
	MessageTypeDirective,
	MessageTypeResponse,
	MessageTypeAgentReply,
	MessageTypeSystem,
	MessageTypeAggregate,
	MessageTypeAggregateDraft,
}

// recordedTypes are the message types the coordinator keeps in the transcript:
// everything shown, plus the round and export events the export file needs.
var recordedTypes = append(append([]MessageType{}, shownTypes...),
	MessageTypeNewRound,
	MessageTypeRoundChange,
	MessageTypeExport,
	MessageTypeExportComplete,
)

// ShownTypes returns the message types a session shows as transcript entries,
// whether they arrive live or in a transcript snapshot.
func ShownTypes() []MessageType { return append([]MessageType{}, shownTypes...) }

// RecordedTypes returns the message types recorded in the transcript. Every
// shown type is recorded; not every recorded type is shown.
func RecordedTypes() []MessageType { return append([]MessageType{}, recordedTypes...) }

// Shown reports whether a session shows a message of this type as a
// transcript entry.
func (t MessageType) Shown() bool {
	for _, s := range shownTypes {
		if s == t {
			return true
		}
	}
	return false
}

type Message struct {
	ID   string      `json:"id"`
	Type MessageType `json:"type"`
	From string      `json:"from"`
	To   string      `json:"to,omitempty"`
	// FromID is the sender's stable participant id, when From is a display name.
	FromID    string    `json:"from_id,omitempty"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Round     int       `json:"round"`
	// Retry, on an agent_error, is the message to send again to retry.
	Retry *Retry `json:"retry,omitempty"`
}

func NewMessage(msgType MessageType, from, content string, round int) *Message {
	return &Message{
		ID:        uuid.New().String(),
		Type:      msgType,
		From:      from,
		Content:   content,
		Timestamp: time.Now(),
		Round:     round,
	}
}

// NewFlag is the host asking the participant with id target to revise.
func NewFlag(round int, target string) *Message {
	m := NewMessage(MessageTypeFlag, HostID, "", round)
	m.To = target
	return m
}

// NewUnflag is the host withdrawing the Flag on the participant with id target.
func NewUnflag(round int, target string) *Message {
	m := NewMessage(MessageTypeUnflag, HostID, "", round)
	m.To = target
	return m
}

// NewResolve is the host clearing every Flag at once.
func NewResolve(round int) *Message {
	return NewMessage(MessageTypeResolve, HostID, "", round)
}

// NewRoundState publishes the current round and the flagged Participant ids.
func NewRoundState(round int, ids []string) *Message {
	if ids == nil {
		ids = []string{}
	}
	payload, _ := json.Marshal(ids)
	return NewMessage(MessageTypeRoundState, "hub", string(payload), round)
}

// RoundState returns the whole concept a Round state message carries: the
// round and the flagged Participant ids. Read both from here rather than
// reaching into the message's Round field separately.
func (m Message) RoundState() (round int, ids []string, err error) {
	if err := json.Unmarshal([]byte(m.Content), &ids); err != nil {
		return 0, nil, err
	}
	return m.Round, ids, nil
}

// transcriptSnapshot is the payload of a Transcript snapshot message.
type transcriptSnapshot struct {
	Messages       []*Message `json:"messages"`
	LastSubmission *Message   `json:"last_submission,omitempty"`
}

// NewTranscriptSnapshot is the recorded conversation for the one session with
// participant id to, plus that participant's last submission for the current
// round (nil when there is none).
func NewTranscriptSnapshot(round int, to string, transcript []*Message, last *Message) *Message {
	if transcript == nil {
		transcript = []*Message{}
	}
	payload, _ := json.Marshal(transcriptSnapshot{Messages: transcript, LastSubmission: last})
	m := NewMessage(MessageTypeTranscriptSnapshot, "hub", string(payload), round)
	m.To = to
	return m
}

// TranscriptSnapshot returns the whole concept a Transcript snapshot message
// carries: the recorded messages and the addressee's last submission, or nil.
func (m Message) TranscriptSnapshot() (transcript []*Message, last *Message, err error) {
	var s transcriptSnapshot
	if err := json.Unmarshal([]byte(m.Content), &s); err != nil {
		return nil, nil, err
	}
	return s.Messages, s.LastSubmission, nil
}

func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

type Participant struct {
	ID          string            `json:"id"`
	DisplayName string            `json:"display_name"`
	Status      ParticipantStatus `json:"status"`
	Event       string            `json:"event,omitempty"` // "connect", "disconnect", "status_change"
}

const (
	ParticipantEventConnect      = "connect"
	ParticipantEventDisconnect   = "disconnect"
	ParticipantEventStatusChange = "status_change"
)

func NewParticipantEvent(id, displayName string, status ParticipantStatus, event string) *Participant {
	return &Participant{
		ID:          id,
		DisplayName: displayName,
		Status:      status,
		Event:       event,
	}
}

// Retry names a message to send again after the call it triggered failed.
type Retry struct {
	Type    MessageType `json:"type"`
	Content string      `json:"content"`
}
