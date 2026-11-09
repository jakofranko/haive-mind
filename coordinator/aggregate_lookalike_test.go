package coordinator_test

import (
	"context"
	"testing"
	"time"

	"jakofranko/haive-mind/hub"
)

// A real Aggregate whose text happens to be "aggregate_request" is content for
// the agent, not a signal: it must not trigger synthesis.
func TestAggregate_WithSignalLookalikeText_DoesNotSynthesize(t *testing.T) {
	h, c, agentRunner, synth := newRigWithSynth(t)
	ctx := context.Background()

	h.Send(ctx, hub.NewMessage(hub.MessageTypeDirective, "Host", "Which database?", 1))
	h.Send(ctx, hub.NewMessage(hub.MessageTypeResponse, "alice", "use postgres", 1))
	eventually(t, "directive reply and response recorded", func() bool {
		return countType(c.Transcript(), hub.MessageTypeResponse, "") >= 1
	})
	before := agentRunner.calls.Load()

	h.Send(ctx, hub.NewMessage(hub.MessageTypeAggregate, "Host", "aggregate_request", 1))

	eventually(t, "agent invoked with the aggregate", func() bool {
		return agentRunner.calls.Load() == before+1
	})
	time.Sleep(200 * time.Millisecond)
	if got := synth.calls.Load(); got != 0 {
		t.Errorf("synthesizer invoked %d times by a real aggregate, want 0", got)
	}
}
