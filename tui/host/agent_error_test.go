package host

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/stretchr/testify/assert"
)

// hostWithFailure is a host whose last directive failed, showing the toast.
func hostWithFailure(t *testing.T) (*AppModel, func() []*hub.Message) {
	t.Helper()
	h, _ := serverHub(t)
	m := joinHost(t, h)
	sent := recordSent(t, h, hub.MessageTypeDirective)
	failure := hub.NewMessage(hub.MessageTypeAgentError, "hub", "Agent error: boom", 1)
	failure.Retry = &hub.Retry{Type: hub.MessageTypeDirective, Content: "ask again"}

	h.Send(context.Background(), failure)
	updated, cmd := m.Update(nextMsg(t, m))
	return run(updated.(*AppModel), cmd), sent
}

// G16: an agent failure surfaces as a toast on the host, and Retry sends the
// failed message to the hub again, as the host.
func TestHost_AgentErrorShowsToastAndRetryResends(t *testing.T) {
	m, sent := hostWithFailure(t)

	assert.Contains(t, m.View(), "boom")
	assert.Contains(t, m.View(), "Retry")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	run(updated.(*AppModel), cmd)

	got := sent()
	if assert.Len(t, got, 1) {
		assert.Equal(t, "ask again", got[0].Content)
		assert.Equal(t, hub.MessageTypeDirective, got[0].Type)
		assert.Equal(t, "TestHost", got[0].From)
	}
}

func TestHost_AgentErrorDismissDoesNotResend(t *testing.T) {
	m, sent := hostWithFailure(t)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = run(updated.(*AppModel), cmd)

	assert.NotContains(t, m.View(), "boom")
	assert.Empty(t, sent())
}
