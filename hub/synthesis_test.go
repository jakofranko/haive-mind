package hub

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type stubRunner struct {
	mu    sync.Mutex
	calls []string
	reply string
	err   error
}

func (r *stubRunner) Run(ctx context.Context, prompt string) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, prompt)
	r.mu.Unlock()
	return r.reply, r.err
}

func (r *stubRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func collect(t *testing.T, h *hubImpl, types ...MessageType) chan *Message {
	t.Helper()
	got := make(chan *Message, 16)
	if _, err := h.SubscribeTypes(context.Background(), types, func(m *Message) error {
		got <- m
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

func next(t *testing.T, ch chan *Message) *Message {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a message")
		return nil
	}
}

// A synthesis request runs the one-off synthesizer and publishes its output
// as a draft for everyone; the main agent is not involved.
func TestSynthesisRequest_PublishesDraftWithoutInvokingAgent(t *testing.T) {
	h := NewHub()
	agentRunner := &stubRunner{reply: "agent should not run"}
	synth := &stubRunner{reply: "Team consensus: use Postgres"}
	h.SetAgentRunner(agentRunner)
	h.SetSynthesizer(synth)
	drafts := collect(t, h, MessageTypeAggregateDraft)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	h.Send(context.Background(), NewMessage(MessageTypeSynthesisRequest, "hub", "Synthesize: A says x; B says y", 3))

	d := next(t, drafts)
	if d.Content != "Team consensus: use Postgres" {
		t.Errorf("draft content = %q", d.Content)
	}
	if d.Round != 3 {
		t.Errorf("draft round = %d, want 3", d.Round)
	}
	if len(synth.calls) != 1 || !strings.Contains(synth.calls[0], "A says x; B says y") {
		t.Errorf("synthesizer calls = %q", synth.calls)
	}
	if agentRunner.callCount() != 0 {
		t.Errorf("main agent was invoked %d times, want 0", agentRunner.callCount())
	}
}

// When synthesis fails the host gets a visible failure message, not silence,
// and no draft is published.
func TestSynthesisRequest_FailureIsReportedAsSystemMessage(t *testing.T) {
	h := NewHub()
	h.SetSynthesizer(&stubRunner{err: errors.New("boom")})
	got := collect(t, h, MessageTypeAggregateDraft, MessageTypeSystem)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	h.Send(context.Background(), NewMessage(MessageTypeSynthesisRequest, "hub", "Synthesize", 1))

	m := next(t, got)
	if m.Type != MessageTypeSystem || !strings.Contains(m.Content, "Synthesis failed") || !strings.Contains(m.Content, "boom") {
		t.Errorf("got %s %q, want a system message reporting the synthesis failure", m.Type, m.Content)
	}
}
