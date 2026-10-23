package hub

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewHub(t *testing.T) {
	h := NewHub()
	if h == nil {
		t.Fatal("NewHub() returned nil")
	}
}

func TestAddParticipant(t *testing.T) {
	h := NewHub()

	// Test adding a new participant
	p, err := h.AddParticipant("p1", "Alice")
	if err != nil {
		t.Fatalf("AddParticipant() error = %v", err)
	}
	if p.ID != "p1" || p.Name != "Alice" || !p.IsActive {
		t.Errorf("Participant = %+v, want ID=p1, Name=Alice, IsActive=true", p)
	}
	if p.JoinedAt.IsZero() {
		t.Error("JoinedAt should be set")
	}

	// Test adding duplicate participant
	_, err = h.AddParticipant("p1", "Bob")
	if err == nil {
		t.Error("AddParticipant() should error for duplicate ID")
	}
}

func TestGetParticipant(t *testing.T) {
	h := NewHub()
	h.AddParticipant("p1", "Alice")

	// Test existing participant
	p, ok := h.GetParticipant("p1")
	if !ok {
		t.Fatal("GetParticipant() should return true for existing participant")
	}
	if p.Name != "Alice" {
		t.Errorf("Participant name = %s, want Alice", p.Name)
	}

	// Test non-existing participant
	_, ok = h.GetParticipant("p2")
	if ok {
		t.Error("GetParticipant() should return false for non-existing participant")
	}
}

func TestGetParticipants(t *testing.T) {
	h := NewHub()
	h.AddParticipant("p1", "Alice")
	h.AddParticipant("p2", "Bob")

	participants := h.GetParticipants()
	if len(participants) != 2 {
		t.Errorf("GetParticipants() returned %d participants, want 2", len(participants))
	}
}

func TestCreateSession(t *testing.T) {
	h := NewHub()

	s := h.CreateSession("s1")
	if s == nil {
		t.Fatal("CreateSession() returned nil")
	}
	if s.ID != "s1" || s.Round != 0 || s.Status != SessionStatusActive {
		t.Errorf("Session = %+v, want ID=s1, Round=0, Status=active", s)
	}
	if s.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestGetSession(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")

	// Test existing session
	s, ok := h.GetSession("s1")
	if !ok {
		t.Fatal("GetSession() should return true for existing session")
	}
	if s.ID != "s1" {
		t.Errorf("Session ID = %s, want s1", s.ID)
	}

	// Test non-existing session
	_, ok = h.GetSession("s2")
	if ok {
		t.Error("GetSession() should return false for non-existing session")
	}
}

func TestStartRound(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")

	// Test starting first round
	round, err := h.StartRound("s1")
	if err != nil {
		t.Fatalf("StartRound() error = %v", err)
	}
	if round.Number != 1 || round.Status != RoundStatusActive {
		t.Errorf("Round = %+v, want Number=1, Status=active", round)
	}
	if round.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}

	// Test starting round on non-existing session
	_, err = h.StartRound("s2")
	if err == nil {
		t.Error("StartRound() should error for non-existing session")
	}

	// Test starting round when one is already active
	_, err = h.StartRound("s1")
	if err == nil {
		t.Error("StartRound() should error when round already active")
	}
}

func TestSubmitResponse(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	h.StartRound("s1")

	// Test submitting a response
	r, err := h.SubmitResponse("s1", "p1", "Hello world")
	if err != nil {
		t.Fatalf("SubmitResponse() error = %v", err)
	}
	if r.SessionID != "s1" || r.ParticipantID != "p1" || r.Content != "Hello world" || r.Round != 1 {
		t.Errorf("Response = %+v, want SessionID=s1, ParticipantID=p1, Content=Hello world, Round=1", r)
	}
	if r.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	// Test submitting response for non-existing session
	_, err = h.SubmitResponse("s2", "p1", "test")
	if err == nil {
		t.Error("SubmitResponse() should error for non-existing session")
	}

	// Test submitting response for non-existing participant
	_, err = h.SubmitResponse("s1", "p2", "test")
	if err == nil {
		t.Error("SubmitResponse() should error for non-existing participant")
	}

	// Test submitting response when no active round
	h2 := NewHub()
	h2.CreateSession("s3")
	h2.AddParticipant("p3", "Charlie")
	_, err = h2.SubmitResponse("s3", "p3", "test")
	if err == nil {
		t.Error("SubmitResponse() should error when no active round")
	}
}

