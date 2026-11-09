package participant

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
)

// syncBuffer is an output sink the test can read while the program writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

var ansiCodes = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// text is everything the program has drawn so far, without escape codes.
func (b *syncBuffer) text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return ansiCodes.ReplaceAllString(b.buf.String(), "")
}

// runProgram runs the model as a real Bubble Tea program and returns what it
// draws. The test sees only that output, never the model's state, so nothing is
// read from two goroutines.
func runProgram(t *testing.T, model *ParticipantModel) *syncBuffer {
	t.Helper()
	out := &syncBuffer{}
	prog := tea.NewProgram(model, tea.WithInput(nil), tea.WithOutput(out))
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := prog.Run(); err != nil {
			t.Logf("Program error: %v", err)
		}
	}()
	t.Cleanup(func() {
		prog.Quit()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("program did not stop")
		}
	})

	select {
	case <-model.client.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for subscriptions to be ready")
	}
	prog.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	return out
}

func eventuallyDrawn(t *testing.T, out *syncBuffer, want ...string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		screen := out.text()
		ok := true
		for _, w := range want {
			if !strings.Contains(screen, w) {
				ok = false
			}
		}
		if ok {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("never drew %q. Output so far:\n%s", want, out.text())
}

func startedHub(t *testing.T) hub.Hub {
	t.Helper()
	h := hub.NewHub()
	if err := h.Start(); err != nil {
		t.Fatalf("Failed to start hub: %v", err)
	}
	t.Cleanup(func() { h.Stop() })
	h.SetDisplayName("Agent-user1", "Participant One")
	h.RegisterParticipant(hub.NewParticipantEvent("host", "Host", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	h.RegisterParticipant(hub.NewParticipantEvent("Agent-user1", "Participant One", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	return h
}

func TestParticipantModel_ReceivesMessages(t *testing.T) {
	h := startedHub(t)
	out := runProgram(t, NewParticipantModel(h, "Agent-user1", "user1"))

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeDirective, "Host", "Test directive", 1))

	eventuallyDrawn(t, out, "Test directive", "Host")
}

func TestParticipantModel_ReceivesParticipantEvents(t *testing.T) {
	h := startedHub(t)
	out := runProgram(t, NewParticipantModel(h, "Agent-user1", "user1"))
	eventuallyDrawn(t, out, "Participant One")

	h.RegisterParticipant(hub.NewParticipantEvent("Agent-user2", "Participant Two", hub.StatusIdle, 1, hub.ParticipantEventConnect))

	eventuallyDrawn(t, out, "Participant Two")
}
