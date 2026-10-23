package hub

import (
	"encoding/json"
	"time"
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
	ID        string            `json:"id"`
	Type      MessageType       `json:"type"`
	From      string            `json:"from"`
	To        string            `json:"to,omitempty"`
	Content   string            `json:"content"`
	Timestamp time.Time         `json:"timestamp"`
	Round     int               `json:"round"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

func NewMessage(msgType MessageType, from, content string, round int) *Message {
	return &Message{
		ID:        generateID(),
		Type:      msgType,
		From:      from,
		Content:   content,
		Timestamp: time.Now(),
		Round:     round,
		Metadata:  make(map[string]string),
	}
}

func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

func generateID() string {
	return time.Now().Format("20060102150405.000000")
}

type Participant struct {
	Name   string            `json:"name"`
	Status ParticipantStatus `json:"status"`
	Round  int               `json:"round"`
}

type HubChannels struct {
	Inbound  chan *Message
	Outbound chan *Message
	Commands chan HubCommand
	Events   chan HubEvent
}

type HubCommand struct {
	Type    CommandType
	Payload interface{}
}

type CommandType string

const (
	CmdAggregate CommandType = "aggregate"
	CmdSend      CommandType = "send"
	CmdNewRound  CommandType = "new_round"
	CmdExport    CommandType = "export"
)

type HubEvent struct {
	Type    EventType
	Payload interface{}
}

type EventType string

const (
	EventMessageReceived   EventType = "message_received"
	EventParticipantUpdate EventType = "participant_update"
	EventRoundChanged      EventType = "round_changed"
	EventExportComplete    EventType = "export_complete"
	EventError             EventType = "error"
)

func NewHubChannels() *HubChannels {
	return &HubChannels{
		Inbound:  make(chan *Message, 100),
		Outbound: make(chan *Message, 100),
		Commands: make(chan HubCommand, 10),
		Events:   make(chan HubEvent, 100),
	}
}
