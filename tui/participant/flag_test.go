package participant

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/session"
)

func flagUpdate(ids ...string) eventMsg {
	return eventMsg{session.RoundStateEvent{Round: 1, IDs: ids}}
}

// sized gives the model a screen so View() shows the panes.
func sized(m *ParticipantModel) *ParticipantModel {
	return step(m, tea.WindowSizeMsg{Width: 140, Height: 40})
}

// submitted is a participant who has already sent an answer.
func submitted(t *testing.T, answer string) (*ParticipantModel, func() hub.ParticipantStatus) {
	t.Helper()
	m, status := statusRig(t)
	m = sized(m)
	m = typeRunes(m, answer)
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})
	return m, status
}

// G12: when the host flags this participant they are told, their last answer
// comes back for editing, and they are idle again.
func TestFlagged_GetsNoticeAndTheirLastAnswerBack(t *testing.T) {
	m, status := submitted(t, "use postgres")
	assert.Equal(t, hub.StatusSubmitted, status())
	assert.Equal(t, "", m.response.Value())

	m = step(m, flagUpdate("Agent-alice"))

	assert.Contains(t, visible(m.View()), "asked you to revise")
	assert.Equal(t, "use postgres", m.response.Value())
	assert.Equal(t, hub.StatusIdle, status())
}

// Text they have already started typing is left alone.
func TestFlagged_DoesNotOverwriteWhatTheyAreTyping(t *testing.T) {
	m, _ := submitted(t, "use postgres")
	m = typeRunes(m, "on second thought, sqlite")

	m = step(m, flagUpdate("Agent-alice"))

	assert.Equal(t, "on second thought, sqlite", m.response.Value())
}

// Someone else being flagged is not this participant's notice.
func TestFlagged_OtherParticipantsFlagIsNotMyNotice(t *testing.T) {
	m, _ := submitted(t, "use postgres")

	m = step(m, flagUpdate("Agent-bob"))

	assert.NotContains(t, visible(m.View()), "asked you to revise")
	assert.Equal(t, "", m.response.Value())
}

// The notice appears when they become flagged, not on every announcement.
func TestFlagged_NoticeIsShownOncePerFlag(t *testing.T) {
	m, _ := submitted(t, "use postgres")

	m = step(m, flagUpdate("Agent-alice"))
	m = step(m, flagUpdate("Agent-alice", "Agent-bob"))

	assert.Equal(t, 1, strings.Count(visible(m.transcript.View()), "asked you to revise"))
}

// #53: a flagged participant whose new session has never seen the flag is told
// once when the connect announcement arrives, and a repeat does not tell them again.
func TestFlagged_ReconnectingSessionIsToldOnce(t *testing.T) {
	m, _ := statusRig(t)
	m = sized(m)

	m = step(m, flagUpdate("Agent-alice"))
	m = step(m, flagUpdate("Agent-alice"))

	assert.Equal(t, 1, strings.Count(visible(m.transcript.View()), "asked you to revise"))
}

// Everyone sees who is flagged, and the announcement itself is not chat.
func TestFlagged_SidebarShowsReviseForFlaggedParticipants(t *testing.T) {
	h := hub.NewHub()
	h.SetDisplayName("Agent-alice", "Alice")
	h.RegisterParticipant(hub.NewParticipantEvent("Agent-bob", "Bobby", hub.StatusSubmitted, 1, hub.ParticipantEventConnect))
	h.RegisterParticipant(hub.NewParticipantEvent("Agent-alice", "Alice", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	m := sized(NewParticipantModel(h, "Agent-alice", "alice"))
	m = step(m, initialParticipantsMsg(m.client.Participants()))
	assert.NotContains(t, visible(m.View()), "revise")

	m = step(m, flagUpdate("Agent-bob"))
	assert.Contains(t, visible(m.View()), "revise")
	assert.NotContains(t, visible(m.View()), `["Agent-bob"]`)

	m = step(m, flagUpdate())
	assert.NotContains(t, visible(m.View()), "revise")
}

func roundState(round int, ids ...string) eventMsg {
	return eventMsg{session.RoundStateEvent{Round: round, IDs: ids}}
}

// A Round state sets the round (and the sidebar title) but is not a new round:
// the participant keeps their status and what they are typing.
func TestRoundState_SetsRoundWithoutResettingStatusOrResponseBox(t *testing.T) {
	m, status := submitted(t, "use postgres")
	m = typeRunes(m, "draft in progress")
	assert.Equal(t, hub.StatusTyping, status())

	m = step(m, roundState(3))

	assert.Equal(t, 3, m.round)
	assert.Contains(t, visible(m.View()), "Participants (Round 3")
	assert.Equal(t, hub.StatusTyping, status())
	assert.Equal(t, "draft in progress", m.response.Value())
}

// A participant who already submitted stays submitted through a Round state.
func TestRoundState_LeavesSubmittedStatusAlone(t *testing.T) {
	m, status := submitted(t, "use postgres")

	m = step(m, roundState(1))

	assert.Equal(t, hub.StatusSubmitted, status())
}

// Hearing the same flag again, as a repeat announcement can cause, does not
// tell the participant a second time.
func TestRoundState_RepeatDoesNotRenotifyAFlaggedParticipant(t *testing.T) {
	m, _ := submitted(t, "use postgres")

	m = step(m, roundState(1, "Agent-alice"))
	m = step(m, roundState(1, "Agent-alice"))

	assert.Equal(t, 1, strings.Count(visible(m.transcript.View()), "asked you to revise"))
}

// A participant who is newly flagged is told once, and the round is set.
func TestRoundState_NewlyFlaggedParticipantIsToldOnce(t *testing.T) {
	m, _ := submitted(t, "use postgres")

	m = step(m, roundState(2, "Agent-alice"))

	assert.Equal(t, 1, strings.Count(visible(m.transcript.View()), "asked you to revise"))
	assert.Contains(t, visible(m.View()), "Participants (Round 2")
}
