package host

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/hub"
	"github.com/stretchr/testify/assert"
)

// When an export finishes the host's transcript says where the file went. The
// notice travels over a real hub and the model's thin wait command.
func TestHost_ExportCompleteShowsWhereTheFileWent(t *testing.T) {
	h, _ := serverHub(t)
	m := joinHost(t, h)

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeExportComplete, "hub", "out/transcript.json", 0))

	receiveUntil(t, m, shows("Exported to out/transcript.json"))
}

// Closing the model ends a wait on the next event. The wait command exposes no
// signal that it has begun blocking, so the test does not try to order the two:
// Close releases a wait already blocked in Next, and a wait that starts after
// Close sees a closed client and returns at once. Both orderings must yield nil.
func TestHost_CloseEndsAPendingWait(t *testing.T) {
	h, _ := serverHub(t)
	m := NewAppModel(h)
	res := make(chan tea.Msg, 1)
	go func() { res <- m.waitForEvent()() }()
	m.Close()

	select {
	case msg := <-res:
		assert.Nil(t, msg)
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not release the pending wait")
	}
}
