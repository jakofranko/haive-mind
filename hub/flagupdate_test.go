package hub

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlagUpdate_FlaggedIDsRoundTrip(t *testing.T) {
	m := NewFlagUpdate(2, []string{"Agent-alice", "Agent-bob"})
	assert.Equal(t, MessageTypeFlagUpdate, m.Type)
	assert.Equal(t, 2, m.Round)

	ids, err := m.FlaggedIDs()
	require.NoError(t, err)
	assert.Equal(t, []string{"Agent-alice", "Agent-bob"}, ids)
}

func TestFlagUpdate_EmptySetDecodesToNoIDs(t *testing.T) {
	ids, err := NewFlagUpdate(1, nil).FlaggedIDs()
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestFlagUpdate_FlaggedIDsRejectsMalformedContent(t *testing.T) {
	m := NewMessage(MessageTypeFlagUpdate, "hub", "not json", 1)
	_, err := m.FlaggedIDs()
	assert.Error(t, err)
}