func TestAdvanceRound(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	h.StartRound("s1")
	h.SubmitResponse("s1", "p1", "Response 1")

	// Test advancing round
	round, err := h.AdvanceRound("s1")
	if err != nil {
		t.Fatalf("AdvanceRound() error = %v", err)
	}
	if round.Status != RoundStatusCompleted || round.CompletedAt == nil {
		t.Errorf("Round = %+v, want Status=completed, CompletedAt set", round)
	}

	// Verify next round is pending
	session, _ := h.GetSession("s1")
	if session.Round != 2 {
		t.Errorf("Session round = %d, want 2", session.Round)
	}

	// Test advancing round on non-existing session
	_, err = h.AdvanceRound("s2")
	if err == nil {
		t.Error("AdvanceRound() should error for non-existing session")
	}

	// Test advancing round when no active round
	h2 := NewHub()
	h2.CreateSession("s3")
	_, err = h2.AdvanceRound("s3")
	if err == nil {
		t.Error("AdvanceRound() should error when no active round")
	}
}

func TestGetTranscript(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	h.StartRound("s1")
	h.SubmitResponse("s1", "p1", "Test response")

	// Test transcript with responses
	transcript := h.GetTranscript("s1")
	if len(transcript) != 1 {
		t.Errorf("GetTranscript() returned %d responses, want 1", len(transcript))
	}
	if transcript[0].Content != "Test response" {
		t.Errorf("Transcript content = %q, want %q", transcript[0].Content, "Test response")
	}

	// Test non-existing session
	transcript = h.GetTranscript("s2")
	if len(transcript) != 0 {
		t.Errorf("GetTranscript() for non-existing session returned %d responses, want 0", len(transcript))
	}
}

func TestGetResponses(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	h.StartRound("s1")
	h.SubmitResponse("s1", "p1", "Test response")

	responses := h.GetResponses("s1")
	if len(responses) != 1 {
		t.Errorf("GetResponses() returned %d responses, want 1", len(responses))
	}
}

func TestFullRoundCycle(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	h.AddParticipant("p2", "Bob")

	// Start round 1
	round1, err := h.StartRound("s1")
	if err != nil {
		t.Fatalf("StartRound() error = %v", err)
	}
	if round1.Number != 1 {
		t.Errorf("Round number = %d, want 1", round1.Number)
	}

	// Submit responses
	_, err = h.SubmitResponse("s1", "p1", "Alice's response")
	if err != nil {
		t.Fatalf("SubmitResponse() error = %v", err)
	}
	_, err = h.SubmitResponse("s1", "p2", "Bob's response")
	if err != nil {
		t.Fatalf("SubmitResponse() error = %v", err)
	}

	// Advance to round 2
	round1, err = h.AdvanceRound("s1")
	if err != nil {
		t.Fatalf("AdvanceRound() error = %v", err)
	}
	if round1.Status != RoundStatusCompleted {
		t.Errorf("Round status = %s, want completed", round1.Status)
	}

	// Start round 2
	round2, err := h.StartRound("s1")
	if err != nil {
		t.Fatalf("StartRound() error = %v", err)
	}
	if round2.Number != 2 {
		t.Errorf("Round number = %d, want 2", round2.Number)
	}
	if round2.Status != RoundStatusActive {
		t.Errorf("Round status = %s, want active", round2.Status)
	}

	// Verify participants
	participants := h.GetParticipants()
	if len(participants) != 2 {
		t.Errorf("Participant count = %d, want 2", len(participants))
	}
}

