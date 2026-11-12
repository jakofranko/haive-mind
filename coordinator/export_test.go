package coordinator_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jakofranko/haive-mind/hub"
)

// exportWriter is a stub writer: it captures the transcript it is given and
// returns a path made from the file name.
type exportWriter struct {
	mu    sync.Mutex
	calls []exportCall
	err   error
}

type exportCall struct {
	name       string
	transcript []*hub.Message
}

func (w *exportWriter) write(transcript []*hub.Message, name string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, exportCall{name: name, transcript: transcript})
	if w.err != nil {
		return "", w.err
	}
	return "saved/" + name, nil
}

func (w *exportWriter) all() []exportCall {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]exportCall(nil), w.calls...)
}

// An export request handled straight after other messages exports all of
// them: it is applied in order with everything else.
func TestExport_IncludesEverythingSentBeforeIt(t *testing.T) {
	h, c, _ := newRig(t)
	w := &exportWriter{}
	c.SetExportWriter(w.write)

	const n = 200
	for i := 0; i < n; i++ {
		h.Send(context.Background(), hub.NewMessage(hub.MessageTypeSystem, "hub", fmt.Sprintf("m%03d", i), 1))
	}
	if err := h.Export("transcript.json"); err != nil {
		t.Fatal(err)
	}

	eventually(t, "the writer called", func() bool { return len(w.all()) == 1 })
	call := w.all()[0]
	if call.name != "transcript.json" {
		t.Errorf("writer given name %q, want transcript.json", call.name)
	}
	if got := countType(call.transcript, hub.MessageTypeSystem, "hub"); got != n {
		t.Errorf("export has %d of the %d messages sent before it", got, n)
	}
}

// export_complete carries the path the writer returned.
func TestExport_CompleteCarriesTheWritersPath(t *testing.T) {
	h, c, _ := newRig(t)
	w := &exportWriter{}
	c.SetExportWriter(w.write)
	done := collect(t, h, hub.MessageTypeExportComplete)

	h.Export("transcript.json")

	eventually(t, "export_complete", func() bool { return len(done.all()) == 1 })
	if got := done.all()[0].Content; got != "saved/transcript.json" {
		t.Errorf("export_complete content = %q, want the writer's path", got)
	}
}

// With no writer configured, an export request does nothing.
func TestExport_WithNoWriterSendsNoCompletion(t *testing.T) {
	h, c, _ := newRig(t)
	done := collect(t, h, hub.MessageTypeExportComplete)

	h.Export("transcript.json")
	eventually(t, "export request applied", func() bool {
		return countType(c.Transcript(), hub.MessageTypeExport, "") == 1
	})
	time.Sleep(100 * time.Millisecond) // negative check: no completion follows

	if got := len(done.all()); got != 0 {
		t.Errorf("%d export_complete messages with no writer, want 0", got)
	}
}

// A writer that fails announces no completion.
func TestExport_FailedWriteSendsNoCompletion(t *testing.T) {
	h, c, _ := newRig(t)
	w := &exportWriter{err: errors.New("disk full")}
	c.SetExportWriter(w.write)
	done := collect(t, h, hub.MessageTypeExportComplete)

	h.Export("transcript.json")
	eventually(t, "the writer called", func() bool { return len(w.all()) == 1 })
	time.Sleep(100 * time.Millisecond) // negative check: no completion follows

	if got := len(done.all()); got != 0 {
		t.Errorf("%d export_complete messages after a failed write, want 0", got)
	}
}
