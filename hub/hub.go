package hub

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var idCounter uint64

// Hub manages the session state and participants
type Hub struct {
	mu            sync.RWMutex
	sessions      map[string]*Session
	participants  map[string]*Participant
	responses     map[string][]Response
	rounds        map[string]*Round
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		sessions:     make(map[string]*Session),
		participants: make(map[string]*Participant),
		responses:    make(map[string][]Response),
		rounds:       make(map[string]*Round),
	}
}

func roundKey(sessionID string, roundNum int) string {
	return sessionID + ":" + strconv.Itoa(roundNum)
}

// AddParticipant adds a new participant to the hub
func (h *Hub) AddParticipant(id, name string) (*Participant, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.participants[id]; exists {
		return nil, fmt.Errorf("participant %q already exists", id)
	}

	p := &Participant{
		ID:       id,
		Name:     name,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	h.participants[id] = p
	return p, nil
}

// GetParticipant returns a copy of a participant by ID
func (h *Hub) GetParticipant(id string) (Participant, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.participants[id]
	if !ok {
		return Participant{}, false
	}
	return *p, true
}

// GetParticipants returns copies of all participants
func (h *Hub) GetParticipants() []Participant {
	h.mu.RLock()
	defer h.mu.RUnlock()

	participants := make([]Participant, 0, len(h.participants))
	for _, p := range h.participants {
		participants = append(participants, *p)
	}
	return participants
}

// SubmitResponse submits a response from a participant for the current round
func (h *Hub) SubmitResponse(sessionID, participantID, content string) (*Response, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	session, ok := h.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}

	if _, ok := h.participants[participantID]; !ok {
		return nil, fmt.Errorf("participant %q not found", participantID)
	}

	key := roundKey(sessionID, session.Round)
	round, ok := h.rounds[key]
	if !ok || round.Status != RoundStatusActive {
		return nil, errors.New("no active round")
	}

	r := Response{
		ID:            generateID(),
		SessionID:     sessionID,
		Round:         session.Round,
		ParticipantID: participantID,
		Content:       content,
		CreatedAt:     time.Now(),
	}

	h.responses[sessionID] = append(h.responses[sessionID], r)
	round.Responses = append(round.Responses, r)

	return &r, nil
}

// GetTranscript returns all responses for a session (alias for GetResponses)
func (h *Hub) GetTranscript(sessionID string) []Response {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if responses, ok := h.responses[sessionID]; ok {
		return responses
	}
	return []Response{}
}

// GetResponses returns all responses for a session
func (h *Hub) GetResponses(sessionID string) []Response {
	return h.GetTranscript(sessionID)
}

// StartRound starts a new round for a session
func (h *Hub) StartRound(sessionID string) (*Round, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	session, ok := h.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}

	if session.Status != SessionStatusActive {
		return nil, errors.New("session not active")
	}

	key := roundKey(sessionID, session.Round)
	if round, ok := h.rounds[key]; ok && round.Status == RoundStatusActive {
		return nil, errors.New("round already active")
	}

	// If round doesn't exist or is pending, start it
	if round, ok := h.rounds[key]; ok && round.Status == RoundStatusPending {
		round.Status = RoundStatusActive
		round.StartedAt = time.Now()
		return round, nil
	}

	// Create first round (round 1) if session.Round is 0
	if session.Round == 0 {
		session.Round = 1
		key = roundKey(sessionID, session.Round)
	}

	round := &Round{
		Number:    session.Round,
		SessionID: sessionID,
		Status:    RoundStatusActive,
		StartedAt: time.Now(),
		Responses: []Response{},
	}

	h.rounds[key] = round
	return round, nil
}

// AdvanceRound advances the current round to completed and prepares the next
func (h *Hub) AdvanceRound(sessionID string) (*Round, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	session, ok := h.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}

	key := roundKey(sessionID, session.Round)
	round, ok := h.rounds[key]
	if !ok {
		return nil, errors.New("no active round")
	}

	now := time.Now()
	round.Status = RoundStatusCompleted
	round.CompletedAt = &now

	// Create next round as pending
	session.Round++
	nextKey := roundKey(sessionID, session.Round)
	nextRound := &Round{
		Number:    session.Round,
		SessionID: sessionID,
		Status:    RoundStatusPending,
		StartedAt: time.Time{},
		Responses: []Response{},
	}
	h.rounds[nextKey] = nextRound

	return round, nil
}

// CreateSession creates a new session
func (h *Hub) CreateSession(id string) *Session {
	h.mu.Lock()
	defer h.mu.Unlock()

	s := &Session{
		ID:        id,
		CreatedAt: time.Now(),
		Round:     0,
		Status:    SessionStatusActive,
	}
	h.sessions[id] = s
	return s
}

// GetSession returns a copy of a session by ID
func (h *Hub) GetSession(id string) (Session, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sessions[id]
	if !ok {
		return Session{}, false
	}
	return *s, true
}

// generateID generates a unique ID using atomic counter
func generateID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), atomic.AddUint64(&idCounter, 1))
}