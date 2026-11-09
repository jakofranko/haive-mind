package agent_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"jakofranko/haive-mind/agent"
)

// The test binary doubles as a stand-in `opencode`: when STUB_OPENCODE is set
// it behaves as the stub and exits instead of running tests.
func TestMain(m *testing.M) {
	if os.Getenv("STUB_OPENCODE") == "1" {
		runStub()
		return
	}
	os.Exit(m.Run())
}

func runStub() {
	if p := os.Getenv("STUB_ARGS_FILE"); p != "" {
		os.WriteFile(p, []byte(strings.Join(os.Args[1:], "\x00")), 0o644)
	}
	if ms := os.Getenv("STUB_SLEEP_MS"); ms != "" {
		d, _ := time.ParseDuration(ms + "ms")
		time.Sleep(d)
	}
	os.Stdout.WriteString(os.Getenv("STUB_STDOUT"))
	os.Stderr.WriteString(os.Getenv("STUB_STDERR"))
	if os.Getenv("STUB_FAIL") == "1" {
		os.Exit(3)
	}
}

// stubOpencode puts an `opencode` on PATH that behaves as env describes.
func stubOpencode(t *testing.T, env map[string]string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "opencode"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dir := t.TempDir()
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("STUB_OPENCODE", "1")
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestRun_ReturnsWhatTheAgentPrintsAndPassesThePrompt(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	stubOpencode(t, map[string]string{"STUB_STDOUT": "the answer", "STUB_ARGS_FILE": argsFile})

	got, err := agent.NewDefaultRunner().Run(context.Background(), "what is 2+2?")
	if err != nil {
		t.Fatal(err)
	}
	if got != "the answer" {
		t.Errorf("reply = %q, want %q", got, "the answer")
	}
	args, _ := os.ReadFile(argsFile)
	if string(args) != "run\x00what is 2+2?" {
		t.Errorf("opencode args = %q, want run <prompt>", args)
	}
}

// G10: diagnostics on stderr are not part of the answer.
func TestRun_StderrIsNotPartOfTheReply(t *testing.T) {
	stubOpencode(t, map[string]string{"STUB_STDOUT": "the answer", "STUB_STDERR": "warning: noisy"})

	got, err := agent.NewDefaultRunner().Run(context.Background(), "q")
	if err != nil {
		t.Fatal(err)
	}
	if got != "the answer" {
		t.Errorf("reply = %q, want only stdout", got)
	}
}

// A failing agent reports why, using what it wrote to stderr.
func TestRun_FailureErrorCarriesStderr(t *testing.T) {
	stubOpencode(t, map[string]string{"STUB_FAIL": "1", "STUB_STDERR": "model not found"})

	_, err := agent.NewDefaultRunner().Run(context.Background(), "q")
	if err == nil {
		t.Fatal("want an error from a non-zero exit")
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Errorf("error %q does not say why the agent failed", err)
	}
}

// G15: the timeout is configurable, and exceeding it is ErrAgentTimeout.
func TestRun_TimesOutAfterTheConfiguredDuration(t *testing.T) {
	stubOpencode(t, map[string]string{"STUB_SLEEP_MS": "5000"})

	start := time.Now()
	_, err := agent.NewRunner(200 * time.Millisecond).Run(context.Background(), "q")
	if !errors.Is(err, agent.ErrAgentTimeout) {
		t.Fatalf("err = %v, want ErrAgentTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("took %v to give up, want about 200ms", elapsed)
	}
}

func TestRun_StopsWhenTheCallerCancels(t *testing.T) {
	stubOpencode(t, map[string]string{"STUB_SLEEP_MS": "5000"})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)

	start := time.Now()
	_, err := agent.NewDefaultRunner().Run(ctx, "q")
	if err == nil {
		t.Fatal("want an error after cancel")
	}
	if errors.Is(err, agent.ErrAgentTimeout) {
		t.Errorf("a cancel is not a timeout: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("did not stop promptly")
	}
}
