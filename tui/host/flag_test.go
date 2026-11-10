package host

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
)

// hostWithAlice is a host who has caught up and sees Alice join, with the
// participants pane focused and the cursor on Alice. sent records the given
// types from that point on.
func hostWithAlice(t *testing.T, types ...hub.MessageType) (*AppModel, hub.Hub, func() []*hub.Message) {
	t.Helper()
	h, _ := serverHub(t)
	m := joinHost(t, h)
	m = addParticipant(t, m, h, "Agent-alice", "Alice", hub.StatusSubmitted)
	m = press(m, tea.KeyTab) // transcript -> participants
	m = typeText(m, "j")     // host row -> Alice
	return m, h, recordSent(t, h, types...)
}

func noRevise(screen string) bool { return !strings.Contains(screen, "revise") }

// flagAlice is the host's flag on Alice reaching everyone, as if the host had
// raised it.
func flagAlice(t *testing.T, m *AppModel, h hub.Hub) *AppModel {
	t.Helper()
	h.Send(context.Background(), hub.NewFlag(1, "Agent-alice"))
	return receiveUntil(t, m, shows("revise"))
}

// G12: `c` on the selected participant asks the coordinator to flag them, and
// `c` again lifts the flag.
func TestHost_CFlagsAndUnflagsTheSelectedParticipant(t *testing.T) {
	m, _, sent := hostWithAlice(t, hub.MessageTypeFlag, hub.MessageTypeUnflag)

	m = typeText(m, "c")
	m = receiveUntil(t, m, shows("revise")) // the coordinator's Round state
	m = typeText(m, "c")
	receiveUntil(t, m, noRevise)

	got := sent()
	if assert.Len(t, got, 2) {
		assert.Equal(t, hub.MessageTypeFlag, got[0].Type)
		assert.Equal(t, "Agent-alice", got[0].To)
		assert.Equal(t, hub.MessageTypeUnflag, got[1].Type)
		assert.Equal(t, "Agent-alice", got[1].To)
	}
}

func TestHost_CannotFlagTheHost(t *testing.T) {
	m, _, sent := hostWithAlice(t, hub.MessageTypeFlag, hub.MessageTypeUnflag)
	m = typeText(m, "k") // Alice -> host row
	typeText(m, "c")

	assert.Empty(t, sent())
}

// A flagged participant shows as "revise" in the list, and the Round state
// itself is not transcript content.
func TestHost_FlaggedParticipantShowsRevise(t *testing.T) {
	m, h, _ := hostWithAlice(t)
	assert.NotContains(t, visible(m.View()), "revise")

	m = flagAlice(t, m, h)
	assert.NotContains(t, visible(m.View()), `["Agent-alice"]`)

	h.Send(context.Background(), hub.NewResolve(1))
	receiveUntil(t, m, noRevise)
}

// [Resolve] clears every flag.
func TestHost_ResolveButtonSendsResolve(t *testing.T) {
	m, _, sent := hostWithAlice(t, hub.MessageTypeResolve)
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
	m, h, sent := hostWithAlice(t, hub.MessageTypeAggregateRequest)
	m = flagAlice(t, m, h)

	m, cmd := pressAggregateButton(m)
	m = run(m, cmd)

	assert.Contains(t, visible(m.View()), "still flagged")
	assert.Contains(t, visible(m.View()), "Continue")
	assert.Empty(t, sent(), "nothing is aggregated before the host confirms")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	run(updated.(*AppModel), cmd)

	got := sent()
	if assert.Len(t, got, 1) {
		assert.Equal(t, hub.MessageTypeAggregateRequest, got[0].Type)
	}
}

func TestHost_AggregateWarningCanBeCancelled(t *testing.T) {
	m, h, sent := hostWithAlice(t, hub.MessageTypeAggregateRequest)
	m = flagAlice(t, m, h)

	m, cmd := pressAggregateButton(m)
	m = run(m, cmd)
	press(m, tea.KeyEsc)

	assert.Empty(t, sent())
}

func TestHost_AggregateWithoutFlagsGoesStraightThrough(t *testing.T) {
	m, _, sent := hostWithAlice(t, hub.MessageTypeAggregateRequest)

	pressAggregateButton(m)

	assert.Len(t, sent(), 1)
}

// #32: if the participant under the cursor (the last row) leaves, the cursor
// moves to a row that exists, so `c` still targets someone.
func TestHost_CursorStaysOnARowWhenTheSelectedParticipantLeaves(t *testing.T) {
	m, h, sent := hostWithAlice(t, hub.MessageTypeFlag)
	m = addParticipant(t, m, h, "Agent-bob", "Bobby", hub.StatusIdle)
	m = typeText(m, "j") // Alice -> Bobby, the last row

	h.UnregisterParticipant("Agent-bob")
	m = receiveUntil(t, m, func(s string) bool { return !strings.Contains(s, "Bobby") })

	typeText(m, "c")
	got := sent()
	if assert.Len(t, got, 1, "c should land on the last remaining participant") {
		assert.Equal(t, "Agent-alice", got[0].To)
	}
}

// A Round state sets the host's round and the markers, and the host sends
// nothing in reply.
func TestHost_RoundStateSetsRoundAndFlaggedMarkers(t *testing.T) {
	m, h, sent := hostWithAlice(t, hub.MessageTypeDirective, hub.MessageTypeAggregate, hub.MessageTypeAggregateRequest)

	h.StartRound(1)
	m = receiveUntil(t, m, shows("Participants (Round 2"))
	h.Send(context.Background(), hub.NewFlag(2, "Agent-alice"))
	receiveUntil(t, m, shows("revise"))

	assert.Empty(t, sent())
}
