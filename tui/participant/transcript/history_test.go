package transcript

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
)

// G22: messages that have scrolled out of view are still in the transcript
// and can be scrolled back to.
func TestTranscript_KeepsHistoryBeyondTheVisibleWindow(t *testing.T) {
	m := New()
	m.SetSize(80, 12)
	for i := 0; i < 30; i++ {
		m.AddMessage(hub.Message{From: "p", Content: fmt.Sprintf("marker-%02d", i), Timestamp: time.Now(), Round: 1})
	}

	var model tea.Model = m
	for i := 0; i < 60; i++ { // page all the way back up
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	}

	if view := model.View(); !strings.Contains(view, "marker-00") {
		t.Errorf("after scrolling to the top, first message is missing:\n%s", view)
	}
}

// An empty snapshot changes nothing, including where the reader has scrolled to.
func TestTranscript_PrependingNothingKeepsTheView(t *testing.T) {
	m := New()
	m.SetSize(80, 12)
	for i := 0; i < 30; i++ {
		m.AddMessage(hub.Message{From: "p", Content: fmt.Sprintf("marker-%02d", i), Timestamp: time.Now(), Round: 1})
	}
	var model tea.Model = m
	for i := 0; i < 60; i++ {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	}
	before := model.View()

	m = model.(Model)
	m.PrependMessages(nil)

	if after := m.View(); after != before {
		t.Errorf("view changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
