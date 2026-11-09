package host

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
)

// hostWithAlice is a host model that knows two participants, with the
// participants pane focused and the cursor on Alice.
func hostWithAlice(t *testing.T) (*AppModel, func() []*hub.Message) {
	t.Helper()
	h, sent := sentRig(t, hub.MessageTypeFlag, hub.MessageTypeUnflag, hub.MessageTypeResolve, hub.MessageTypeAggregateRequest)
	m := NewAppModel(h)
	updated, _ := m.Update(initialParticipantsMsg{
		{Name: "host", DisplayName: "TestHost", Status: hub.StatusIdle},
		{Name: "Agent-alice", DisplayName: "Alice", Status: hub.StatusSubmitted},
	})
	m = updated.(*AppModel)
	m = press(m, tea.KeyTab) // transcript -> participants
	m = typeText(m, "j")     // host row -> Alice
	return m, sent
}

func flaggedUpdate(ids ...string) hubMessageMsg {
	return hubMessageMsg(*hub.NewFlagUpdate(1, ids))
}

func runAll(m *AppModel, cmd tea.Cmd) *AppModel { return run(m, cmd) }

// G12: `c` on the selected participant asks the coordinator to flag them, and
// `c` again lifts the flag.
func TestHost_CFlagsAndUnflagsTheSelectedParticipant(t *testing.T) {
	m, sent := hostWithAlice(t)

	m = typeText(m, "c")
	updated, _ := m.Update(flaggedUpdate("Agent-alice")) // the coordinator's announcement
	m = updated.(*AppModel)
	m = typeText(m, "c")

	got := sent()
	if assert.Len(t, got, 2) {
		assert.Equal(t, hub.MessageTypeFlag, got[0].Type)
		assert.Equal(t, "Agent-alice", got[0].To)
		assert.Equal(t, hub.MessageTypeUnflag, got[1].Type)
		assert.Equal(t, "Agent-alice", got[1].To)
	}
}

func TestHost_CannotFlagTheHost(t *testing.T) {
	h, sent := sentRig(t, hub.MessageTypeFlag, hub.MessageTypeUnflag)
	m := NewAppModel(h)
	updated, _ := m.Update(initialParticipantsMsg{{Name: "host", DisplayName: "TestHost", Status: hub.StatusIdle}})
	m = updated.(*AppModel)
	m = press(m, tea.KeyTab)
	typeText(m, "c")

	assert.Empty(t, sent())
}

// A flagged participant shows as "revise" in the list.
func TestHost_FlaggedParticipantShowsRevise(t *testing.T) {
	m, _ := hostWithAlice(t)
	assert.NotContains(t, visible(m.View()), "revise")

	updated, _ := m.Update(flaggedUpdate("Agent-alice"))
	m = updated.(*AppModel)
	assert.Contains(t, visible(m.View()), "revise")

	updated, _ = m.Update(flaggedUpdate())
	m = updated.(*AppModel)
	assert.NotContains(t, visible(m.View()), "revise")
}

// The flag update is not transcript content.
func TestHost_FlagUpdateIsNotShownInTheTranscript(t *testing.T) {
	m, _ := hostWithAlice(t)
	updated, _ := m.Update(flaggedUpdate("Agent-alice"))
	m = updated.(*AppModel)
	assert.NotContains(t, visible(m.View()), "[\"Agent-alice\"]")
}

// [Resolve] clears every flag.
func TestHost_ResolveButtonSendsResolve(t *testing.T) {
	m, sent := hostWithAlice(t)
	m = press(m, tea.KeyTab) // directive
	m = press(m, tea.KeyTab) // buttons
	for i := 0; i < 4; i++ { // Aggregate, Send, New Round, Export, then Resolve
		m = typeText(m, "l")
	}
	assert.Contains(t, visible(m.View()), "[Resolve]")
	m = press(m, tea.KeyEnter)

	got := sent()
	if assert.Len(t, got, 1) {
		assert.Equal(t, hub.MessageTypeResolve, got[0].Type)
	}
}

func pressAggregateButton(m *AppModel) (*AppModel, tea.Cmd) {
	m = press(m, tea.KeyTab) // directive
	m = press(m, tea.KeyTab) // buttons; [Aggregate] is first
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(*AppModel), cmd
}

// Aggregating with flags up warns first; Enter goes ahead anyway.
func TestHost_AggregateWithFlagsWarnsThenContinues(t *testing.T) {
	m, sent := hostWithAlice(t)
	updated, _ := m.Update(flaggedUpdate("Agent-alice"))
	m = updated.(*AppModel)

	m, cmd := pressAggregateButton(m)
	m = runAll(m, cmd)

	assert.Contains(t, visible(m.View()), "still flagged")
	assert.Contains(t, visible(m.View()), "Continue")
	assert.Empty(t, sent(), "nothing is aggregated before the host confirms")

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(updated.(*AppModel), cmd)

	got := sent()
	if assert.Len(t, got, 1) {
		assert.Equal(t, hub.MessageTypeAggregateRequest, got[0].Type)
	}
}

func TestHost_AggregateWarningCanBeCancelled(t *testing.T) {
	m, sent := hostWithAlice(t)
	updated, _ := m.Update(flaggedUpdate("Agent-alice"))
	m = updated.(*AppModel)

	m, cmd := pressAggregateButton(m)
	m = runAll(m, cmd)
	m = press(m, tea.KeyEsc)

	assert.Empty(t, sent())
}

func TestHost_AggregateWithoutFlagsGoesStraightThrough(t *testing.T) {
	m, sent := hostWithAlice(t)

	pressAggregateButton(m)

	assert.Len(t, sent(), 1)
}

// #32: if the participant under the cursor (the last row) leaves, the cursor
// moves to a row that exists, so `c` still targets someone.
func TestHost_CursorStaysOnARowWhenTheSelectedParticipantLeaves(t *testing.T) {
	m, sent := hostWithAlice(t)
	updated, _ := m.Update(participantUpdateMsg(&hub.Participant{Name: "Agent-bob", DisplayName: "Bobby", Status: hub.StatusIdle}))
	m = updated.(*AppModel)
	m = typeText(m, "j") // Alice -> Bobby, the last row

	updated, _ = m.Update(participantUpdateMsg(&hub.Participant{Name: "Agent-bob", Status: hub.StatusDisconnected}))
	m = updated.(*AppModel)
	assert.NotContains(t, visible(m.View()), "Bobby")

	typeText(m, "c")
	got := sent()
	if assert.Len(t, got, 1, "c should land on the last remaining participant") {
		assert.Equal(t, "Agent-alice", got[0].To)
	}
}
