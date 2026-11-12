package participant

import (
	"context"
	"strings"
	"testing"

	"github.com/jakofranko/haive-mind/hub"
	"github.com/stretchr/testify/assert"
)

// A participant who joins late sees the conversation so far, and what comes
// after it follows it.
func TestCatchUp_ShowsTheEarlierConversationBeforeWhatFollows(t *testing.T) {
	h := serverHub(t)
	ctx := context.Background()
	h.Send(ctx, hub.NewMessage(hub.MessageTypeSystem, "hub", "first entry", 1))
	h.Send(ctx, hub.NewMessage(hub.MessageTypeSystem, "hub", "second entry", 1))

	m := join(t, h, "Agent-alice", "Alice")
	h.Send(ctx, hub.NewMessage(hub.MessageTypeSystem, "hub", "live entry", 1))
	m = receiveUntil(t, m, shows("live entry"))

	screen := visible(m.View())
	assert.Less(t, strings.Index(screen, "first entry"), strings.Index(screen, "second entry"))
	assert.Less(t, strings.Index(screen, "second entry"), strings.Index(screen, "live entry"))
	assert.Equal(t, 1, strings.Count(screen, "second entry"))
}

// A participant who joins after a new round sees the current round, and who
// else is here.
func TestCatchUp_ShowsTheRoundAndWhoIsHere(t *testing.T) {
	h := serverHub(t)
	bob := join(t, h, "Agent-bob", "Bob")
	h.StartRound(1)
	receiveUntil(t, bob, shows("Participants (Round 2"))

	m := join(t, h, "Agent-alice", "Alice")

	screen := visible(m.View())
	assert.Contains(t, screen, "Participants (Round 2")
	assert.Contains(t, screen, "Bob")
}
