// Package e2e drives the real haive-mind binary over SSH, the way a host and
// participants would, with a fake `opencode` standing in for the agent.
package e2e

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07`)

// term is one SSH session with a pseudo-terminal, as a user would have.
type term struct {
	name string
	mu   sync.Mutex
	buf  bytes.Buffer
	in   io.WriteCloser
	// disconnect ends this SSH session, as a dropped connection would.
	disconnect func()
}

func (tm *term) screen() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return ansi.ReplaceAllString(tm.buf.String(), "")
}

// seen reports whether text has appeared on this terminal at any point.
func (tm *term) seen(text string) bool { return strings.Contains(tm.screen(), text) }

func (tm *term) waitFor(t *testing.T, text string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if tm.seen(text) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("%s: timed out waiting for %q. Screen so far:\n%s", tm.name, text, tail(tm.screen(), 3000))
}

func (tm *term) send(t *testing.T, s string) {
	t.Helper()
	if _, err := io.WriteString(tm.in, s); err != nil {
		t.Fatalf("%s: write: %v", tm.name, err)
	}
	time.Sleep(60 * time.Millisecond) // let the TUI process the keystrokes
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func connect(t *testing.T, addr, user string) *term {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	client, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("%s: dial: %v", user, err)
	}
	sess, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.RequestPty("xterm-256color", 40, 140, ssh.TerminalModes{ssh.ECHO: 1}); err != nil {
		t.Fatal(err)
	}
	tm := &term{name: user}
	out, _ := sess.StdoutPipe()
	tm.in, _ = sess.StdinPipe()
	if err := sess.Shell(); err != nil {
		t.Fatal(err)
	}
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := out.Read(b)
			if n > 0 {
				tm.mu.Lock()
				tm.buf.Write(b[:n])
				tm.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	tm.disconnect = func() { sess.Close(); client.Close() }
	t.Cleanup(tm.disconnect)
	return tm
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(file))
}

func goBuild(t *testing.T, root, out, pkg string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, b)
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// startServer builds and launches haive-mind with a fake opencode first on
// PATH, and returns its address and data directory.
func startServer(t *testing.T) (addr, dataDir, agentLog string) {
	return startServerWith(t, nil, nil)
}

// startServerWith adds command-line flags and environment to the server.
func startServerWith(t *testing.T, flags, env []string) (addr, dataDir, agentLog string) {
	t.Helper()
	root := repoRoot(t)
	bin := t.TempDir()
	goBuild(t, root, filepath.Join(bin, exe("haive-mind")), "./cmd/haive-mind")
	goBuild(t, root, filepath.Join(bin, exe("opencode")), "./e2e/fakeagent")

	port := freePort(t)
	dataDir = t.TempDir()
	agentLog = filepath.Join(t.TempDir(), "agent.log")
	var logs bytes.Buffer
	args := append([]string{
		"--port", port,
		"--host-key", filepath.Join(t.TempDir(), "host_key"),
		"--data-dir", dataDir, "--debug"}, flags...)
	cmd := exec.Command(filepath.Join(bin, exe("haive-mind")), args...)
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_AGENT_LOG="+agentLog)
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
		if t.Failed() {
			t.Logf("server output:\n%s", tail(logs.String(), 4000))
			b, _ := os.ReadFile(agentLog)
			t.Logf("fake agent was invoked with:\n%s", b)
		}
	})

	addr = "127.0.0.1:" + port
	for i := 0; i < 100; i++ {
		if c, err := net.Dial("tcp", addr); err == nil {
			c.Close()
			return addr, dataDir, agentLog
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("server never started listening")
	return
}

// joinAs completes the first-connect display-name prompt.
func (tm *term) joinAs(t *testing.T, display string) {
	t.Helper()
	tm.waitFor(t, "Enter display name")
	tm.send(t, display+"\r")
	tm.waitFor(t, "Participants (Round")
}

// TestFullRound runs the story from ticket #10: the host directs the agent,
// two participants respond, the host aggregates, and the agent replies again.
func TestFullRound(t *testing.T) {
	addr, dataDir, agentLog := startServer(t)

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")
	alice := connect(t, addr, "alice")
	alice.joinAs(t, "Alice")
	bob := connect(t, addr, "bob")
	bob.joinAs(t, "Bobby")

	// Host sends a directive: Tab transcript -> participants -> directive.
	host.send(t, "\t\t")
	host.send(t, "hello\r")
	for _, tm := range []*term{host, alice, bob} {
		tm.waitFor(t, "AGENTREPLY-hello")
	}

	// Participants respond: Tab transcript -> sidebar -> response.
	alice.send(t, "\t\t")
	alice.send(t, "alicesays-postgres")
	host.waitFor(t, "Alice  typing") // the host's list follows what participants are doing
	alice.send(t, "\r")
	host.waitFor(t, "Alice  submitted")
	bob.send(t, "\t\t")
	bob.send(t, "bobsays-sqlite\r")

	// Everyone sees both responses, including each other's.
	for _, tm := range []*term{host, alice, bob} {
		tm.waitFor(t, "alicesays-postgres")
		tm.waitFor(t, "bobsays-sqlite")
	}

	// Host aggregates: Tab on to the buttons; [Aggregate] is first. The
	// synthesis comes back as a draft that everyone sees.
	host.send(t, "\t")
	host.send(t, "\r")
	for _, tm := range []*term{host, alice, bob} {
		tm.waitFor(t, "AGENTREPLY-Several")
	}

	// The draft opens in the host's input; the host edits it and sends it.
	host.send(t, " (edited)\r")
	for _, tm := range []*term{host, alice, bob} {
		tm.waitFor(t, "AGENTREPLY-AGENTREPLY-Several")
	}

	// Host exports (button 4: three moves right) and the file is valid JSON
	// containing the whole round.
	host.send(t, "\t") // the host is still in the input pane after sending
	for i := 0; i < 3; i++ { // one key event per press; "lll" would be a single event
		host.send(t, "l")
	}
	host.send(t, "\r")
	host.waitFor(t, "Exported to")

	files, _ := filepath.Glob(filepath.Join(dataDir, "transcript_*.json"))
	if len(files) != 1 {
		t.Fatalf("expected one exported transcript in %s, got %v", dataDir, files)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Messages []struct{ Content string }
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("exported transcript is not valid JSON: %v", err)
	}
	for _, want := range []string{"hello", "AGENTREPLY-hello", "alicesays-postgres", "bobsays-sqlite", "AGENTREPLY-AGENTREPLY-Several"} {
		found := false
		for _, m := range doc.Messages {
			if strings.Contains(m.Content, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("exported transcript has no message containing %q", want)
		}
	}

	// Every call reached its runner exactly once: the directive, one
	// synthesis of both responses, and the host's edited aggregate.
	b, err := os.ReadFile(agentLog)
	if err != nil {
		t.Fatal(err)
	}
	var prompts []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		p, err := strconv.Unquote(line)
		if err != nil {
			t.Fatalf("bad agent log line %q: %v", line, err)
		}
		prompts = append(prompts, p)
	}
	if len(prompts) != 3 {
		t.Fatalf("fake opencode invoked %d times, want 3: %q", len(prompts), prompts)
	}
	if prompts[0] != "hello" {
		t.Errorf("prompt 0 = %q, want the directive", prompts[0])
	}
	for _, want := range []string{"hello", "Alice: alicesays-postgres", "Bobby: bobsays-sqlite"} {
		if !strings.Contains(prompts[1], want) {
			t.Errorf("synthesis prompt lacks %q: %s", want, prompts[1])
		}
	}
	if prompts[2] != "AGENTREPLY-Several (edited)" {
		t.Errorf("aggregate sent to agent = %q, want the host's edited draft", prompts[2])
	}
}

// G14: text a participant has typed but not sent is waiting for them when they
// reconnect.
func TestDraftSurvivesReconnect(t *testing.T) {
	addr, _, _ := startServer(t)

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")
	alice := connect(t, addr, "alice")
	alice.joinAs(t, "Alice")

	alice.send(t, "\t\t") // transcript -> sidebar -> response
	alice.send(t, "half-formed-thought")
	host.waitFor(t, "Alice  typing")

	alice.disconnect()
	time.Sleep(300 * time.Millisecond) // let the server notice

	back := connect(t, addr, "alice") // same SSH user: no name prompt this time
	back.waitFor(t, "half-formed-thought")
}

// G15 + G16: an agent that takes too long is cut off at --agent-timeout, the
// host is told with a Retry, and Retry gets the answer once the agent behaves.
func TestAgentTimeoutThenRetry(t *testing.T) {
	addr, _, agentLog := startServerWith(t,
		[]string{"--agent-timeout", "2s"},
		[]string{"FAKE_AGENT_HANG_FIRST=1"})

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")

	host.send(t, "\t\t")
	host.send(t, "hello\r")
	host.waitFor(t, "agent timed out") // the toast
	host.waitFor(t, "Retry")

	host.send(t, "\r") // Retry
	host.waitFor(t, "AGENTREPLY-hello")

	b, _ := os.ReadFile(agentLog)
	if n := strings.Count(string(b), "\n"); n != 2 {
		t.Errorf("agent invoked %d times, want 2 (the timeout and the retry):\n%s", n, b)
	}
}

// count reports how many times text has appeared on this terminal.
func (tm *term) count(text string) int { return strings.Count(tm.screen(), text) }

// G12: the host flags Alice, who is told and gets her answer back; Aggregate
// warns while she is flagged; once she resubmits, only her new answer is
// synthesized.
func TestHostAsksForARevision(t *testing.T) {
	addr, _, agentLog := startServer(t)

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")
	alice := connect(t, addr, "alice")
	alice.joinAs(t, "Alice")
	bob := connect(t, addr, "bob")
	bob.joinAs(t, "Bobby")

	host.send(t, "\t\t")
	host.send(t, "hello\r")
	alice.waitFor(t, "AGENTREPLY-hello")

	alice.send(t, "\t\t")
	alice.send(t, "alice-first\r")
	bob.send(t, "\t\t")
	bob.send(t, "bobsays-sqlite\r")
	host.waitFor(t, "alice-first")
	host.waitFor(t, "bobsays-sqlite")

	// The host selects Alice's row (host, Alice, Bobby) and flags her.
	aliceTypedBefore := alice.count("alice-first")
	host.send(t, "\x1b[Z") // back from the input to the participants list
	host.send(t, "j")
	host.send(t, "c")
	host.waitFor(t, "revise")
	alice.waitFor(t, "asked you to revise")
	if alice.count("alice-first") <= aliceTypedBefore {
		t.Errorf("Alice's earlier answer did not come back into her response box")
	}

	// Aggregating while she is flagged asks first; the host backs out.
	host.send(t, "\t\t") // participants -> input -> buttons; [Aggregate] is first
	host.send(t, "\r")
	host.waitFor(t, "still flagged")
	host.send(t, "\x1b")

	// Alice revises and resubmits; that lifts her flag.
	alice.send(t, "-revised\r")
	bob.waitFor(t, "alice-first-revised")
	time.Sleep(300 * time.Millisecond)

	host.send(t, "\r") // [Aggregate] again: no warning now
	host.waitFor(t, "AGENTREPLY-Several")

	b, _ := os.ReadFile(agentLog)
	var prompts []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		p, err := strconv.Unquote(line)
		if err != nil {
			t.Fatalf("bad agent log line %q: %v", line, err)
		}
		prompts = append(prompts, p)
	}
	if len(prompts) != 2 {
		t.Fatalf("agent invoked %d times, want 2 (directive, one synthesis): %q", len(prompts), prompts)
	}
	synthesis := prompts[1]
	if !strings.Contains(synthesis, "Alice: alice-first-revised") || !strings.Contains(synthesis, "Bobby: bobsays-sqlite") {
		t.Errorf("synthesis lacks the current answers: %s", synthesis)
	}
	if strings.Contains(synthesis, "Alice: alice-first\n") || strings.HasSuffix(synthesis, "Alice: alice-first") {
		t.Errorf("synthesis still has Alice's superseded answer: %s", synthesis)
	}
}
