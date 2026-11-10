package host

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
)

// hostWithDraft is a host whose input has just been given a synthesized draft.
func hostWithDraft(t *testing.T, draft string) (*AppModel, func() []*hub.Message) {
	t.Helper()
	h, _ := serverHub(t)
	m := joinHost(t, h)
	sent := recordSent(t, h, hub.MessageTypeAggregate, hub.MessageTypeDirective)

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", draft, 1))
	return receiveUntil(t, m, shows(draft)), sent
}

// G11: a synthesized draft opens in the host's input, the host edits it, and
// Enter sends the edited text as the aggregate (not as a directive).
func TestHost_DraftIsEditableAndSentAsAggregate(t *testing.T) {
	m, sent := hostWithDraft(t, "Group picks Postgres")

	assert.Equal(t, paneDirective, m.activePane)
	assert.Equal(t, "Group picks Postgres", m.directive.Value())

	m = typeText(m, " for prod")
	m = press(m, tea.KeyEnter)

	got := sent()
	if assert.Len(t, got, 1) {
		assert.Equal(t, hub.MessageTypeAggregate, got[0].Type)
		assert.Equal(t, "Group picks Postgres for prod", got[0].Content)
		assert.Equal(t, "TestHost", got[0].From)
	}
	assert.Equal(t, "", m.directive.Value())
}

// Esc throws the draft away: nothing is sent and the input is cleared.
func TestHost_EscDiscardsDraft(t *testing.T) {
	m, sent := hostWithDraft(t, "Group picks Postgres")

	m = press(m, tea.KeyEsc)

	assert.Equal(t, "", m.directive.Value())
	assert.Empty(t, sent())

	// And Enter afterwards is an ordinary directive again.
	m = typeText(m, "next question")
	press(m, tea.KeyEnter)
	got := sent()
	if assert.Len(t, got, 1) {
		assert.Equal(t, hub.MessageTypeDirective, got[0].Type)
	}
}
