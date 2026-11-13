package e2e

import (
	"os"
	"strings"
	"testing"
)

// #107: while a slow synthesis runs, the host and a participant both see it
// as pending, and pressing Aggregate again does not start a second one.
func TestPendingSynthesisShownToEveryoneAndNotRepeated(t *testing.T) {
	addr, _, agentLog := startServerWith(t, nil,
		[]string{"FAKE_AGENT_SLOW_PROMPT=Several", "FAKE_AGENT_SLOW_FOR=4s"})

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")
	bob := connect(t, addr, "bob")
	bob.joinAs(t, "Bobby")

	host.send(t, "\t\t")
	host.send(t, "hello\r")
	bob.waitFor(t, "AGENTREPLY-hello")
	bob.send(t, "\t\t")
	bob.send(t, "bobsays-sqlite\r")
	host.waitFor(t, "bobsays-sqlite")

	host.send(t, "\t") // input -> buttons, on [New Round]
	host.send(t, "h")
	host.send(t, "h")
	host.send(t, "\r") // [Aggregate]
	for _, tm := range []*term{host, bob} {
		tm.waitFor(t, "Synthesizing the round 1 aggregate…")
	}
	host.send(t, "\r") // a second press while it runs

	host.waitFor(t, "AGENTREPLY-Several")
	b, _ := os.ReadFile(agentLog)
	if n := strings.Count(string(b), "Several"); n != 1 {
		t.Errorf("synthesizer invoked %d times, want 1:\n%s", n, b)
	}
}
