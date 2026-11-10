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

var (
	buildOnce sync.Once
	buildDir  string
	buildErr  error
)

// binaries builds haive-mind and the fake opencode once per test run and
// returns the directory holding them. The directory is the same on every run,
// so an operating system that remembers approvals per program path (the
// Windows firewall) sees the same two programs each time rather than a new
// pair per test.
func binaries(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		cache, err := os.UserCacheDir()
		if err != nil {
			cache = os.TempDir()
		}
		buildDir = filepath.Join(cache, "haive-mind-e2e")
		if buildErr = os.MkdirAll(buildDir, 0o755); buildErr != nil {
			return
		}
		root := repoRoot(t)
		for out, pkg := range map[string]string{
			exe("haive-mind"): "./cmd/haive-mind",
			exe("opencode"):   "./e2e/fakeagent",
		} {
			// The test cache cannot see what this build reads, so run e2e
			// uncached (-count=1, or scripts/check.sh --full).
			cmd := exec.Command("go", "build", "-o", filepath.Join(buildDir, out), pkg)
			cmd.Dir = root
			if b, err := cmd.CombinedOutput(); err != nil {
				buildErr = fmt.Errorf("go build %s: %v\n%s", pkg, err, b)
				return
			}
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return buildDir
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
	if testing.Short() {
		t.Skip("builds and runs the real binary; skipped with -short")
	}
	bin := binaries(t)

	port := freePort(t)
	dataDir = t.TempDir()
	agentLog = filepath.Join(t.TempDir(), "agent.log")
	var logs bytes.Buffer
	args := append([]string{
		"--bind", "127.0.0.1", "--port", port,
		"--host-key", filepath.Join(t.TempDir(), "host_key"),
		"--data-dir", dataDir, "--debug"}, flags...)
	cmd := exec.Command(filepath.Join(bin, exe("haive-mind")), args...)
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_AGENT_LOG="+agentLog)
	cmd.Env = append(cmd.Env, env...)
	prepareInterruptable(cmd)
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
	servers.Store(addr, cmd)
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
	host.send(t, "\t")       // the host is still in the input pane after sending
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
		Messages []struct{ Type, Content string }
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("exported transcript is not valid JSON: %v", err)
	}
	agentRepliesChecked := 0
	for _, want := range []string{"hello", "AGENTREPLY-hello", "alicesays-postgres", "bobsays-sqlite", "AGENTREPLY-AGENTREPLY-Several"} {
		found := false
		for _, m := range doc.Messages {
			if strings.Contains(m.Content, want) {
				found = true
				if strings.HasPrefix(want, "AGENTREPLY-") {
					agentRepliesChecked++
					if m.Type != "agent_reply" {
						t.Errorf("exported agent message %q has type %q, want agent_reply", want, m.Type)
					}
				}
			}
		}
		if !found {
			t.Errorf("exported transcript has no message containing %q", want)
		}
	}
	if agentRepliesChecked == 0 {
		t.Error("exported transcript has no agent_reply message to check")
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

// #53, #67: a participant who joins after a new round started sees that round,
// not round 1, and the directive and aggregate already exchanged.
func TestLateJoinerSeesCurrentRound(t *testing.T) {
	addr, _, _ := startServer(t)

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")
	bob := connect(t, addr, "bob")
	bob.joinAs(t, "Bobby")

	startNewRound(t, host)
	directAndAggregate(t, host, bob)

	alice := connect(t, addr, "alice")
	alice.joinAs(t, "Alice")
	alice.waitFor(t, "Participants (Round 2")
	alice.waitFor(t, "AGENTREPLY-hello")
	alice.waitFor(t, "bobsays-sqlite")
	alice.waitFor(t, "AGENTREPLY-Several")
}

// directAndAggregate has the host (focus on the buttons, as startNewRound
// leaves it) direct the agent, bob answer, and the host aggregate and send the
// draft.
func directAndAggregate(t *testing.T, host, bob *term) {
	t.Helper()
	host.send(t, "\t\t\t") // buttons -> transcript -> participants -> input
	host.send(t, "hello\r")
	bob.waitFor(t, "AGENTREPLY-hello")
	bob.send(t, "\t\t")
	bob.send(t, "bobsays-sqlite\r")
	host.waitFor(t, "bobsays-sqlite")
	host.send(t, "\t") // input -> buttons, still on [New Round]; back to [Aggregate]
	host.send(t, "h")
	host.send(t, "h")
	host.send(t, "\r")
	host.waitFor(t, "AGENTREPLY-Several")
	host.send(t, "\r") // the draft opens in the input; send it as it is
	host.waitFor(t, "AGENTREPLY-AGENTREPLY-Several")
}

// startNewRound has the host press [New Round] and waits for round 2.
func startNewRound(t *testing.T, host *term) {
	t.Helper()
	// Tab: transcript -> participants -> input -> buttons; [New Round] is the
	// third button.
	host.send(t, "\t\t\t")
	host.send(t, "l")
	host.send(t, "l")
	host.send(t, "\r")
	host.waitFor(t, "Participants (Round 2")
}

// #53: the host reconnecting after a new round started sees that round.
func TestReconnectedHostSeesCurrentRound(t *testing.T) {
	addr, _, _ := startServer(t)

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")
	bob := connect(t, addr, "bob")
	bob.joinAs(t, "Bobby")
	startNewRound(t, host)
	directAndAggregate(t, host, bob)

	host.disconnect()
	time.Sleep(300 * time.Millisecond) // let the server notice

	back := connect(t, addr, "host") // same SSH user: no name prompt this time
	back.waitFor(t, "Participants (Round 2")
	back.waitFor(t, "AGENTREPLY-hello")
	back.waitFor(t, "bobsays-sqlite")
	back.waitFor(t, "AGENTREPLY-AGENTREPLY-Several") // the newest message is on screen
	// Only the host screen has the buttons: the old session's end must have
	// freed the host role, or this session is a participant.
	back.waitFor(t, "New Round")
}

// #53: a participant with a stored display name reconnects (no name prompt, so
// the TUI does not register again) after a new round and sees that round.
func TestReconnectedParticipantSeesCurrentRound(t *testing.T) {
	addr, _, _ := startServer(t)

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")
	alice := connect(t, addr, "alice")
	alice.joinAs(t, "Alice")

	alice.disconnect()
	time.Sleep(300 * time.Millisecond) // let the server notice

	startNewRound(t, host)

	back := connect(t, addr, "alice") // same SSH user: no name prompt this time
	back.waitFor(t, "Participants (Round 2")
}

// #67: a participant who submitted, dropped, and reconnects after the host
// flagged them is told to revise and finds their previous answer waiting.
func TestReconnectedFlaggedParticipantGetsAnswerBack(t *testing.T) {
	addr, _, _ := startServer(t)

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")
	alice := connect(t, addr, "alice")
	alice.joinAs(t, "Alice")

	host.send(t, "\t\t")
	host.send(t, "hello\r")
	alice.waitFor(t, "AGENTREPLY-hello")
	alice.send(t, "\t\t")
	alice.send(t, "alice-first\r")
	host.waitFor(t, "alice-first")

	// The host flags Alice (host, Alice); she is told and her answer comes back.
	host.send(t, "\x1b[Z") // back from the input to the participants list
	host.send(t, "j")
	host.send(t, "c")
	host.waitFor(t, "revise")
	alice.waitFor(t, "asked you to revise")

	// She clears the box and drops while still flagged.
	for i := 0; i < len("alice-first"); i++ {
		alice.send(t, "\x7f")
	}
	alice.disconnect()

	back := connect(t, addr, "alice") // same SSH user: no name prompt this time
	back.waitFor(t, "asked you to revise")

	// Her previous answer is in the response box: what she types is appended
	// to it, and everyone sees the whole.
	back.send(t, "\t\t") // transcript -> sidebar -> response
	back.send(t, "-revised\r")
	host.waitFor(t, "alice-first-revised")
}

// servers maps a server's address to its process, so a test can signal it.
var servers sync.Map

// #30: Ctrl+C / SIGTERM tells connected users the session ended, saves the
// transcript to --data-dir, and exits on its own.
func TestGracefulShutdownSavesTranscript(t *testing.T) {
	addr, dataDir, _ := startServer(t)
	proc, _ := servers.Load(addr)
	cmd := proc.(*exec.Cmd)

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")
	alice := connect(t, addr, "alice")
	alice.joinAs(t, "Alice")
	host.send(t, "\t\t")
	host.send(t, "hello\r")
	alice.waitFor(t, "AGENTREPLY-hello")

	if err := interrupt(cmd); err != nil {
		t.Skipf("cannot signal the server on this machine: %v", err)
	}

	alice.waitFor(t, "Session ended")

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("server exited with an error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not exit after the interrupt")
	}

	raw, err := os.ReadFile(filepath.Join(dataDir, "transcript.json"))
	if err != nil {
		t.Fatalf("no transcript saved on shutdown: %v", err)
	}
	var doc struct {
		Messages []struct{ Type, Content string }
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("saved transcript is not valid JSON: %v", err)
	}
	found := false
	for _, m := range doc.Messages {
		if strings.Contains(m.Content, "AGENTREPLY-hello") {
			found = true
		}
	}
	if !found {
		t.Errorf("saved transcript lacks the session's messages: %s", raw)
	}
}
