package session_test

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jakofranko/haive-mind/hub"
	"github.com/jakofranko/haive-mind/tui/session"
)

// What every Session client does, whatever its role: deliver in send order
// without drops, catch up once, and stop cleanly.

const (
	wait = 5 * time.Second
	me   = "Agent-ann" // the participant id the test clients play
)

func startedHub(t *testing.T) hub.Hub {
	t.Helper()
	h := hub.NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })
	return h
}

func msg(mt hub.MessageType, content string) *hub.Message {
	return hub.NewMessage(mt, "x", content, 1)
}

// eventSource is any Session client: something that hands out events.
type eventSource interface {
	Next() (session.Event, bool)
}

// next returns the client's next event, failing the test if none arrives.
func next(t *testing.T, c eventSource) session.Event {
	t.Helper()
	type result struct {
		ev session.Event
		ok bool
	}
	res := make(chan result, 1)
	go func() {
		ev, ok := c.Next()
		res <- result{ev, ok}
	}()
	select {
	case r := <-res:
		if !r.ok {
			t.Fatal("Next reported closed, want an event")
		}
		return r.ev
	case <-time.After(wait):
		t.Fatal("no event arrived")
		return nil
	}
}

// caughtUp is a participant's client that has had its (empty) Transcript
// snapshot, so live messages now reach it.
func caughtUp(t *testing.T, h hub.Hub) *session.Participant {
	t.Helper()
	p := newParticipant(t, h, me)
	h.Send(context.Background(), hub.NewTranscriptSnapshot(1, me, nil, nil))
	expect[session.CatchUpEvent](t, p)
	return p
}

func TestMessagesArriveAsTypedEventsInSendOrder(t *testing.T) {
	h := startedHub(t)
	c := caughtUp(t, h)

	ctx := context.Background()
	types := []hub.MessageType{
		hub.MessageTypeDirective, hub.MessageTypeResponse, hub.MessageTypeAgentReply,
		hub.MessageTypeSystem, hub.MessageTypeAggregate, hub.MessageTypeAggregateDraft,
	}
	for i, mt := range types {
		h.Send(ctx, hub.NewMessage(mt, "x", fmt.Sprintf("%d", i), 1))
	}
	for i, mt := range types {
		ev := expect[session.MessageEvent](t, c)
		if ev.Message.Type != mt || ev.Message.Content != fmt.Sprintf("%d", i) {
			t.Fatalf("event %d = %s %q, want %s %d", i, ev.Message.Type, ev.Message.Content, mt, i)
		}
	}
}

func TestBurstIsDeliveredWithoutDrops(t *testing.T) {
	h := startedHub(t)
	c := caughtUp(t, h)

	const n = 300
	ctx := context.Background()
	for i := 0; i < n; i++ { // nothing is reading yet
		h.Send(ctx, hub.NewMessage(hub.MessageTypeSystem, "hub", fmt.Sprintf("%03d", i), 1))
	}
	for i := 0; i < n; i++ {
		ev := expect[session.MessageEvent](t, c)
		if want := fmt.Sprintf("%03d", i); ev.Message.Content != want {
			t.Fatalf("event %d = %s, want %s", i, ev.Message.Content, want)
		}
	}
}

func TestParticipantEventsAreDecoded(t *testing.T) {
	h := startedHub(t)
	c := caughtUp(t, h)

	h.RegisterParticipant(hub.NewParticipantEvent("Agent-alice", "Alice", hub.StatusIdle, hub.ParticipantEventConnect))
	connect := expect[session.ParticipantEvent](t, c).Participant
	if connect.ID != "Agent-alice" || connect.DisplayName != "Alice" || connect.Event != hub.ParticipantEventConnect {
		t.Errorf("connect = %+v", connect)
	}

	h.UpdateParticipantStatus("Agent-alice", hub.StatusTyping)
	change := expect[session.ParticipantEvent](t, c).Participant
	if change.ID != "Agent-alice" || change.Status != hub.StatusTyping {
		t.Errorf("status change = %+v", change)
	}

	// The hub forgets a participant on disconnect, so this cannot be a lookup.
	h.UnregisterParticipant("Agent-alice")
	gone := expect[session.ParticipantEvent](t, c).Participant
	if gone.ID != "Agent-alice" || gone.Status != hub.StatusDisconnected || gone.Event != hub.ParticipantEventDisconnect {
		t.Errorf("disconnect = %+v", gone)
	}
}

// Round state and Round change both arrive as a RoundEvent, in send order
// with everything else.
func TestRoundEventsKeepSendOrder(t *testing.T) {
	h := startedHub(t)
	c := caughtUp(t, h)
	ctx := context.Background()

	h.Send(ctx, msg(hub.MessageTypeSystem, "before"))
	h.Send(ctx, hub.NewRoundState(2, []string{"Agent-a", "Agent-b"}))
	h.Send(ctx, hub.NewMessage(hub.MessageTypeRoundChange, "hub", "", 3))

	expect[session.MessageEvent](t, c)
	if ev := expect[session.RoundEvent](t, c); ev.Round != 2 || !reflect.DeepEqual(ev.Flagged, []string{"Agent-a", "Agent-b"}) {
		t.Errorf("Round state = %+v, want round 2 with both flagged", ev)
	}
	if ev := expect[session.RoundEvent](t, c); ev.Round != 3 || len(ev.Flagged) != 0 {
		t.Errorf("Round change = %+v, want round 3 with no one flagged", ev)
	}
}

