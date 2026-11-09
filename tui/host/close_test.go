package host

import (
	"context"
	"sync/atomic"
	"testing"

	"jakofranko/haive-mind/hub"
)

// countingHub wraps a real hub and counts subscriptions and unsubscribes.
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
	s, err := c.Hub.Subscribe(ctx, t, fn)
	c.subs.Add(1)
	return &countedSub{Subscription: s, h: c}, err
}

// G5: when a host session ends, every hub subscription it made is released.
func TestAppModel_Close_UnsubscribesEverything(t *testing.T) {
	ch := &countingHub{Hub: hub.NewHub()}
	m := NewAppModel(ch)
	m.Init() // registers the subscriptions

	if ch.subs.Load() == 0 {
		t.Fatal("model made no subscriptions; test is not exercising anything")
	}
	m.Close()

	if got, want := ch.unsubs.Load(), ch.subs.Load(); got != want {
		t.Errorf("unsubscribed %d of %d subscriptions", got, want)
	}
}
