package participant

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/stretchr/testify/assert"
)

const notice = "asked you to revise"

func notices(m *ParticipantModel) int {
	return strings.Count(visible(m.transcript.View()), notice)
}

// G12: when the host flags this participant they are told, their last answer
// comes back for editing, and they are idle again.
func TestFlagged_GetsNoticeAndTheirLastAnswerBack(t *testing.T) {
	m, h := submitted(t, "use postgres")
	assert.Equal(t, "", m.response.Value())

	flag(h, 1, "Agent-alice")
	m = receiveUntil(t, m, shows(notice))

	assert.Equal(t, "use postgres", m.response.Value())
	assert.Equal(t, hub.StatusIdle, statusOf(t, h, "Agent-alice")())
}

// Text they have already started typing is left alone.
func TestFlagged_DoesNotOverwriteWhatTheyAreTyping(t *testing.T) {
	m, h := submitted(t, "use postgres")
	m = typeRunes(m, "on second thought, sqlite")

	flag(h, 1, "Agent-alice")
	m = receiveUntil(t, m, shows(notice))

	assert.Equal(t, "on second thought, sqlite", m.response.Value())
}

// Everyone sees who is flagged, someone else's flag is not this participant's
// notice, and the announcement itself is not chat.
func TestFlagged_SomeoneElse_ShowsReviseButNoNotice(t *testing.T) {
	m, h := submitted(t, "use postgres")
	join(t, h, "Agent-bob", "Bob")
	m = receiveUntil(t, m, shows("Bob"))
	assert.NotContains(t, visible(m.View()), "revise")

	flag(h, 1, "Agent-bob")
	m = receiveUntil(t, m, shows("revise"))

	assert.NotContains(t, visible(m.View()), notice)
	assert.NotContains(t, visible(m.View()), `["Agent-bob"]`)
	assert.Equal(t, "", m.response.Value())

	h.Send(context.Background(), hub.NewUnflag(1, "Agent-bob"))
	m = receiveUntil(t, m, func(s string) bool { return !strings.Contains(s, "revise") })
}

// The notice appears when they become flagged, not on every Round state.
func TestFlagged_NoticeIsShownOncePerFlag(t *testing.T) {
	m, h := submitted(t, "use postgres")

	flag(h, 1, "Agent-alice")
	m = receiveUntil(t, m, shows(notice))
	join(t, h, "Agent-bob", "Bob") // its connect repeats the Round state to everyone
	flag(h, 1, "Agent-bob")
	m = receiveUntil(t, m, func(s string) bool { return strings.Count(s, "revise") >= 3 }) // two rows, one notice

	assert.Equal(t, 1, notices(m))
}

// #53, #70: a participant whose connection drops while flagged is told once in
// their new session, and gets their answer back.
func TestFlagged_ReconnectingSessionIsToldOnceWithTheirAnswer(t *testing.T) {
	first, h := submitted(t, "use postgres")
	flag(h, 1, "Agent-alice")
	receiveUntil(t, first, shows(notice))
	first.Close()
	h.UnregisterParticipant("Agent-alice")

	m := join(t, h, "Agent-alice", "Alice")
	m = receiveUntil(t, m, shows(notice))
	assert.Equal(t, "use postgres", m.response.Value())

	join(t, h, "Agent-bob", "Bob") // its connect repeats the Round state
	m = receiveUntil(t, m, shows("Bob"))
	assert.Equal(t, 1, notices(m))
}

// A Round state is not a new round: a participant who is typing keeps their
// status and their text.
func TestRoundState_LeavesStatusAndResponseBoxAlone(t *testing.T) {
	m, h := submitted(t, "use postgres")
	status := statusOf(t, h, "Agent-alice")
	m = typeRunes(m, "draft in progress")

	join(t, h, "Agent-bob", "Bob") // its connect repeats the Round state
	m = receiveUntil(t, m, shows("Bob"))

	assert.Equal(t, hub.StatusTyping, status())
	assert.Equal(t, "draft in progress", m.response.Value())
}

// #74: the answer offered back is for the current round. A participant who
// answered round 1 and is flagged in round 2 before answering is told, but
// not handed their round 1 answer.
func TestFlagged_DoesNotOfferAnAnswerFromAnEarlierRound(t *testing.T) {
	m, h := submitted(t, "use postgres")

	h.StartRound(1)
	m = receiveUntil(t, m, shows("Participants (Round 2"))
	flag(h, 2, "Agent-alice")
	m = receiveUntil(t, m, shows(notice))

	assert.Equal(t, 1, notices(m))
	assert.Equal(t, "", m.response.Value())
}

// Submitting takes the box's text; a blank box sends nothing.
func TestSubmit_BlankBoxSendsNothing(t *testing.T) {
	h := serverHub(t)
	m := join(t, h, "Agent-alice", "Alice")

	m = typeRunes(m, "   ")
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})

	assert.Equal(t, "   ", m.response.Value())
	assert.NotEqual(t, hub.StatusSubmitted, statusOf(t, h, "Agent-alice")())
}
