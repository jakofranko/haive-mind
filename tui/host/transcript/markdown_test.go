package transcript

import (
	"strings"
	"testing"
	"time"

	"jakofranko/haive-mind/hub"
)

// Message bodies are rendered as Markdown, and the header shows the sender's
// name plainly (not wrapped in literal asterisks).
func TestTranscript_RendersMarkdownBodyAndPlainHeader(t *testing.T) {
	m := New()
	m.SetSize(80, 24)
	m.AddMessage(hub.Message{From: "Alice", Content: "- fast\n- safe", Timestamp: time.Now(), Round: 1})

	view := m.View()
	if !strings.Contains(view, "•") {
		t.Errorf("list was not rendered as Markdown:\n%s", view)
	}
	if strings.Contains(view, "**Alice**") {
		t.Errorf("sender shown with literal asterisks:\n%s", view)
	}
	if !strings.Contains(view, "Alice") {
		t.Errorf("sender missing:\n%s", view)
	}
}

// The welcome text and the first message do not run together on one line.
func TestTranscript_FirstMessageStartsOnItsOwnLine(t *testing.T) {
	m := New()
	m.SetSize(80, 24)
	m.AddMessage(hub.Message{From: "Alice", Content: "hi", Timestamp: time.Now(), Round: 1})

	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "Alice") && (strings.Contains(line, "Waiting for messages") || strings.Contains(line, "Press Tab")) {
			t.Errorf("welcome and first message share a line: %q", line)
		}
	}
}
