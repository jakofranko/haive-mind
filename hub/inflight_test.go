package hub

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// slowRunner notices cancellation but takes as long as the test lets it to
// finish, like a real runner killing its process and reaping it.
type slowRunner struct {
	started chan struct{}
	release chan struct{}
}

func newSlowRunner() *slowRunner {
	return &slowRunner{started: make(chan struct{}), release: make(chan struct{})}
}

func (r *slowRunner) Run(ctx context.Context, prompt string) (string, error) {
	close(r.started)
	<-ctx.Done()
	<-r.release
	return "", ctx.Err()
}

// Stop must not return while a call to the agent or the synthesizer is still
// running: it waits for the run to finish.
func TestHub_StopWaitsForInFlightCalls(t *testing.T) {
	cases := []struct {
		name  string
		setup func(h Hub, r *slowRunner)
		msg   *Message
	}{
		{"agent", func(h Hub, r *slowRunner) { h.SetAgentRunner(r) }, NewMessage(MessageTypeDirective, "host", "hi", 1)},
		{"synthesizer", func(h Hub, r *slowRunner) { h.SetSynthesizer(r) }, NewMessage(MessageTypeSynthesisRequest, "hub", "Synthesize: x", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHub()
			r := newSlowRunner()
			tc.setup(h, r)
			if err := h.Start(); err != nil {
				t.Fatal(err)
			}
			if err := h.Send(context.Background(), tc.msg); err != nil {
				t.Fatal(err)
			}
			select {
			case <-r.started:
			case <-time.After(time.Second):
				t.Fatal("run never started")
			}

			stopped := make(chan struct{})
			go func() { h.Stop(); close(stopped) }()

			// The run is still going, so Stop must still be waiting. (A fixed
			// wait is right here: the check is that nothing happens.)
			select {
			case <-stopped:
				t.Fatal("Stop returned while a run was still in flight")
			case <-time.After(100 * time.Millisecond):
			}

			close(r.release)
			select {
			case <-stopped:
			case <-time.After(2 * time.Second):
				t.Fatal("Stop did not return after the run finished")
			}
		})
	}
}

// lockedBuffer is a log sink the hub's goroutines write while the test reads.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A hub with no runner set skips the call, says so in the log, and publishes
// nothing: production wires both runners, and a bare hub stays quiet in tests.
func TestHub_NoRunnerSkipsTheCallAndLogsAWarning(t *testing.T) {
	logs := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := NewHub()
	defer h.Stop()
	var published atomic.Int32
	if _, err := h.SubscribeTypes(context.Background(), []MessageType{MessageTypeAgentReply, MessageTypeAgentError, MessageTypeSystem}, func(*Message) error {
		published.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}

	if err := h.Send(context.Background(), NewMessage(MessageTypeDirective, "host", "hi", 1)); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(logs.String(), "no runner set") {
		if time.Now().After(deadline) {
			t.Fatalf("no warning logged; log was %q", logs.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := published.Load(); got != 0 {
		t.Fatalf("%d messages published by a hub with no runner, want none", got)
	}
}
