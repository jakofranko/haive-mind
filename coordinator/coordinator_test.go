package coordinator_test

import (
	"context"
	"fmt"
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
	t.Helper()
	h := hub.NewHub()
	r := &countingRunner{}
	h.SetAgentRunner(r)
	c := coordinator.New(h)
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })
	return h, c, r
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
		return countType(c.Transcript(), hub.MessageTypeResponse, "agent") >= 1
	})
	time.Sleep(100 * time.Millisecond) // give any duplicate time to show up

	if got := r.calls.Load(); got != 1 {
		t.Errorf("agent invoked %d times, want 1", got)
	}
	if got := countType(c.Transcript(), hub.MessageTypeResponse, "agent"); got != 1 {
		t.Errorf("transcript has %d agent replies, want 1", got)
	}
}

// G3: an aggregate request builds exactly one aggregate from this round's
// participant responses, sends it to the agent once, and does not loop.
func TestAggregateRequest_ProducesOneAggregateAndNoLoop(t *testing.T) {
	h, c, r := newRig(t)
	ctx := context.Background()

	h.Send(ctx, hub.NewMessage(hub.MessageTypeResponse, "alice", "use postgres", 1))
	h.Send(ctx, hub.NewMessage(hub.MessageTypeResponse, "bob", "use sqlite", 1))
	eventually(t, "both responses recorded", func() bool {
		return countType(c.Transcript(), hub.MessageTypeResponse, "") >= 2
	})

	if err := h.RequestAggregate(1); err != nil {
		t.Fatal(err)
	}

	eventually(t, "agent reply to the aggregate", func() bool {
		return countType(c.Transcript(), hub.MessageTypeResponse, "agent") >= 1
	})
	time.Sleep(200 * time.Millisecond) // a loop would keep producing aggregates

	aggregates := 0
	for _, m := range c.Transcript() {
		if m.Type == hub.MessageTypeAggregate && m.Content != "aggregate_request" {
			aggregates++
		}
	}
	if aggregates != 1 {
		t.Errorf("got %d aggregates, want 1", aggregates)
	}
	if got := r.calls.Load(); got != 1 {
		t.Errorf("agent invoked %d times, want 1", got)
	}
}

// A new-round request advances the round once and announces it once.
func TestNewRound_AdvancesAndAnnouncesOnce(t *testing.T) {
	h, c, _ := newRig(t)

	if err := h.StartRound(2); err != nil {
		t.Fatal(err)
	}

	eventually(t, "round change announced", func() bool {
		return countType(c.Transcript(), hub.MessageTypeRoundChange, "") >= 1
	})
	time.Sleep(100 * time.Millisecond)

	if got := c.Round(); got != 2 {
		t.Errorf("Round() = %d, want 2", got)
	}
	if got := countType(c.Transcript(), hub.MessageTypeRoundChange, ""); got != 1 {
		t.Errorf("got %d round-change messages, want 1", got)
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
