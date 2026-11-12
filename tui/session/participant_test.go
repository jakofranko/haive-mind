package session_test

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jakofranko/haive-mind/coordinator"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/jakofranko/haive-mind/tui/session"
)

// coordinatedHub is a started hub with the real Coordinator beside it, so
// Round state and Transcript snapshots arrive in the order the server sends.
func coordinatedHub(t *testing.T) hub.Hub {
	t.Helper()
	h := startedHub(t)
	if err := coordinator.New(h).Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return h
}

// eventually fails the test unless cond comes true in time.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// newParticipant is the participant's end of a session, subscribed and ready.
func newParticipant(t *testing.T, h hub.Hub, id string) *session.Participant {
	t.Helper()
	p := session.NewParticipant(h, id)
	t.Cleanup(p.Close)
	select {
	case <-p.Ready():
	case <-time.After(wait):
		t.Fatal("participant client never became ready")
	}
	return p
}

// connect is what the SSH server does for a participant: the session's client
// subscribes, then the participant is registered.
func connect(t *testing.T, h hub.Hub, id string) *session.Participant {
	t.Helper()
	p := newParticipant(t, h, id)
	h.RegisterParticipant(hub.NewParticipantEvent(id, id, hub.StatusIdle, hub.ParticipantEventConnect))
	return p
}

