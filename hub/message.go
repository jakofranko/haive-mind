package hub

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ParticipantStatus string

const (
	StatusIdle      ParticipantStatus = "idle"
	StatusTyping    ParticipantStatus = "typing"
	StatusSubmitted ParticipantStatus = "submitted"
	StatusReady     ParticipantStatus = "ready"
)

type MessageType string

const (
	MessageTypeDirective MessageType = "directive"
	MessageTypeResponse  MessageType = "response"
	MessageTypeSystem    MessageType = "system"
	MessageTypeAggregate MessageType = "aggregate"
)

type Message struct {
	ID        string      `json:"id"`
	Type      MessageType `json:"type"`
	From      string      `json:"from"`
	To        string      `json:"to,omitempty"`
	Content   string      `json:"content"`
	Timestamp time.Time   `json:"timestamp"`
	Round     int         `json:"round"`
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
	Name   string            `json:"name"`
	Status ParticipantStatus `json:"status"`
	Round  int               `json:"round"`
}
