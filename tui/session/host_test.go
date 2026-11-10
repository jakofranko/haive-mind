package session_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"jakofranko/haive-mind/hub"
	"jakofranko/haive-mind/tui/session"
)

// connectHost is what the SSH server does for the host: the session's client
// subscribes, then the host is registered.
func connectHost(t *testing.T, h hub.Hub) *session.Host {
	t.Helper()
	c := session.NewHost(h)
	t.Cleanup(c.Close)
	select {
	case <-c.Ready():
	case <-time.After(wait):
		t.Fatal("host client never became ready")
	}
	h.RegisterParticipant(hub.NewParticipantEvent(hub.HostID, "Host", hub.StatusIdle, hub.ParticipantEventConnect))
	return c
}

// recorder keeps every message of some types that reaches the hub.
type recorder struct {
	mu  sync.Mutex
	got []*hub.Message
}

func record(t *testing.T, h hub.Hub, types ...hub.MessageType) *recorder {
	t.Helper()
	r := &recorder{}
	if _, err := h.SubscribeTypes(context.Background(), types, func(m *hub.Message) error {
		r.mu.Lock()
		r.got = append(r.got, m)
		r.mu.Unlock()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

// wantN waits for n messages and returns them.
func (r *recorder) wantN(t *testing.T, n int) []*hub.Message {
	t.Helper()
	var got []*hub.Message
	eventually(t, "the messages reach the hub", func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		got = append([]*hub.Message{}, r.got...)
		return len(got) >= n
	})
	return got
}

type failingRunner struct{}

func (failingRunner) Run(context.Context, string) (string, error) { return "", errors.New("boom") }

// The host is caught up like anyone else: Catch-up first, with the round and
// who is here.
func TestHostCatchUp_IsTheFirstEvent(t *testing.T) {
	h := coordinatedHub(t)
	h.RegisterParticipant(hub.NewParticipantEvent("Agent-alice", "Alice", hub.StatusSubmitted, hub.ParticipantEventConnect))
	startRound(t, h, 1)

	c := connectHost(t, h)

	ev := expect[session.CatchUpEvent](t, c)
	if ev.Round != 2 || len(ev.Participants) != 2 {
		t.Errorf("Catch-up = round %d with %+v, want round 2 with Alice and the host", ev.Round, ev.Participants)
	}
}

// A failed agent call reaches the host as a failure it can retry; the retry
// sends the same directive again, for the same round, as the host.
func TestHostAgentFailure_RetrySendsTheDirectiveAgain(t *testing.T) {
	h := coordinatedHub(t)
	h.SetAgentRunner(failingRunner{})
	directives := record(t, h, hub.MessageTypeDirective)
	c := connectHost(t, h)
	expect[session.CatchUpEvent](t, c)
	c.Rename("Hope")

	c.SendDirective("pick a database")
	failed := until[session.AgentFailedEvent](t, c)
	if !strings.Contains(failed.Text, "boom") || !failed.CanRetry {
		t.Fatalf("failure = %+v, want the error and a retry", failed)
	}
	if err := c.Retry(failed); err != nil {
		t.Fatal(err)
	}

	got := directives.wantN(t, 2)
	again := got[1]
	if again.Content != "pick a database" || again.From != "Hope" || again.FromID != hub.HostID || again.Round != 1 {
		t.Errorf("retry = {Content:%q From:%q FromID:%q Round:%d}, want the directive again from Hope in round 1", again.Content, again.From, again.FromID, again.Round)
	}
}

// A synthesized draft is the host's to edit, not just a transcript entry.
func TestHostDraft_IsItsOwnEvent(t *testing.T) {
	h := coordinatedHub(t)
	c := connectHost(t, h)
	expect[session.CatchUpEvent](t, c)

	h.Send(context.Background(), hub.NewMessage(hub.MessageTypeAggregateDraft, "synthesizer", "Group picks Postgres", 1))

	if ev := until[session.DraftEvent](t, c); ev.Message.Content != "Group picks Postgres" {
		t.Errorf("draft = %q", ev.Message.Content)
	}
}

// An agent failure or finished export that arrives before Catch-up is not
// lost: it follows Catch-up.
func TestHostCatchUp_EarlyFailureAndExportFollowIt(t *testing.T) {
	h := startedHub(t)
	c := session.NewHost(h)
	t.Cleanup(c.Close)
	<-c.Ready()
	ctx := context.Background()
	failure := hub.NewMessage(hub.MessageTypeAgentError, "hub", "Agent error: boom", 1)
	failure.Retry = &hub.Retry{Type: hub.MessageTypeDirective, Content: "ask"}

	h.Send(ctx, failure)
	h.Send(ctx, hub.NewMessage(hub.MessageTypeExportComplete, "hub", "out/transcript.json", 1))
	h.Send(ctx, hub.NewTranscriptSnapshot(1, hub.HostID, nil, nil))

	expect[session.CatchUpEvent](t, c)
	if ev := expect[session.AgentFailedEvent](t, c); ev.Text != "Agent error: boom" {
		t.Errorf("failure = %q", ev.Text)
	}
	if ev := expect[session.ExportEvent](t, c); ev.Path != "out/transcript.json" {
		t.Errorf("export = %q", ev.Path)
	}
}

// Every command carries the current round and the host's identity; the
// caller supplies neither.
func TestHostCommands_StampRoundAndIdentity(t *testing.T) {
	h := coordinatedHub(t)
	sent := record(t, h, hub.MessageTypeDirective, hub.MessageTypeAggregate, hub.MessageTypeFlag,
		hub.MessageTypeUnflag, hub.MessageTypeResolve, hub.MessageTypeAggregateRequest, hub.MessageTypeNewRound)
	startRound(t, h, 1)
	c := connectHost(t, h)
	expect[session.CatchUpEvent](t, c)
	c.Rename("Hope")

	if c.SendDirective("   ") {
		t.Error("a blank directive was sent")
	}
	c.SendDirective(" pick a database ")
	c.SendAggregate("Group picks Postgres")
	c.Flag(hub.HostID) // the host cannot flag themselves: nothing is sent
	c.Flag("Agent-alice")
	c.Unflag("Agent-alice")
	c.Resolve()
	c.RequestAggregate()
	c.NewRound()

	got := sent.wantN(t, 8)
	time.Sleep(100 * time.Millisecond) // negative check: nothing more arrives
	got = sent.wantN(t, 8)
	if len(got) != 8 {
		t.Fatalf("sent %d messages, want 8 (startRound's request plus seven commands)", len(got))
	}
	for _, m := range got[1:] {
		if m.Round != 2 {
			t.Errorf("%s sent for round %d, want 2", m.Type, m.Round)
		}
	}
	if d := got[1]; d.Content != "pick a database" || d.From != "Hope" || d.FromID != hub.HostID {
		t.Errorf("directive = {Content:%q From:%q FromID:%q}", d.Content, d.From, d.FromID)
	}
	if a := got[2]; a.Type != hub.MessageTypeAggregate || a.From != "Hope" {
		t.Errorf("aggregate = %+v", a)
	}
	if f := got[3]; f.Type != hub.MessageTypeFlag || f.To != "Agent-alice" {
		t.Errorf("flag = %+v", f)
	}
}
