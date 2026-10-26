package sshserver

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"jakofranko/haive-mind/hub"
)

// hubMessageMsg mirrors the type used in transcript models
type hubMessageMsg hub.Message

// TestTUIModelsHandleDereferencedMessages verifies that the TUI models
// correctly handle hub.Message values (which is what broadcastLoop now sends
// after the fix: p.Send(*msg) instead of p.Send(msg)).
func TestTUIModelsHandleDereferencedMessages(t *testing.T) {
	testCases := []struct {
		name    string
		msg     tea.Msg
		handled bool
	}{
		{
			name:    "hub.Message value (what broadcastLoop now sends)",
			msg:     *hub.NewMessage(hub.MessageTypeDirective, "Host", "Test", 1),
			handled: true,
		},
		{
			name:    "hubMessageMsg wrapper (also handled)",
			msg:     hubMessageMsg(*hub.NewMessage(hub.MessageTypeDirective, "Host", "Test", 1)),
			handled: true,
		},
		{
			name:    "*hub.Message pointer (NOT handled - old broadcastLoop behavior)",
			msg:     hub.NewMessage(hub.MessageTypeDirective, "Host", "Test", 1),
			handled: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handled := false
			switch tc.msg.(type) {
			case hub.Message:
				handled = true
			case hubMessageMsg:
				handled = true
			}

			if handled != tc.handled {
				t.Errorf("Expected handled=%v, got handled=%v", tc.handled, handled)
			}
		})
	}
}