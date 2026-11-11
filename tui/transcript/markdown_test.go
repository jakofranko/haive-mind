package transcript

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"jakofranko/haive-mind/hub"
)

// Message bodies are rendered as Markdown, and the header shows the sender's
// name plainly (not wrapped in literal asterisks).
func TestTranscript_RendersMarkdownBodyAndPlainHeader(t *testing.T) {
	m := New("welcome")
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
	m := New("welcome")
	m.SetSize(80, 24)
	m.AddMessage(hub.Message{From: "Alice", Content: "hi", Timestamp: time.Now(), Round: 1})

	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "Alice") && (strings.Contains(line, "Waiting for messages") || strings.Contains(line, "Press Tab")) {
			t.Errorf("welcome and first message share a line: %q", line)
		}
	}
}

// Emphasis in a message is rendered, not shown as raw Markdown markers. This
// must not depend on whether the server itself is attached to a terminal.
func TestTranscript_RendersEmphasisInTheBody(t *testing.T) {
	m := New("welcome")
	m.SetSize(80, 24)
	m.AddMessage(hub.Message{From: "Alice", Content: "use **Postgres** here", Timestamp: time.Now(), Round: 1})

	view := stripANSI(m.View())
	if strings.Contains(view, "**") {
		t.Errorf("bold shown as raw Markdown:\n%s", view)
	}
	if !strings.Contains(view, "Postgres") {
		t.Errorf("body text missing:\n%s", view)
	}
}

var ansiEscapes = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func stripANSI(s string) string { return ansiEscapes.ReplaceAllString(s, "") }
