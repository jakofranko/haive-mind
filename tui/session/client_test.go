package session_test

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/session"
)

const wait = 5 * time.Second

func startedHub(t *testing.T) hub.Hub {
	t.Helper()
	h := hub.NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })
	return h
}

// next returns the client's next event, failing the test if none arrives.
func next(t *testing.T, c *session.Client) session.Event {
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

func newClient(t *testing.T, h hub.Hub, extra ...hub.MessageType) *session.Client {
	t.Helper()
	c := session.New(h, extra...)
	t.Cleanup(c.Close)
	select {
	case <-c.Ready():
	case <-time.After(wait):
		t.Fatal("client never became ready")
	}
	return c
}

func TestMessagesArriveAsTypedEventsInSendOrder(t *testing.T) {
	h := startedHub(t)
	c := newClient(t, h)

	ctx := context.Background()
	types := []hub.MessageType{
		hub.MessageTypeDirective, hub.MessageTypeResponse, hub.MessageTypeAgentReply,
		hub.MessageTypeSystem, hub.MessageTypeAggregate, hub.MessageTypeAggregateDraft,
	}
	for i, mt := range types {
		h.Send(ctx, hub.NewMessage(mt, "x", fmt.Sprintf("%d", i), 1))
	}
	for i, mt := range types {
		ev, ok := next(t, c).(session.MessageEvent)
		if !ok {
			t.Fatalf("event %d is not a MessageEvent", i)
		}
		if ev.Message.Type != mt || ev.Message.Content != fmt.Sprintf("%d", i) {
			t.Fatalf("event %d = %s %q, want %s %d", i, ev.Message.Type, ev.Message.Content, mt, i)
		}
	}
}

func TestBurstIsDeliveredWithoutDrops(t *testing.T) {
	h := startedHub(t)
	c := newClient(t, h)

	const n = 300
	ctx := context.Background()
	for i := 0; i < n; i++ { // nothing is reading yet
		h.Send(ctx, hub.NewMessage(hub.MessageTypeSystem, "hub", fmt.Sprintf("%03d", i), 1))
	}
	for i := 0; i < n; i++ {
		ev := next(t, c).(session.MessageEvent)
		if want := fmt.Sprintf("%03d", i); ev.Message.Content != want {
			t.Fatalf("event %d = %s, want %s", i, ev.Message.Content, want)
		}
	}
}

func TestParticipantEventsAreDecoded(t *testing.T) {
	h := startedHub(t)
	c := newClient(t, h)

	h.RegisterParticipant(hub.NewParticipantEvent("Agent-alice", "Alice", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	connect := next(t, c).(session.ParticipantEvent).Participant
	if connect.Name != "Agent-alice" || connect.DisplayName != "Alice" || connect.Event != hub.ParticipantEventConnect {
		t.Errorf("connect = %+v", connect)
	}

	h.UpdateParticipantStatus("Agent-alice", hub.StatusTyping)
	change := next(t, c).(session.ParticipantEvent).Participant
	if change.Name != "Agent-alice" || change.Status != hub.StatusTyping {
		t.Errorf("status change = %+v", change)
	}

	// The hub forgets a participant on disconnect, so this cannot be a lookup.
	h.UnregisterParticipant("Agent-alice")
	gone := next(t, c).(session.ParticipantEvent).Participant
	if gone.Name != "Agent-alice" || gone.Status != hub.StatusDisconnected || gone.Event != hub.ParticipantEventDisconnect {
		t.Errorf("disconnect = %+v", gone)
	}
}

func TestRoundChangeIsDelivered(t *testing.T) {
	h := startedHub(t)
	c := newClient(t, h)

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeRoundChange, "hub", "", 3))
	if got := next(t, c).(session.RoundEvent).Round; got != 3 {
		t.Errorf("round = %d, want 3", got)
	}
}

func TestFlagUpdateDeliversFlaggedIDs(t *testing.T) {
	h := startedHub(t)
	c := newClient(t, h)

	h.Send(context.Background(), hub.NewFlagUpdate(1, []string{"Agent-a", "Agent-b"}))
	got := next(t, c).(session.FlagEvent).IDs
	if want := []string{"Agent-a", "Agent-b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("flagged = %v, want %v", got, want)
	}
}

func TestExtraTypesAreDeliveredOnlyToAClientThatAsks(t *testing.T) {
	h := startedHub(t)
	host := newClient(t, h, hub.MessageTypeAgentError, hub.MessageTypeExportComplete)
	participant := newClient(t, h)

	ctx := context.Background()
	h.Send(ctx, hub.NewMessage(hub.MessageTypeExportComplete, "hub", "saved", 1))
	h.Send(ctx, hub.NewMessage(hub.MessageTypeSystem, "hub", "after", 1))

	if ev, ok := next(t, host).(session.ExportEvent); !ok || ev.Path != "saved" {
		t.Errorf("host first event = %+v, want an ExportEvent for \"saved\"", ev)
	}
	if ev := next(t, participant).(session.MessageEvent); ev.Message.Type != hub.MessageTypeSystem {
		t.Errorf("participant first event = %s, want system (export_complete must not reach it)", ev.Message.Type)
	}
}

func TestReadyMeansMessagesAreNowDelivered(t *testing.T) {
	h := startedHub(t)
	c := session.New(h)
	defer c.Close()
	select {
	case <-c.Ready():
	case <-time.After(wait):
		t.Fatal("never ready")
	}
	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeSystem, "hub", "hello", 1))
	if ev := next(t, c).(session.MessageEvent); ev.Message.Content != "hello" {
		t.Errorf("got %q", ev.Message.Content)
	}
}

func TestCloseReleasesAPendingNext(t *testing.T) {
	h := startedHub(t)
	c := newClient(t, h)

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
	c := newClient(t, h)
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
	ch := &countingHub{Hub: startedHub(t)}
	c := session.New(ch)
	if ch.subs.Load() == 0 {
		t.Fatal("client made no subscriptions; test is not exercising anything")
	}
	c.Close()
	if got, want := ch.unsubs.Load(), ch.subs.Load(); got != want {
		t.Errorf("unsubscribed %d of %d subscriptions", got, want)
	}
}

// failingHub embeds a real hub whose SubscribeTypes always fails.
type failingHub struct{ hub.Hub }

func (failingHub) SubscribeTypes(context.Context, []hub.MessageType, func(*hub.Message) error) (hub.Subscription, error) {
	return nil, fmt.Errorf("subscribe refused")
}

func TestFailedSubscriptionClosesTheClient(t *testing.T) {
	c := session.New(failingHub{Hub: startedHub(t)})
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