// Agent failures and finished exports are the host's: a participant never
// hears of them.
func TestHostOnlyMessagesDoNotReachAParticipant(t *testing.T) {
	h := startedHub(t)
	c := caughtUp(t, h)
	ctx := context.Background()

	h.Send(ctx, hub.NewMessage(hub.MessageTypeExportComplete, "hub", "saved", 1))
	h.Send(ctx, hub.NewMessage(hub.MessageTypeAgentError, "hub", "boom", 1))
	h.Send(ctx, msg(hub.MessageTypeSystem, "after"))

	if ev := expect[session.MessageEvent](t, c); ev.Message.Content != "after" {
		t.Errorf("first event = %q, want the system message", ev.Message.Content)
	}
}

// The coordinator records round changes and exports for the export file, but
// no session shows them live, so Catch-up does not show them either (#76).
func TestCatchUpShowsOnlyWhatALiveSessionShows(t *testing.T) {
	h := startedHub(t)
	c := newParticipant(t, h, me)

	d, r := msg(hub.MessageTypeDirective, "q"), msg(hub.MessageTypeResponse, "a")
	recorded := []*hub.Message{
		d,
		msg(hub.MessageTypeNewRound, ""),
		msg(hub.MessageTypeRoundChange, ""),
		msg(hub.MessageTypeExport, "transcript.md"),
		msg(hub.MessageTypeExportComplete, "/tmp/transcript.md"),
		r,
	}
	h.Send(context.Background(), hub.NewTranscriptSnapshot(1, me, recorded, nil))

	if got := contents(expect[session.CatchUpEvent](t, c).Messages); !reflect.DeepEqual(got, []string{"q", "a"}) {
		t.Errorf("Messages = %q, want only the directive then the response", got)
	}
}

// A live message the session already has from Catch-up is dropped; one it
// does not have is kept.
func TestLiveMessageAlreadyCaughtUpIsDropped(t *testing.T) {
	h := startedHub(t)
	c := newParticipant(t, h, me)
	ctx := context.Background()
	have, fresh := msg(hub.MessageTypeDirective, "have"), msg(hub.MessageTypeDirective, "fresh")

	h.Send(ctx, hub.NewTranscriptSnapshot(1, me, []*hub.Message{have}, nil))
	expect[session.CatchUpEvent](t, c)
	h.Send(ctx, have)
	h.Send(ctx, fresh)

	if ev := expect[session.MessageEvent](t, c); ev.Message.ID != fresh.ID {
		t.Errorf("got %q, want only the fresh message", ev.Message.Content)
	}
}

func TestCloseReleasesAPendingNext(t *testing.T) {
	h := startedHub(t)
	c := newParticipant(t, h, me)

	released := make(chan bool, 1)
	go func() {
		_, ok := c.Next()
		released <- ok
	}()
	time.Sleep(50 * time.Millisecond) // let Next block
	c.Close()

	select {
	case ok := <-released:
		if ok {
			t.Error("Next returned ok=true after Close")
		}
	case <-time.After(wait):
		t.Fatal("Close did not release the pending Next")
	}
}

func TestSecondCloseIsHarmless(t *testing.T) {
	h := startedHub(t)
	c := newParticipant(t, h, me)
	c.Close()
	c.Close()
	if _, ok := c.Next(); ok {
		t.Error("Next after Close returned an event")
	}
}

// countingHub embeds a real hub and counts subscribes and unsubscribes.
type countingHub struct {
	hub.Hub
	subs, unsubs atomic.Int32
}

type countedSub struct {
	hub.Subscription
	h *countingHub
}

func (c *countedSub) Unsubscribe() { c.h.unsubs.Add(1); c.Subscription.Unsubscribe() }

func (c *countingHub) Subscribe(ctx context.Context, t hub.MessageType, fn func(*hub.Message) error) (hub.Subscription, error) {
	return c.SubscribeTypes(ctx, []hub.MessageType{t}, fn)
}

func (c *countingHub) SubscribeTypes(ctx context.Context, ts []hub.MessageType, fn func(*hub.Message) error) (hub.Subscription, error) {
	s, err := c.Hub.SubscribeTypes(ctx, ts, fn)
	c.subs.Add(1)
	return &countedSub{Subscription: s, h: c}, err
}

func TestCloseUnsubscribesEverything(t *testing.T) {
	for name, open := range map[string]func(hub.Hub) interface{ Close() }{
		"participant": func(h hub.Hub) interface{ Close() } { return session.NewParticipant(h, me) },
		"host":        func(h hub.Hub) interface{ Close() } { return session.NewHost(h) },
	} {
		t.Run(name, func(t *testing.T) {
			ch := &countingHub{Hub: startedHub(t)}
			c := open(ch)
			if ch.subs.Load() == 0 {
				t.Fatal("client made no subscriptions; test is not exercising anything")
			}
			c.Close()
			if got, want := ch.unsubs.Load(), ch.subs.Load(); got != want {
				t.Errorf("unsubscribed %d of %d subscriptions", got, want)
			}
		})
	}
}

// failingHub embeds a real hub whose SubscribeTypes always fails.
type failingHub struct{ hub.Hub }

func (failingHub) SubscribeTypes(context.Context, []hub.MessageType, func(*hub.Message) error) (hub.Subscription, error) {
	return nil, fmt.Errorf("subscribe refused")
}

func TestFailedSubscriptionClosesTheClient(t *testing.T) {
	c := session.NewParticipant(failingHub{Hub: startedHub(t)}, me)
	defer c.Close()

	select {
	case <-c.Ready():
	case <-time.After(wait):
		t.Fatal("Ready never closed")
	}
	done := make(chan bool, 1)
	go func() { _, ok := c.Next(); done <- ok }()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("Next reported an event from a client that never subscribed")
		}
	case <-time.After(time.Second):
		t.Fatal("Next blocked on a client whose subscription failed")
	}
}
