package hub

import (
	"time"
)

// MessageType represents the type of a message
type MessageType string

const (
	MessageTypeAgent       MessageType = "agent"
	MessageTypeParticipant MessageType = "participant"
	MessageTypeAggregated  MessageType = "aggregated"
	MessageTypeSystem      MessageType = "system"
)

// Participant represents a participant in a session
type Participant struct {
	ID        string
	Name      string
	JoinedAt  time.Time
	IsActive  bool
}

// Session represents a collaborative session
type Session struct {
	ID        string
	CreatedAt time.Time
	Round     int
	Status    SessionStatus
}

// SessionStatus represents the status of a session
type SessionStatus string

const (
	SessionStatusActive   SessionStatus = "active"
	SessionStatusPaused   SessionStatus = "paused"
	SessionStatusComplete SessionStatus = "complete"
)

// Message represents a message in the session
type Message struct {
	ID        string
	SessionID string
	Round     int
	Type      MessageType
	AuthorID  string
	Content   string
	CreatedAt time.Time
}

// Response represents a participant's response in a round
type Response struct {
	ID        string
	SessionID string
	Round     int
	ParticipantID string
	Content   string
	CreatedAt time.Time
}

// Round represents a round in the session
type Round struct {
	Number      int
	SessionID   string
	Status      RoundStatus
	StartedAt   time.Time
	CompletedAt *time.Time
	Responses   []Response
}

// RoundStatus represents the status of a round
type RoundStatus string

const (
	RoundStatusPending   RoundStatus = "pending"
	RoundStatusActive    RoundStatus = "active"
	RoundStatusCompleted RoundStatus = "completed"
)