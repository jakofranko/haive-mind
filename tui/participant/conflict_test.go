package participant

import (
	"encoding/json"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
)

func flagUpdate(ids ...string) hubMessageMsg {
	b, _ := json.Marshal(ids)
	return hubMessageMsg(*hub.NewMessage(hub.MessageTypeConflictUpdate, "hub", string(b), 1))
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

	assert.Equal(t, 1, countOccurrences(visible(m.transcript.View()), "asked you to revise"))
}

func countOccurrences(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}

// Everyone sees who is flagged, and the announcement itself is not chat.
func TestFlagged_SidebarShowsReviseForFlaggedParticipants(t *testing.T) {
	h := hub.NewHub()
	h.SetDisplayName("alice", "Alice")
	h.RegisterParticipant(hub.NewParticipantEvent("Agent-bob", "Bobby", hub.StatusSubmitted, 1, hub.ParticipantEventConnect))
	h.RegisterParticipant(hub.NewParticipantEvent("Agent-alice", "Alice", hub.StatusIdle, 1, hub.ParticipantEventConnect))
	m := sized(NewParticipantModel(h, "Agent-alice", "alice"))
	m = step(m, initialParticipantsMsg(h.GetParticipants()))
	assert.NotContains(t, visible(m.View()), "revise")

	m = step(m, flagUpdate("Agent-bob"))
	assert.Contains(t, visible(m.View()), "revise")
	assert.NotContains(t, visible(m.View()), `["Agent-bob"]`)

	m = step(m, flagUpdate())
	assert.NotContains(t, visible(m.View()), "revise")
}