func TestConcurrentAccess(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")

	done := make(chan bool, 100)

	// Concurrent participants
	for i := 0; i < 50; i++ {
		go func(id string) {
			h.AddParticipant(id, "User"+id)
			done <- true
		}(string(rune('a' + i)))
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		go func() {
			h.GetParticipants()
			h.GetParticipant("p1")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	participants := h.GetParticipants()
	if len(participants) < 1 {
		t.Errorf("Expected at least 1 participant, got %d", len(participants))
	}
}

func TestSessionStatusTransitions(t *testing.T) {
	h := NewHub()
	s := h.CreateSession("s1")

	if s.Status != SessionStatusActive {
		t.Errorf("Initial session status = %s, want active", s.Status)
	}

	// Test that we can create multiple sessions
	s2 := h.CreateSession("s2")
	if s2.ID != "s2" {
		t.Errorf("Second session ID = %s, want s2", s2.ID)
	}
	if s2.Status != SessionStatusActive {
		t.Errorf("Second session status = %s, want active", s2.Status)
	}
}

func TestRoundStatusTransitions(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")

	// Round should not exist before StartRound
	if _, ok := h.rounds["s1:1"]; ok {
		t.Error("Round should not exist before StartRound")
	}

	round, _ := h.StartRound("s1")
	if round.Status != RoundStatusActive {
		t.Errorf("Round status after StartRound = %s, want active", round.Status)
	}

	h.AdvanceRound("s1")
	if round.Status != RoundStatusCompleted {
		t.Errorf("Round status after AdvanceRound = %s, want completed", round.Status)
	}
	if round.CompletedAt == nil {
		t.Error("CompletedAt should be set after AdvanceRound")
	}
}

func TestRoundKeyEncoding(t *testing.T) {
	// Test that round keys are correctly formatted
	h := NewHub()
	h.CreateSession("s1")
	h.StartRound("s1")

	// Round 1 should exist with key "s1:1"
	if _, ok := h.rounds["s1:1"]; !ok {
		t.Error("Round 1 should have key 's1:1'")
	}

	h.AdvanceRound("s1")
	// Round 2 should exist with key "s1:2" as pending
	if _, ok := h.rounds["s1:2"]; !ok {
		t.Error("Round 2 should have key 's1:2'")
	}
}

func TestAggregate(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	h.AddParticipant("p2", "Bob")
	h.StartRound("s1")
	h.SubmitResponse("s1", "p1", "Response A")
	h.SubmitResponse("s1", "p2", "Response B")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// opencode run will fail since it's not installed, so we expect an error
	_, err := h.Aggregate(ctx, "s1", "What is the answer?")
	if err == nil {
		t.Error("Aggregate() should error when opencode is not available")
	}
	if !strings.Contains(err.Error(), "opencode run failed") {
		t.Errorf("Aggregate() error = %v, want opencode run failed", err)
	}
}

func TestAggregateNoResponses(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.StartRound("s1")

	ctx := context.Background()
	_, err := h.Aggregate(ctx, "s1", "What is the answer?")
	if err == nil {
		t.Error("Aggregate() should error when no responses")
	}
	if !strings.Contains(err.Error(), "no responses to aggregate") {
		t.Errorf("Aggregate() error = %v, want no responses to aggregate", err)
	}
}

func TestAggregateNonExistingSession(t *testing.T) {
	h := NewHub()

	ctx := context.Background()
	_, err := h.Aggregate(ctx, "s2", "What is the answer?")
	if err == nil {
		t.Error("Aggregate() should error for non-existing session")
	}
	if !strings.Contains(err.Error(), "session") {
		t.Errorf("Aggregate() error = %v, want session not found", err)
	}
}

func TestBuildSynthesisPrompt(t *testing.T) {
	h := NewHub()
	h.AddParticipant("p1", "Alice")
	h.AddParticipant("p2", "Bob")

	responses := []Response{
		{ParticipantID: "p1", Content: "Response A"},
		{ParticipantID: "p2", Content: "Response B"},
	}

	prompt := h.buildSynthesisPrompt("What is the answer?", responses)

	if !strings.Contains(prompt, "Original question:") {
		t.Error("Prompt should contain 'Original question:'")
	}
	if !strings.Contains(prompt, "What is the answer?") {
		t.Error("Prompt should contain the question")
	}
	if !strings.Contains(prompt, "Participant responses:") {
		t.Error("Prompt should contain 'Participant responses:'")
	}
	if !strings.Contains(prompt, "Alice") {
		t.Error("Prompt should contain participant name")
	}
	if !strings.Contains(prompt, "Response A") {
		t.Error("Prompt should contain response content")
	}
	if !strings.Contains(prompt, "Instruction: Reconcile") {
		t.Error("Prompt should contain reconciliation instruction")
	}
}

func TestMarkConflict(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	h.AddParticipant("p2", "Bob")
	h.StartRound("s1")
	h.SubmitResponse("s1", "p1", "Response A")
	h.SubmitResponse("s1", "p2", "Response B")

	conflict, err := h.MarkConflict("s1", []string{"p1", "p2"})
	if err != nil {
		t.Fatalf("MarkConflict() error = %v", err)
	}
	if conflict.SessionID != "s1" {
		t.Errorf("Conflict sessionID = %s, want s1", conflict.SessionID)
	}
	if conflict.Round != 1 {
		t.Errorf("Conflict round = %d, want 1", conflict.Round)
	}
	if len(conflict.ParticipantIDs) != 2 {
		t.Errorf("Conflict participantIDs = %v, want [p1 p2]", conflict.ParticipantIDs)
	}
	if conflict.Status != ConflictStatusDetected {
		t.Errorf("Conflict status = %s, want detected", conflict.Status)
	}
	if conflict.DetectedAt.IsZero() {
		t.Error("DetectedAt should be set")
	}

	// Verify conflict is stored in round
	h.mu.RLock()
	round := h.rounds["s1:1"]
	h.mu.RUnlock()
	if round.Conflict == nil {
		t.Error("Round should have conflict")
	}
	if round.Conflict.ID != conflict.ID {
		t.Error("Round conflict ID mismatch")
	}
}

func TestMarkConflictDuplicate(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	h.AddParticipant("p2", "Bob")
	h.StartRound("s1")

	_, err := h.MarkConflict("s1", []string{"p1", "p2"})
	if err != nil {
		t.Fatalf("First MarkConflict() error = %v", err)
	}

	_, err = h.MarkConflict("s1", []string{"p1", "p2"})
	if err == nil {
		t.Error("MarkConflict() should error when conflict already exists")
	}
}

func TestMarkConflictNonExistingSession(t *testing.T) {
	h := NewHub()

	_, err := h.MarkConflict("s2", []string{"p1", "p2"})
	if err == nil {
		t.Error("MarkConflict() should error for non-existing session")
	}
}

func TestMarkConflictNoActiveRound(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	// No StartRound called

	_, err := h.MarkConflict("s1", []string{"p1"})
	if err == nil {
		t.Error("MarkConflict() should error when no active round")
	}
}

func TestResolveConflict(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	h.AddParticipant("p2", "Bob")
	h.StartRound("s1")
	h.SubmitResponse("s1", "p1", "Response A")
	h.SubmitResponse("s1", "p2", "Response B")

	_, err := h.MarkConflict("s1", []string{"p1", "p2"})
	if err != nil {
		t.Fatalf("MarkConflict() error = %v", err)
	}

	conflict, err := h.ResolveConflict("s1", "Agreed on unified response")
	if err != nil {
		t.Fatalf("ResolveConflict() error = %v", err)
	}
	if conflict.Status != ConflictStatusResolved {
		t.Errorf("Conflict status = %s, want resolved", conflict.Status)
	}
	if conflict.Resolution != "Agreed on unified response" {
		t.Errorf("Conflict resolution = %s, want 'Agreed on unified response'", conflict.Resolution)
	}
	if conflict.ResolvedAt == nil || conflict.ResolvedAt.IsZero() {
		t.Error("ResolvedAt should be set")
	}
}

func TestResolveConflictNoConflict(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	h.StartRound("s1")

	_, err := h.ResolveConflict("s1", "Resolution")
	if err == nil {
		t.Error("ResolveConflict() should error when no conflict exists")
	}
}

func TestResolveConflictNonExistingSession(t *testing.T) {
	h := NewHub()

	_, err := h.ResolveConflict("s2", "Resolution")
	if err == nil {
		t.Error("ResolveConflict() should error for non-existing session")
	}
}

func TestGetConflict(t *testing.T) {
	h := NewHub()
	h.CreateSession("s1")
	h.AddParticipant("p1", "Alice")
	h.AddParticipant("p2", "Bob")
	h.StartRound("s1")

	// No conflict yet
	conflict, ok := h.GetConflict("s1")
	if ok {
		t.Error("GetConflict() should return false when no conflict")
	}
	if conflict != nil {
		t.Error("GetConflict() should return nil when no conflict")
	}

	// Mark conflict
	_, err := h.MarkConflict("s1", []string{"p1", "p2"})
	if err != nil {
		t.Fatalf("MarkConflict() error = %v", err)
	}

	// Get conflict
	conflict, ok = h.GetConflict("s1")
	if !ok {
		t.Error("GetConflict() should return true when conflict exists")
	}
	if conflict == nil {
		t.Error("GetConflict() should return conflict")
	}
	if len(conflict.ParticipantIDs) != 2 {
		t.Errorf("Conflict participantIDs = %v, want [p1 p2]", conflict.ParticipantIDs)
	}
}