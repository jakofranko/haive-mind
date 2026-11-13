package host

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// hostAfterNewRound is a host that has seen the second round begin.
func hostAfterNewRound(t *testing.T) (*AppModel, hub.Hub) {
	t.Helper()
	h, _ := serverHub(t)
	m := joinHost(t, h)
	h.StartRound(1)
	return receiveUntil(t, m, shows("Round 2")), h
}

// sendDraft has the synthesizer publish a Draft for round and applies events
// until the host shows it.
func sendDraft(t *testing.T, m *AppModel, h hub.Hub, text string, round int) *AppModel {
	t.Helper()
	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", text, round))
	return receiveUntil(t, m, shows(text))
}

// A Stale draft joins the transcript with its round; the input, focus and
// aggregate mode stay as they were, so Enter keeps its meaning.
func TestHost_StaleDraftIsOnlyInTheTranscript(t *testing.T) {
	m, h := hostAfterNewRound(t)
	sent := recordSent(t, h, hub.MessageTypeAggregate, hub.MessageTypeDirective)
	m = press(press(m, tea.KeyTab), tea.KeyTab) // focus the input
	m = typeText(m, "my directive")
	pane := m.activePane

	m = sendDraft(t, m, h, "old round draft", 1)

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
	m, h := hostAfterNewRound(t)

	m = sendDraft(t, m, h, "old round draft", 1)

	assert.Equal(t, "", m.directive.Value())
	assert.False(t, m.draftingAggregate)
}

// A second Draft in the current round replaces the input; both stay shown.
func TestHost_SecondCurrentDraftReplacesTheInput(t *testing.T) {
	m, h := hostAfterNewRound(t)

	m = sendDraft(t, m, h, "first draft", 2)
	m = sendDraft(t, m, h, "second draft", 2)

	assert.Equal(t, "second draft", m.directive.Value())
	assert.Contains(t, visible(m.View()), "first draft")
}

// After a Stale draft arrives, Enter still sends the open current Draft as
// the aggregate, never the stale text.
func TestHost_StaleDraftDoesNotReplaceAnOpenCurrentDraft(t *testing.T) {
	m, h := hostAfterNewRound(t)
	sent := recordSent(t, h, hub.MessageTypeAggregate, hub.MessageTypeDirective)

	m = sendDraft(t, m, h, "current draft", 2)
	m = sendDraft(t, m, h, "stale draft", 1)

	assert.Equal(t, "current draft", m.directive.Value())
	assert.True(t, m.draftingAggregate)

	press(m, tea.KeyEnter)
	got := sent()
	if assert.Len(t, got, 1) {
		assert.Equal(t, hub.MessageTypeAggregate, got[0].Type)
		assert.Equal(t, "current draft", got[0].Content)
	}
}

// A Draft the hub replays through Catch-up is history: it is in the
// transcript and the input is left alone.
func TestHost_DraftReplayedByCatchUpDoesNotOpen(t *testing.T) {
	h, c := serverHub(t)
	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", "earlier draft", 1))
	require.Eventually(t, func() bool { return len(c.Transcript()) > 0 }, 5*time.Second, 10*time.Millisecond)

	// Sized before Catch-up arrives, so the transcript renders what it is given.
	m := NewAppModel(h)
	t.Cleanup(m.Close)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	m = updated.(*AppModel)
	h.RegisterParticipant(hub.NewParticipantEvent(hub.HostID, "TestHost", hub.StatusIdle, hub.ParticipantEventConnect))
	m = receiveUntil(t, m, shows("earlier draft"))

	assert.Equal(t, "", m.directive.Value())
	assert.False(t, m.draftingAggregate)
}

// A Draft for a later round than the host has seen opens: only an earlier
// round makes a Draft stale.
func TestHost_DraftForALaterRoundOpens(t *testing.T) {
	m, h := hostAfterNewRound(t)

	m = sendDraft(t, m, h, "future draft", 3)

	assert.Equal(t, "future draft", m.directive.Value())
	assert.True(t, m.draftingAggregate)
}

// A Draft that arrives live while the host is still catching up is held until
// Catch-up has said which round it is: a stale one stays out of the input, a
// current one opens. There is no Coordinator here, so the snapshot cannot
// already carry the Draft.
func TestHost_DraftHeldDuringCatchUp(t *testing.T) {
	for _, tc := range []struct {
		name      string
		round     int
		wantOpens bool
	}{
		{"stale", 1, false},
		{"current", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := hub.NewHub()
			h.SetDisplayName(hub.HostID, "TestHost")
			require.NoError(t, h.Start())
			t.Cleanup(func() { h.Stop() })
			m := NewAppModel(h)
			t.Cleanup(m.Close)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
			m = updated.(*AppModel)

			h.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", "early draft", tc.round))
			h.Send(context.Background(), hub.NewTranscriptSnapshot(2, hub.HostID, nil, nil))
			m = receiveUntil(t, m, shows("early draft"))

			if tc.wantOpens {
				assert.Equal(t, "early draft", m.directive.Value())
			} else {
				assert.Equal(t, "", m.directive.Value())
			}
			assert.Equal(t, tc.wantOpens, m.draftingAggregate)
		})
	}
}
