package sshserver

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"jakofranko/haive-mind/hub"
)

type stubModel struct{}

func (stubModel) Init() tea.Cmd                       { return nil }
func (stubModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return stubModel{}, nil }
func (stubModel) View() string                        { return "" }
func (stubModel) Close()                              {}

func newTestSession(sshUsername string) *trackedSession {
	return &trackedSession{
		program:  tea.NewProgram(stubModel{}),
		model:    stubModel{},
		username: sshUsername,
		id:       hub.ParticipantID(sshUsername),
	}
}

func registered(h hub.Hub, id string) bool {
	for _, p := range h.GetParticipants() {
		if p.ID == id {
			return true
		}
	}
	return false
}

func tracked(m *sessionManager, id string) *trackedSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.participantSessions[id]
}

func newTestManager(t *testing.T) (*sessionManager, hub.Hub) {
	h := hub.NewHub()
	t.Cleanup(func() { h.Stop() })
	return &sessionManager{hub: h, participantSessions: map[string]*trackedSession{}}, h
}

// waitUntil polls for an end-of-session effect that happens on another goroutine.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The first of two sessions sharing an SSH username ends while the second is
// live: the participant stays registered and the second stays tracked.
func TestStaleSessionEndingKeepsNewerSessionForSameUsername(t *testing.T) {
	m, h := newTestManager(t)
	id := hub.ParticipantID("alice")
	ctx1, end1 := context.WithCancel(context.Background())
	ctx2, end2 := context.WithCancel(context.Background())
	defer end2()
	first, second := newTestSession("alice"), newTestSession("alice")

	m.admit(ctx1, first)
	m.admit(ctx2, second) // reconnect while the first is still tracked
	if !registered(h, id) || tracked(m, id) != second {
		t.Fatal("after the reconnect the participant should be listed and the second session tracked")
	}

	end1()
	// The first session's cleanup is asynchronous; give it time to (wrongly) act.
	time.Sleep(200 * time.Millisecond)

	if !registered(h, id) {
		t.Error("participant was unregistered by the stale session ending")
	}
	if tracked(m, id) != second {
		t.Error("the live session is no longer tracked")
	}
}

// A session that is still the current one for its id unregisters it when it ends.
func TestCurrentSessionEndingUnregistersParticipant(t *testing.T) {
	m, h := newTestManager(t)
	id := hub.ParticipantID("alice")
	ctx, end := context.WithCancel(context.Background())

	m.admit(ctx, newTestSession("alice"))
	end()

	waitUntil(t, "participant to be unregistered", func() bool { return !registered(h, id) })
	if tracked(m, id) != nil {
		t.Error("ended session is still tracked")
	}
}

type closeCountingModel struct {
	stubModel
	closes *atomic.Int32
}

func (c closeCountingModel) Close() { c.closes.Add(1) }

func newCountingSession(sshUsername string) (*trackedSession, *atomic.Int32) {
	closes := &atomic.Int32{}
	s := newTestSession(sshUsername)
	s.model = closeCountingModel{closes: closes}
	return s, closes
}

// A reconnect replaces the tracked session for its id: the replaced session's
// model is closed exactly once (even after its own connection ends), and the
// new session stays tracked and registered.
func TestReplacedSessionIsClosedOnce(t *testing.T) {
	m, h := newTestManager(t)
	id := hub.ParticipantID("alice")
	ctx1, end1 := context.WithCancel(context.Background())
	ctx2, end2 := context.WithCancel(context.Background())
	defer end2()
	first, firstCloses := newCountingSession("alice")
	second, secondCloses := newCountingSession("alice")

	m.admit(ctx1, first)
	m.admit(ctx2, second)

	if got := firstCloses.Load(); got != 1 {
		t.Fatalf("replaced session closed %d times on replacement, want 1", got)
	}
	end1() // the old connection ends on its own afterwards
	time.Sleep(200 * time.Millisecond)

	if got := firstCloses.Load(); got != 1 {
		t.Errorf("replaced session closed %d times in total, want 1", got)
	}
	if secondCloses.Load() != 0 {
		t.Error("the new session was closed")
	}
	if tracked(m, id) != second || !registered(h, id) {
		t.Error("the new session should stay tracked and registered")
	}
}