// startRound is the host ending round ending, and waits until the Coordinator
// has started the next one.
func startRound(t *testing.T, h hub.Hub, ending int) {
	t.Helper()
	began := make(chan struct{}, 1)
	sub, err := h.Subscribe(context.Background(), hub.MessageTypeRoundChange, func(m *hub.Message) error {
		if m.Round == ending+1 {
			select {
			case began <- struct{}{}:
			default:
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()
	h.StartRound(ending)
	select {
	case <-began:
	case <-time.After(wait):
		t.Fatalf("round %d never began", ending+1)
	}
}

// expect returns the next event, failing unless it is a T.
func expect[T session.Event](t *testing.T, c eventSource) T {
	t.Helper()
	ev := next(t, c)
	got, ok := ev.(T)
	if !ok {
		var want T
		t.Fatalf("next event is %T %+v, want %T", ev, ev, want)
	}
	return got
}

// until skips events until one is a T, and returns it.
func until[T session.Event](t *testing.T, c eventSource) T {
	t.Helper()
	for {
		if got, ok := next(t, c).(T); ok {
			return got
		}
	}
}

func contents(ms []hub.Message) []string {
	out := []string{}
	for _, m := range ms {
		out = append(out, m.Content)
	}
	return out
}

func status(t *testing.T, h hub.Hub, id string) hub.ParticipantStatus {
	t.Helper()
	for _, p := range h.GetParticipants() {
		if p.ID == id {
			return p.Status
		}
	}
	t.Fatalf("%s is not registered", id)
	return ""
}

// Nothing reaches the TUI before Catch-up, and Catch-up says where things
// stand: the earlier conversation, the round, and who is connected.
func TestParticipantCatchUp_IsTheFirstEventAndSaysWhereThingsStand(t *testing.T) {
	h := coordinatedHub(t)
	h.RegisterParticipant(hub.NewParticipantEvent("host", "Host", hub.StatusIdle, hub.ParticipantEventConnect))
	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeSystem, "hub", "pick a database", 1))
	startRound(t, h, 1)

	p := connect(t, h, me)

	ev := expect[session.CatchUpEvent](t, p)
	if !reflect.DeepEqual(contents(ev.Messages), []string{"pick a database"}) {
		t.Errorf("Messages = %q, want the earlier directive only", contents(ev.Messages))
	}
	if ev.Round != 2 {
		t.Errorf("Round = %d, want 2", ev.Round)
	}
	ids := map[string]bool{}
	for _, part := range ev.Participants {
		ids[part.ID] = true
	}
	if !ids[me] || !ids["host"] {
		t.Errorf("Participants = %+v, want me and the host", ev.Participants)
	}
}

// Live messages that arrive before the snapshot are held and come after the
// snapshot's older ones, each once.
func TestParticipantCatchUp_HoldsLiveMessagesAndShowsEachOnce(t *testing.T) {
	h := startedHub(t)
	p := newParticipant(t, h, me)
	ctx := context.Background()
	older, live := msg(hub.MessageTypeDirective, "older"), msg(hub.MessageTypeDirective, "live")

	h.Send(ctx, live)
	h.Send(ctx, hub.NewTranscriptSnapshot(1, me, []*hub.Message{older, live}, nil))

	ev := expect[session.CatchUpEvent](t, p)
	if !reflect.DeepEqual(contents(ev.Messages), []string{"older", "live"}) {
		t.Errorf("Messages = %q, want older then live", contents(ev.Messages))
	}
}

// Another session's snapshot is not this session's Catch-up.
func TestParticipantCatchUp_IgnoresSomeoneElsesSnapshot(t *testing.T) {
	h := startedHub(t)
	p := newParticipant(t, h, me)
	ctx := context.Background()
	secret := hub.NewMessage(hub.MessageTypeResponse, "Bob", "bobs answer", 1)

	h.Send(ctx, hub.NewTranscriptSnapshot(1, "Agent-bob", []*hub.Message{secret}, secret))
	h.Send(ctx, hub.NewTranscriptSnapshot(1, me, nil, nil))
	h.Send(ctx, hub.NewRoundState(1, []string{me}))

	if ev := expect[session.CatchUpEvent](t, p); len(ev.Messages) != 0 {
		t.Errorf("Messages = %q, want none", contents(ev.Messages))
	}
	expect[session.RoundEvent](t, p)
	if rev := expect[session.RevisionEvent](t, p); rev.LastAnswer != "" {
		t.Errorf("LastAnswer = %q, want none: Bob's answer is not mine", rev.LastAnswer)
	}
}

// A flagged participant gets their answer back once, whichever of Round state
// and the Transcript snapshot reaches the session first.
func TestParticipantRevision_EitherArrivalOrder(t *testing.T) {
	last := hub.NewMessage(hub.MessageTypeResponse, "Ann", "use postgres", 1)
	last.FromID = me
	snapshot := func() *hub.Message { return hub.NewTranscriptSnapshot(1, me, nil, last) }
	flagged := func() *hub.Message { return hub.NewRoundState(1, []string{me}) }

	for name, order := range map[string][]func() *hub.Message{
		"round state first": {flagged, snapshot},
		"snapshot first":    {snapshot, flagged},
	} {
		t.Run(name, func(t *testing.T) {
			h := startedHub(t)
			p := newParticipant(t, h, me)
			for _, m := range order {
				h.Send(context.Background(), m())
			}

			expect[session.CatchUpEvent](t, p)
			rev := until[session.RevisionEvent](t, p)
			if rev.LastAnswer != "use postgres" {
				t.Errorf("LastAnswer = %q, want the last answer", rev.LastAnswer)
			}

			// Hearing the same flag again does not ask again.
			h.Send(context.Background(), flagged())
			h.Send(context.Background(), msg(hub.MessageTypeSystem, "marker"))
			for {
				switch ev := next(t, p).(type) {
				case session.RevisionEvent:
					t.Fatal("asked to revise a second time for the same flag")
				case session.MessageEvent:
					if ev.Message.Content == "marker" {
						return
					}
				}
			}
		})
	}
}

// The real server order: a participant who reconnects while flagged is asked
// once, with the answer they sent before the connection dropped.
func TestParticipantRevision_ReconnectingWhileFlagged(t *testing.T) {
	h := coordinatedHub(t)
	first := connect(t, h, me)
	expect[session.CatchUpEvent](t, first)
	first.Submit("use postgres")
	h.Send(context.Background(), hub.NewFlag(1, me))
	until[session.RevisionEvent](t, first)
	first.Close()
	h.UnregisterParticipant(me)

	second := connect(t, h, me)

	expect[session.CatchUpEvent](t, second)
	if rev := expect[session.RevisionEvent](t, second); rev.LastAnswer != "use postgres" {
		t.Errorf("LastAnswer = %q, want the answer sent before reconnecting", rev.LastAnswer)
	}
}

// An answer from an earlier round is never offered back as this round's.
func TestParticipantRevision_DoesNotOfferAnEarlierRoundsAnswer(t *testing.T) {
	h := coordinatedHub(t)
	p := connect(t, h, me)
	expect[session.CatchUpEvent](t, p)
	p.Submit("use postgres")

	h.StartRound(1)
	for ev := until[session.RoundEvent](t, p); ev.Round != 2; ev = until[session.RoundEvent](t, p) {
	}
	h.Send(context.Background(), hub.NewFlag(2, me))

	if rev := until[session.RevisionEvent](t, p); rev.LastAnswer != "" {
		t.Errorf("LastAnswer = %q, want none in round 2", rev.LastAnswer)
	}
}

// #74: an answer sent while the snapshot was on its way is not lost to a
// snapshot that has no answer in it.
func TestParticipantRevision_AnswerSentBeforeCatchUpIsKept(t *testing.T) {
	h := startedHub(t)
	p := newParticipant(t, h, me)

	p.Submit("use postgres")
	h.Send(context.Background(), hub.NewTranscriptSnapshot(1, me, nil, nil))
	h.Send(context.Background(), hub.NewRoundState(1, []string{me}))

	if rev := until[session.RevisionEvent](t, p); rev.LastAnswer != "use postgres" {
		t.Errorf("LastAnswer = %q, want the answer sent before Catch-up", rev.LastAnswer)
	}
}

// statusCounter counts what the client reports, to show it reports each
// change once.
type statusCounter struct {
	hub.Hub
	mu      sync.Mutex
	reports []hub.ParticipantStatus
}

func (s *statusCounter) UpdateParticipantStatus(id string, st hub.ParticipantStatus) error {
	s.mu.Lock()
	s.reports = append(s.reports, st)
	s.mu.Unlock()
	return s.Hub.UpdateParticipantStatus(id, st)
}

func (s *statusCounter) got() []hub.ParticipantStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]hub.ParticipantStatus{}, s.reports...)
}

