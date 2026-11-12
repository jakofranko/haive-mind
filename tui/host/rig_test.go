package host

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jakofranko/haive-mind/coordinator"
	"github.com/jakofranko/haive-mind/hub"
)

type noopRunner struct{}

func (noopRunner) Run(ctx context.Context, prompt string) (string, error) { return "", nil }

// serverHub is a started hub with the Coordinator beside it, as the server
// runs them, and agent and synthesizer stubs so nothing launches opencode.
// The host has already chosen the name TestHost.
func serverHub(t *testing.T) (hub.Hub, *coordinator.Coordinator) {
	t.Helper()
	h := hub.NewHub()
	h.SetAgentRunner(noopRunner{})
	h.SetSynthesizer(noopRunner{})
	h.SetDisplayName(hub.HostID, "TestHost")
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Stop() })
	c := coordinator.New(h)
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return h, c
}

// recordSent keeps every message of the given types that reaches the hub, so
// a test can see what the host sends.
func recordSent(t *testing.T, h hub.Hub, types ...hub.MessageType) func() []*hub.Message {
	t.Helper()
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
	return func() []*hub.Message {
		time.Sleep(150 * time.Millisecond) // let the hub dispatch
		mu.Lock()
		defer mu.Unlock()
		return append([]*hub.Message(nil), got...)
	}
}

// joinHost is what the SSH server does for the host: it makes the model
// (whose Session client subscribes), then registers the host. It returns the
// model with a screen, once it has caught up.
func joinHost(t *testing.T, h hub.Hub) *AppModel {
	t.Helper()
	m := NewAppModel(h)
	t.Cleanup(m.Close)
	h.RegisterParticipant(hub.NewParticipantEvent(hub.HostID, "TestHost", hub.StatusIdle, hub.ParticipantEventConnect))
	m = receive(t, m) // Catch-up is always the first event
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	return updated.(*AppModel)
}

// addParticipant registers someone and waits until the host lists them.
func addParticipant(t *testing.T, m *AppModel, h hub.Hub, id, name string, status hub.ParticipantStatus) *AppModel {
	t.Helper()
	h.RegisterParticipant(hub.NewParticipantEvent(id, name, status, hub.ParticipantEventConnect))
	return receiveUntil(t, m, shows(name))
}

// nextMsg is the next event the model is given, as its program would get it.
func nextMsg(t *testing.T, m *AppModel) tea.Msg {
	t.Helper()
	res := make(chan tea.Msg, 1)
	go func() { res <- m.waitForEvent()() }()
	select {
	case msg := <-res:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("model was given no event")
		return nil
	}
}

// receive applies the next event through Update.
func receive(t *testing.T, m *AppModel) *AppModel {
	t.Helper()
	updated, _ := m.Update(nextMsg(t, m))
	return updated.(*AppModel)
}

// receiveUntil applies events until the screen shows want.
func receiveUntil(t *testing.T, m *AppModel, want func(screen string) bool) *AppModel {
	t.Helper()
	for !want(visible(m.View())) {
		m = receive(t, m)
	}
	return m
}

func shows(text string) func(string) bool {
	return func(screen string) bool { return strings.Contains(screen, text) }
}

func typeText(m *AppModel, text string) *AppModel {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	return updated.(*AppModel)
}

func press(m *AppModel, k tea.KeyType) *AppModel {
	updated, _ := m.Update(tea.KeyMsg{Type: k})
	return updated.(*AppModel)
}

// run feeds a command's message back into the model, as Bubble Tea would.
func run(m *AppModel, cmd tea.Cmd) *AppModel {
	if cmd == nil {
		return m
	}
	msg := runBounded(cmd)
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = run(m, c)
		}
		return m
	}
	if msg == nil {
		return m
	}
	updated, next := m.Update(msg)
	return run(updated.(*AppModel), next)
}

// runBounded executes a command, giving up on one that waits (a timer, such as
// the toast's auto-hide, or the wait for the next event) rather than blocking
// the test.
func runBounded(cmd tea.Cmd) tea.Msg {
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		return msg
	case <-time.After(150 * time.Millisecond):
		return nil
	}
}
