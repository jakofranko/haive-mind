package session_test

import (
	"context"
	"testing"

	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/session"
)

func msg(mt hub.MessageType, content string) *hub.Message {
	return hub.NewMessage(mt, "x", content, 1)
}

func TestSnapshotAddressedToMeIsDeliveredWithTranscriptAndLastSubmission(t *testing.T) {
	h := startedHub(t)
	c := newClient(t, h)

	d, r := msg(hub.MessageTypeDirective, "q"), msg(hub.MessageTypeResponse, "a")
	h.Send(context.Background(), hub.NewTranscriptSnapshot(1, me, []*hub.Message{d, r}, r))

	ev, ok := next(t, c).(session.SnapshotEvent)
	if !ok {
		t.Fatal("event is not a SnapshotEvent")
	}
	if len(ev.Messages) != 2 || ev.Messages[0].ID != d.ID || ev.Messages[1].ID != r.ID {
		t.Errorf("Messages = %+v, want the directive then the response", ev.Messages)
	}
	if ev.LastSubmission == nil || ev.LastSubmission.ID != r.ID {
		t.Errorf("LastSubmission = %+v, want the response", ev.LastSubmission)
	}
}

func TestLiveMessageAlreadyInTheSnapshotIsDroppedAndOthersKept(t *testing.T) {
	h := startedHub(t)
	c := newClient(t, h)
	ctx := context.Background()

	have, fresh := msg(hub.MessageTypeDirective, "have"), msg(hub.MessageTypeDirective, "fresh")
	h.Send(ctx, hub.NewTranscriptSnapshot(1, me, []*hub.Message{have}, nil))
	if _, ok := next(t, c).(session.SnapshotEvent); !ok {
		t.Fatal("first event is not the snapshot")
	}
	h.Send(ctx, have)
	h.Send(ctx, fresh)

	ev, ok := next(t, c).(session.MessageEvent)
	if !ok || ev.Message.ID != fresh.ID {
		t.Errorf("event = %+v, want only the message the snapshot did not hold", ev)
	}
}

func TestLiveMessageBeforeTheSnapshotIsNotRepeatedByIt(t *testing.T) {
	h := startedHub(t)
	c := newClient(t, h)
	ctx := context.Background()

	older, live := msg(hub.MessageTypeDirective, "older"), msg(hub.MessageTypeDirective, "live")
	h.Send(ctx, live)
	h.Send(ctx, hub.NewTranscriptSnapshot(1, me, []*hub.Message{older, live}, nil))

	if ev, ok := next(t, c).(session.MessageEvent); !ok || ev.Message.ID != live.ID {
		t.Fatalf("first event = %+v, want the live message", ev)
	}
	ev, ok := next(t, c).(session.SnapshotEvent)
	if !ok {
		t.Fatal("second event is not the snapshot")
	}
	if len(ev.Messages) != 1 || ev.Messages[0].ID != older.ID {
		t.Errorf("snapshot Messages = %+v, want only the older message (the live one was already delivered)", ev.Messages)
	}
}

func TestSnapshotAddressedToSomeoneElseIsIgnored(t *testing.T) {
	h := startedHub(t)
	c := newClient(t, h)
	ctx := context.Background()

	secret := msg(hub.MessageTypeResponse, "bob's answer")
	h.Send(ctx, hub.NewTranscriptSnapshot(1, "Agent-bob", []*hub.Message{secret}, secret))
	h.Send(ctx, msg(hub.MessageTypeSystem, "marker"))

	ev, ok := next(t, c).(session.MessageEvent)
	if !ok || ev.Message.Content != "marker" {
		t.Errorf("first event = %+v, want the marker (the other snapshot must be ignored)", ev)
	}
}
