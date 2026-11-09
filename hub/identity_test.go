package hub

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParticipantID_IsAgentPrefixedSSHUsername(t *testing.T) {
	assert.Equal(t, "Agent-alice", ParticipantID("alice"))
}

func TestParticipantID_NeverEqualsHostID(t *testing.T) {
	assert.NotEqual(t, HostID, ParticipantID("host"))
}

func TestDisplayNames_AreKeyedByID_HostAndParticipantsStaySeparate(t *testing.T) {
	h := NewHub()
	h.SetDisplayName(HostID, "Real Host")
	h.SetDisplayName(ParticipantID("host"), "Impostor")
	h.SetDisplayName(ParticipantID("alice"), "Alice")

	assert.Equal(t, "Real Host", h.GetDisplayName(HostID))
	assert.Equal(t, "Impostor", h.GetDisplayName(ParticipantID("host")))
	assert.Equal(t, "Alice", h.GetDisplayName(ParticipantID("alice")))
}
