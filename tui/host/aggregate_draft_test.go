package host

import (
	"context"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/session"
)

type noopRunner struct{}

func (noopRunner) Run(ctx context.Context, prompt string) (string, error) { return "", nil }

// sentRig is a started hub that records every message of the given types, so
// a test can see what the host model sends.
func sentRig(t *testing.T, types ...hub.MessageType) (hub.Hub, func() []*hub.Message) {
	t.Helper()
	h := hub.NewHub()
	h.SetAgentRunner(noopRunner{})
	h.SetSynthesizer(noopRunner{})
	h.SetDisplayName("host", "TestHost")
	var mu sync.Mutex
	var got []*hub.Message
	if _, err := h.SubscribeTypes(context.Background(), types, func(m *hub.Message) error {
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })
	return h, func() []*hub.Message {
		time.Sleep(150 * time.Millisecond) // let the hub dispatch
		mu.Lock()
		defer mu.Unlock()
		return append([]*hub.Message(nil), got...)
	}
}

func draftMsg(content string) eventMsg {
	return eventMsg{session.MessageEvent{Message: *hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", content, 1)}}
}

func typeText(m *AppModel, text string) *AppModel {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	return updated.(*AppModel)
}

func press(m *AppModel, k tea.KeyType) *AppModel {
	updated, _ := m.Update(tea.KeyMsg{Type: k})
	return updated.(*AppModel)
}

// G11: a synthesized draft opens in the host's input, the host edits it, and
// Enter sends the edited text as the aggregate (not as a directive).
func TestHost_DraftIsEditableAndSentAsAggregate(t *testing.T) {
	h, sent := sentRig(t, hub.MessageTypeAggregate, hub.MessageTypeDirective)
	m := NewAppModel(h)

	updated, _ := m.Update(draftMsg("Group picks Postgres"))
	m = updated.(*AppModel)

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
	h, sent := sentRig(t, hub.MessageTypeAggregate, hub.MessageTypeDirective)
	m := NewAppModel(h)

	updated, _ := m.Update(draftMsg("Group picks Postgres"))
	m = updated.(*AppModel)
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
