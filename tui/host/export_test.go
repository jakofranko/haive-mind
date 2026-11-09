package host

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"jakofranko/haive-mind/hub"
)

func startedHub(t *testing.T) hub.Hub {
	t.Helper()
	h := hub.NewHub()
	h.SetDisplayName("host", "TestHost")
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })
	return h
}

// When an export finishes the host's transcript says where the file went. The
// notice travels over a real hub and the model's thin wait command.
func TestHost_ExportCompleteShowsWhereTheFileWent(t *testing.T) {
	h := startedHub(t)
	m := NewAppModel(h)
	defer m.Close()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(*AppModel)

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeExportComplete, "hub", "out/transcript.json", 0))

	res := make(chan tea.Msg, 1)
	go func() { res <- m.waitForEvent()() }()
	select {
	case msg := <-res:
		updated, _ = m.Update(msg)
		m = updated.(*AppModel)
	case <-time.After(5 * time.Second):
		t.Fatal("no export notice arrived")
	}

	assert.Contains(t, visible(m.View()), "Exported to out/transcript.json")
}

// Closing the model ends a wait that is blocked on the next event.
func TestHost_CloseEndsAPendingWait(t *testing.T) {
	m := NewAppModel(startedHub(t))
	res := make(chan tea.Msg, 1)
	go func() { res <- m.waitForEvent()() }()
	time.Sleep(50 * time.Millisecond)
	m.Close()

	select {
	case msg := <-res:
		assert.Nil(t, msg)
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not release the pending wait")
	}
}