// The client reports what the participant is doing, once per change: typing
// while the box has text, submitted after sending, idle when a new round
// starts or the host asks for a revision.
func TestParticipantStatus_FollowsTheRoundOncePerChange(t *testing.T) {
	inner := coordinatedHub(t)
	h := &statusCounter{Hub: inner}
	p := connect(t, h, me)
	expect[session.CatchUpEvent](t, p)

	p.Typed("u")
	p.Typed("us")
	p.Typed("use postgres")
	if got := status(t, h, me); got != hub.StatusTyping {
		t.Errorf("while typing: %q, want typing", got)
	}
	p.Submit("use postgres")
	if got := status(t, h, me); got != hub.StatusSubmitted {
		t.Errorf("after submitting: %q, want submitted", got)
	}
	h.Send(context.Background(), hub.NewFlag(1, me))
	until[session.RevisionEvent](t, p)
	if got := status(t, h, me); got != hub.StatusIdle {
		t.Errorf("after being flagged: %q, want idle", got)
	}
	p.Typed("sqlite")
	h.StartRound(1)
	// Round state for round 2 comes before the Round change, so wait for the
	// status rather than the first round-2 event.
	eventually(t, "idle after a new round", func() bool { return status(t, h, me) == hub.StatusIdle })

	want := []hub.ParticipantStatus{hub.StatusTyping, hub.StatusSubmitted, hub.StatusIdle, hub.StatusTyping, hub.StatusIdle}
	if got := h.got(); !reflect.DeepEqual(got, want) {
		t.Errorf("reported %v, want %v", got, want)
	}
}

// Emptying the box puts a typing participant back to idle; a submitted one
// stays submitted.
func TestParticipantStatus_EmptyingTheBox(t *testing.T) {
	h := coordinatedHub(t)
	p := connect(t, h, me)
	expect[session.CatchUpEvent](t, p)

	p.Typed("a")
	p.Typed("")
	if got := status(t, h, me); got != hub.StatusIdle {
		t.Errorf("after emptying the box: %q, want idle", got)
	}
	p.Typed("a")
	p.Submit("a")
	p.Typed("")
	if got := status(t, h, me); got != hub.StatusSubmitted {
		t.Errorf("after submitting then an empty box: %q, want submitted", got)
	}
}

// A response sent through the client carries who sent it and the round it
// answers; the caller supplies neither.
func TestParticipantSubmit_StampsIdentityAndRound(t *testing.T) {
	h := coordinatedHub(t)
	var sent atomic.Pointer[hub.Message]
	if _, err := h.Subscribe(context.Background(), hub.MessageTypeResponse, func(m *hub.Message) error {
		sent.Store(m)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	startRound(t, h, 1)
	p := connect(t, h, me)
	if ev := expect[session.CatchUpEvent](t, p); ev.Round != 2 {
		t.Fatalf("caught up in round %d, want 2", ev.Round)
	}

	p.Rename("Ann")
	if sentOK := p.Submit("  use postgres  "); !sentOK {
		t.Fatal("Submit reported nothing sent")
	}
	if p.Submit("   ") {
		t.Error("Submit sent a blank response")
	}

	deadline := time.Now().Add(wait)
	for sent.Load() == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	m := sent.Load()
	if m == nil {
		t.Fatal("no response reached the hub")
	}
	if m.FromID != me || m.From != "Ann" || m.Round != 2 || m.Content != "use postgres" {
		t.Errorf("sent {FromID:%q From:%q Round:%d Content:%q}, want {%s Ann 2 use postgres}", m.FromID, m.From, m.Round, m.Content, me)
	}
	if got := h.GetDraft(me); got != "" {
		t.Errorf("unsent text after submitting = %q, want none", got)
	}
}

// Unsent text is stored as it is typed, so a new session picks it up.
func TestParticipantTyped_KeepsUnsentTextForTheNextSession(t *testing.T) {
	h := startedHub(t)
	newParticipant(t, h, me).Typed("use postgres")

	if got := newParticipant(t, h, me).UnsentText(); got != "use postgres" {
		t.Errorf("next session's unsent text = %q, want the earlier text", got)
	}
}
