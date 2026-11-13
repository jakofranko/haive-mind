package host

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/stretchr/testify/assert"
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

// inRound2 is a host that has seen round 2 begin.
func inRound2(t *testing.T) (*AppModel, hub.Hub, func() []*hub.Message) {
	t.Helper()
	h, _ := serverHub(t)
	m := joinHost(t, h)
	sent := recordSent(t, h, hub.MessageTypeAggregate, hub.MessageTypeDirective)
	h.StartRound(1)
	m = receiveUntil(t, m, shows("Round 2"))
	return m, h, sent
}

// A Stale draft joins the transcript with its round; the input, focus and
// aggregate mode stay as they were, so Enter keeps its meaning.
func TestHost_StaleDraftIsOnlyInTheTranscript(t *testing.T) {
	m, h, sent := inRound2(t)
	m = press(press(m, tea.KeyTab), tea.KeyTab) // focus the input
	m = typeText(m, "my directive")
	pane := m.activePane

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", "old round draft", 1))
	m = receiveUntil(t, m, shows("old round draft"))

	assert.Contains(t, visible(m.View()), "Round 1")
	assert.Equal(t, "my directive", m.directive.Value())
	assert.Equal(t, pane, m.activePane)

	press(m, tea.KeyEnter)
	got := sent()
	if assert.Len(t, got, 1) {
		assert.Equal(t, hub.MessageTypeDirective, got[0].Type)
		assert.Equal(t, "my directive", got[0].Content)
	}
}

func TestHost_StaleDraftLeavesAnEmptyInputEmpty(t *testing.T) {
	m, h, _ := inRound2(t)

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", "old round draft", 1))
	m = receiveUntil(t, m, shows("old round draft"))

	assert.Equal(t, "", m.directive.Value())
	assert.False(t, m.draftingAggregate)
}

// A second Draft in the current round replaces the input; both stay shown.
func TestHost_SecondCurrentDraftReplacesTheInput(t *testing.T) {
	m, h, _ := inRound2(t)

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", "first draft", 2))
	m = receiveUntil(t, m, shows("first draft"))
	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", "second draft", 2))
	m = receiveUntil(t, m, func(s string) bool { return m.directive.Value() == "second draft" })

	assert.Equal(t, "second draft", m.directive.Value())
	assert.Contains(t, visible(m.View()), "first draft")
}
