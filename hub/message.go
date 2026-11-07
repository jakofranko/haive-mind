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
	MessageTypeSystem           MessageType = "system"
	MessageTypeAggregate        MessageType = "aggregate"
	MessageTypeNewRound         MessageType = "new_round"
	MessageTypeRoundChange      MessageType = "round_change"
	MessageTypeExport           MessageType = "export"
	MessageTypeExportComplete   MessageType = "export_complete"
	MessageTypeParticipantEvent MessageType = "participant_event"
	MessageTypeSynthesisRequest MessageType = "synthesis_request"
	MessageTypeAggregateDraft   MessageType = "aggregate_draft"
	MessageTypeAgentError       MessageType = "agent_error"
)

type Message struct {
	ID        string      `json:"id"`
	Type      MessageType `json:"type"`
	From      string      `json:"from"`
	To        string      `json:"to,omitempty"`
	Content   string      `json:"content"`
	Timestamp time.Time   `json:"timestamp"`
	Round     int         `json:"round"`
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

func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

type Participant struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Status      ParticipantStatus `json:"status"`
	Round       int               `json:"round"`
	Event       string            `json:"event,omitempty"` // "connect", "disconnect", "status_change"
}

const (
	ParticipantEventConnect      = "connect"
	ParticipantEventDisconnect   = "disconnect"
	ParticipantEventStatusChange = "status_change"
)

func NewParticipantEvent(name, displayName string, status ParticipantStatus, round int, event string) *Participant {
	return &Participant{
		Name:        name,
		DisplayName: displayName,
		Status:      status,
		Round:       round,
		Event:       event,
	}
}

// Retry names a message to send again after the call it triggered failed.
type Retry struct {
	Type    MessageType `json:"type"`
	Content string      `json:"content"`
}
