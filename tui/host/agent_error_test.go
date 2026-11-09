package host

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/session"
)

// run feeds a command's message back into the model, as Bubble Tea would.
func run(m *AppModel, cmd tea.Cmd) *AppModel {
	if cmd == nil {
		return m
	}
	msg := runBounded(cmd)
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = run(m, c)
		}
		return m
	}
	if msg == nil {
		return m
	}
	updated, next := m.Update(msg)
	return run(updated.(*AppModel), next)
}

func agentErrorMsg(text string, retry hub.Retry) eventMsg {
	msg := hub.NewMessage(hub.MessageTypeAgentError, "hub", text, 1)
	msg.Retry = &retry
	return eventMsg{session.MessageEvent{Message: *msg}}
}

// G16: an agent failure surfaces as a toast on the host, and Retry sends the
// failed message to the hub again.
func TestHost_AgentErrorShowsToastAndRetryResends(t *testing.T) {
	h, sent := sentRig(t, hub.MessageTypeDirective)
	m := NewAppModel(h)

	updated, cmd := m.Update(agentErrorMsg("Agent error: boom", hub.Retry{Type: hub.MessageTypeDirective, Content: "ask again"}))
	m = run(updated.(*AppModel), cmd)

	assert.Contains(t, m.View(), "boom")
	assert.Contains(t, m.View(), "Retry")

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	run(updated.(*AppModel), cmd)

	got := sent()
	if assert.Len(t, got, 1) {
		assert.Equal(t, "ask again", got[0].Content)
		assert.Equal(t, hub.MessageTypeDirective, got[0].Type)
	}
}

func TestHost_AgentErrorDismissDoesNotResend(t *testing.T) {
	h, sent := sentRig(t, hub.MessageTypeDirective)
	m := NewAppModel(h)

	updated, cmd := m.Update(agentErrorMsg("Agent error: boom", hub.Retry{Type: hub.MessageTypeDirective, Content: "ask again"}))
	m = run(updated.(*AppModel), cmd)
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = run(updated.(*AppModel), cmd)

	assert.NotContains(t, m.View(), "boom")
	assert.Empty(t, sent())
}

// runBounded executes a command, giving up on one that waits (a timer, such as
// the toast's auto-hide) rather than blocking the test.
func runBounded(cmd tea.Cmd) tea.Msg {
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		return msg
	case <-time.After(150 * time.Millisecond):
		return nil
	}
}
