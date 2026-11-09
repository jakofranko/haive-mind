package coordinator_test

import (
	"context"
	"strings"
	"testing"

	"jakofranko/haive-mind/hub"
)

// A participant whose Display name is "agent" is still a participant: their
// Response is recorded and synthesized. The real Agent Reply is in the
// transcript with its own type and is left out of the synthesis.
func TestParticipantNamedAgent_ResponseIsSynthesized_AgentReplyIsNot(t *testing.T) {
	h, c, _, synth := newRigWithSynth(t)
	ctx := context.Background()

	h.Send(ctx, hub.NewMessage(hub.MessageTypeDirective, "Host", "Which database?", 1))
	eventually(t, "agent reply in transcript", func() bool {
		return countType(c.Transcript(), hub.MessageTypeAgentReply, "") == 1
	})
	h.Send(ctx, hub.NewMessage(hub.MessageTypeResponse, "agent", "use duckdb", 1))
	eventually(t, "response recorded", func() bool {
		return countType(c.Transcript(), hub.MessageTypeResponse, "agent") == 1
	})

	if err := h.RequestAggregate(1); err != nil {
		t.Fatal(err)
	}
	eventually(t, "draft in transcript", func() bool {
		return countType(c.Transcript(), hub.MessageTypeAggregateDraft, "") >= 1
	})

	synth.mu.Lock()
	prompt := synth.prompts[0]
	synth.mu.Unlock()
	if !strings.Contains(prompt, "use duckdb") {
		t.Errorf("synthesis prompt lacks the participant's response: %s", prompt)
	}
	if strings.Contains(prompt, "agent says") {
		t.Errorf("synthesis prompt includes the agent reply: %s", prompt)
	}
}
