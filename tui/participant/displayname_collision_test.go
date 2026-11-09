package participant

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
)

// A participant whose SSH username is "host" is not the host: choosing a
// display name must leave the host's display name alone.
func TestParticipant_SshUsernameHost_DoesNotOverwriteHostDisplayName(t *testing.T) {
	h := hub.NewHub()
	h.SetDisplayName(hub.HostID, "Real Host")

	model := NewParticipantModel(h, "Agent-host", "host")
	assert.True(t, model.namePromptActive, "the host's name is not this participant's name")
	model.namePrompt.SetValue("Impostor")
	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Equal(t, "Real Host", h.GetDisplayName(hub.HostID))
	assert.Equal(t, "Impostor", h.GetDisplayName(hub.ParticipantID("host")))
}
