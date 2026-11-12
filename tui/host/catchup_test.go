package host

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
)

// hostAfterSending starts a real hub, sends the given messages in order, and
// feeds every event the host receives through Update until nothing more
// arrives. It returns the host's visible screen.
func hostAfterSending(t *testing.T, sends ...*hub.Message) string {
	t.Helper()
	h := hub.NewHub()
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })

	h.SetDisplayName(hub.HostID, "TestHost")
	m := NewAppModel(h)
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 120})

	for _, s := range sends {
		h.Send(context.Background(), s)
	}
	wait := 10 * time.Second // generous until the first event; short once they are flowing
	for {
		res := make(chan tea.Msg, 1)
		go func() { res <- m.waitForEvent()() }()
		select {
		case msg := <-res:
			model, _ = model.Update(msg)
			wait = 500 * time.Millisecond
		case <-time.After(wait):
			return visible(model.View())
		}
	}
}

func snapshotFor(recorded ...*hub.Message) *hub.Message {
	return hub.NewTranscriptSnapshot(1, hub.HostID, recorded, nil)
}

func TestHost_SnapshotFillsTheTranscript(t *testing.T) {
	screen := hostAfterSending(t, snapshotFor(
		hub.NewMessage(hub.MessageTypeDirective, "host", "earlier directive", 1),
		hub.NewMessage(hub.MessageTypeAggregate, "hub", "earlier aggregate", 1),
	))
	for _, want := range []string{"earlier directive", "earlier aggregate"} {
		if !strings.Contains(screen, want) {
			t.Errorf("transcript is missing %q:\n%s", want, screen)
		}
	}
}

// The snapshot is older than anything already shown, so it goes first.
func TestHost_SnapshotMessagesComeBeforeWhatIsAlreadyShown(t *testing.T) {
	screen := hostAfterSending(t,
		hub.NewMessage(hub.MessageTypeSystem, "hub", "live-entry", 2),
		snapshotFor(hub.NewMessage(hub.MessageTypeDirective, "host", "recorded-entry", 1)),
	)
	rec, live := strings.Index(screen, "recorded-entry"), strings.Index(screen, "live-entry")
	if rec < 0 || live < 0 {
		t.Fatalf("both entries should be shown:\n%s", screen)
	}
	if rec > live {
		t.Errorf("recorded entry is shown after the live one:\n%s", screen)
	}
}

// The newest message in the snapshot is on screen once the host is rebuilt,
// even when the conversation is longer than the pane.
func TestHost_SnapshotShowsTheNewestMessage(t *testing.T) {
	var recorded []*hub.Message
	for i := 0; i < 60; i++ {
		recorded = append(recorded, hub.NewMessage(hub.MessageTypeDirective, "host", fmt.Sprintf("recorded-%02d", i), 1))
	}
	screen := hostAfterSending(t, snapshotFor(recorded...))
	if !strings.Contains(screen, "recorded-59") {
		t.Errorf("newest snapshot message is not on screen:\n%s", screen)
	}
}

// A live message that arrived before the snapshot, and is also in it, is shown
// once, after the snapshot's older messages.
func TestHost_LiveMessageFirst_SnapshotOlderMessagesGoBeforeIt(t *testing.T) {
	older := hub.NewMessage(hub.MessageTypeDirective, "host", "older-entry", 1)
	live := hub.NewMessage(hub.MessageTypeAggregate, "hub", "live-entry", 1)
	screen := hostAfterSending(t, live, snapshotFor(older, live))
	if n := strings.Count(screen, "live-entry"); n != 1 {
		t.Fatalf("live-entry shown %d times, want 1:\n%s", n, screen)
	}
	if o, l := strings.Index(screen, "older-entry"), strings.Index(screen, "live-entry"); o < 0 || o > l {
		t.Errorf("older entry should be shown before the live one:\n%s", screen)
	}
}

// A live message that is also in the snapshot is shown once.
func TestHost_LiveMessageInTheSnapshotIsShownOnce(t *testing.T) {
	dup := hub.NewMessage(hub.MessageTypeDirective, "host", "shared-entry", 1)
	screen := hostAfterSending(t, dup, snapshotFor(dup))
	if n := strings.Count(screen, "shared-entry"); n != 1 {
		t.Errorf("shared-entry shown %d times, want 1:\n%s", n, screen)
	}
}
