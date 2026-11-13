package hub

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundState_RoundAndFlaggedIDsRoundTrip(t *testing.T) {
	m := NewRoundState(2, []string{"Agent-alice", "Agent-bob"}, nil)
	assert.Equal(t, MessageTypeRoundState, m.Type)

	round, ids, _, err := m.RoundState()
	require.NoError(t, err)
	assert.Equal(t, 2, round)
	assert.Equal(t, []string{"Agent-alice", "Agent-bob"}, ids)
}

func TestRoundState_EmptySetDecodesToNoIDs(t *testing.T) {
	round, ids, _, err := NewRoundState(1, nil, nil).RoundState()
	require.NoError(t, err)
	assert.Equal(t, 1, round)
	assert.Empty(t, ids)
}

func TestRoundState_RejectsMalformedContent(t *testing.T) {
	m := NewMessage(MessageTypeRoundState, "hub", "not json", 1)
	_, _, _, err := m.RoundState()
	assert.Error(t, err)
}
