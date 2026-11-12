package host

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/stretchr/testify/assert"
)

// The SSH server registers the host once on connect; choosing a name renames
// that entry. The host TUI never registers itself.
func TestAppModel_ChoosingDisplayName_RenamesTheRegisteredHost(t *testing.T) {
	h := hub.NewHub()
	h.RegisterParticipant(hub.NewParticipantEvent(hub.HostID, "Host", hub.StatusIdle, hub.ParticipantEventConnect))
	model := NewAppModel(h)

	model.namePrompt.SetValue("John Doe")
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	ps := h.GetParticipants()
	assert.Len(t, ps, 1)
	assert.Equal(t, hub.HostID, ps[0].ID)
	assert.Equal(t, "John Doe", ps[0].DisplayName)
	assert.Equal(t, hub.ParticipantEventStatusChange, ps[0].Event)
}

func TestAppModel_ChoosingDisplayName_DoesNotRegister(t *testing.T) {
	h := hub.NewHub()
	model := NewAppModel(h)

	model.namePrompt.SetValue("John Doe")
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Empty(t, h.GetParticipants())
}
