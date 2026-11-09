package hub

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundState_FlaggedIDsRoundTrip(t *testing.T) {
	m := NewRoundState(2, []string{"Agent-alice", "Agent-bob"})
	assert.Equal(t, MessageTypeRoundState, m.Type)
	assert.Equal(t, 2, m.Round)

	ids, err := m.RoundStateFlaggedIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"Agent-alice", "Agent-bob"}, ids)
}

func TestRoundState_EmptySetDecodesToNoIDs(t *testing.T) {
	ids, err := NewRoundState(1, nil).RoundStateFlaggedIDs()
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestRoundState_FlaggedIDsRejectsMalformedContent(t *testing.T) {
	m := NewMessage(MessageTypeRoundState, "hub", "not json", 1)
	_, err := m.RoundStateFlaggedIDs()
	assert.Error(t, err)
}
