package coordinator_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jakofranko/haive-mind/coordinator"
	"jakofranko/haive-mind/hub"
)

// countingRunner records every prompt it is asked to run.
type countingRunner struct {
	mu      sync.Mutex
	prompts []string
	calls   atomic.Int32
}

func (c *countingRunner) Run(ctx context.Context, prompt string) (string, error) {
	c.calls.Add(1)
	c.mu.Lock()
	c.prompts = append(c.prompts, prompt)
	c.mu.Unlock()
	return "agent says: " + prompt, nil
}

func newRig(t *testing.T) (hub.Hub, *coordinator.Coordinator, *countingRunner) {
	h, c, r, _ := newRigWithSynth(t)
	return h, c, r
}

func newRigWithSynth(t *testing.T) (hub.Hub, *coordinator.Coordinator, *countingRunner, *countingRunner) {
	t.Helper()
	h := hub.NewHub()
	r := &countingRunner{}
	synth := &countingRunner{}
	h.SetAgentRunner(r)
	h.SetSynthesizer(synth)
	c := coordinator.New(h)
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })
	return h, c, r, synth
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// response is a participant's response the way a session sends it: From is the
// display name, FromID the stable id.
func response(id, display, text string, round int) *hub.Message {
	m := hub.NewMessage(hub.MessageTypeResponse, display, text, round)
	m.FromID = id
	return m
}

func countType(msgs []*hub.Message, typ hub.MessageType, from string) int {
	n := 0
	for _, m := range msgs {
		if m.Type == typ && (from == "" || m.From == from) {
			n++
		}
	}
	return n
}

// G2: a directive reaches the agent exactly once and yields exactly one reply.
func TestDirective_InvokesAgentOnce(t *testing.T) {
	h, c, r := newRig(t)

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeDirective, "Host", "hello", 1))

	eventually(t, "agent reply in transcript", func() bool {
		return countType(c.Transcript(), hub.MessageTypeAgentReply, "") >= 1
	})
	time.Sleep(100 * time.Millisecond) // give any duplicate time to show up

	if got := r.calls.Load(); got != 1 {
		t.Errorf("agent invoked %d times, want 1", got)
	}
	if got := countType(c.Transcript(), hub.MessageTypeAgentReply, ""); got != 1 {
		t.Errorf("transcript has %d agent replies, want 1", got)
	}
}

// G11: an aggregate request asks the one-off synthesizer (never the main
// agent) to combine this round's question and responses, and the draft is
// shared in the transcript. Exactly one synthesis, no loop.
func TestAggregateRequest_SynthesizesOnceAndSharesTheDraft(t *testing.T) {
	h, c, agentRunner, synth := newRigWithSynth(t)
	ctx := context.Background()

	h.Send(ctx, hub.NewMessage(hub.MessageTypeDirective, "Host", "Which database?", 1))
	h.Send(ctx, hub.NewMessage(hub.MessageTypeResponse, "alice", "use postgres", 1))
	h.Send(ctx, hub.NewMessage(hub.MessageTypeResponse, "bob", "use sqlite", 1))
	eventually(t, "directive reply and both responses recorded", func() bool {
		tr := c.Transcript()
		return countType(tr, hub.MessageTypeAgentReply, "") >= 1 &&
			countType(tr, hub.MessageTypeResponse, "") >= 2
	})
	agentCallsBefore := agentRunner.calls.Load()

	if err := h.RequestAggregate(1); err != nil {
		t.Fatal(err)
	}

	eventually(t, "draft in transcript", func() bool {
		return countType(c.Transcript(), hub.MessageTypeAggregateDraft, "") >= 1
	})
	time.Sleep(200 * time.Millisecond) // a loop would keep producing syntheses

	if got := synth.calls.Load(); got != 1 {
		t.Fatalf("synthesizer invoked %d times, want 1", got)
	}
	synth.mu.Lock()
	prompt := synth.prompts[0]
	synth.mu.Unlock()
	for _, want := range []string{"Which database?", "alice", "use postgres", "bob", "use sqlite"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("synthesis prompt lacks %q: %s", want, prompt)
		}
	}
	if got := agentRunner.calls.Load(); got != agentCallsBefore {
		t.Errorf("main agent invoked %d extra times by aggregate request, want 0", got-agentCallsBefore)
	}
	if got := countType(c.Transcript(), hub.MessageTypeAggregateDraft, ""); got != 1 {
		t.Errorf("transcript has %d drafts, want 1", got)
	}
}

// G4: concurrent traffic must not race on coordinator state (run with -race).
func TestConcurrentTraffic_NoRaces(t *testing.T) {
	h, c, _ := newRig(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Send(ctx, hub.NewMessage(hub.MessageTypeResponse, "p", "x", 1))
			h.StartRound(1)
			_ = c.Transcript()
			_ = c.Round()
		}()
	}
	wg.Wait()
	eventually(t, "responses recorded", func() bool {
		return countType(c.Transcript(), hub.MessageTypeResponse, "p") == 20
	})
}

// G23: the recorded transcript keeps send order across message types.
func TestTranscript_KeepsSendOrderAcrossTypes(t *testing.T) {
	h, c, _ := newRig(t)
	ctx := context.Background()

	types := []hub.MessageType{hub.MessageTypeSystem, hub.MessageTypeDirective, hub.MessageTypeAggregate, hub.MessageTypeExportComplete}
	const n = 100
	for i := 0; i < n; i++ {
		typ := types[i%len(types)]
		content := fmt.Sprintf("%03d", i)
		h.Send(ctx, hub.NewMessage(typ, "x", content, 1))
	}
	eventually(t, "all messages recorded", func() bool {
		count := 0
		for _, m := range c.Transcript() {
			if len(m.Content) == 3 && m.Content[0] >= '0' && m.Content[0] <= '9' {
				count++
			}
		}
		return count >= n
	})

	i := 0
	for _, m := range c.Transcript() {
		if want := fmt.Sprintf("%03d", i); m.Content == want {
			i++
		} else if len(m.Content) == 3 && m.Content[0] >= '0' && m.Content[0] <= '9' {
			t.Fatalf("recorded %s where %s was expected (out of order)", m.Content, want)
		}
	}
}
