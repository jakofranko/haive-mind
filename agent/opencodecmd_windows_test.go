package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"jakofranko/haive-mind/agent"
)

// stubOpencodeCmd puts opencode on PATH the way npm installs it on Windows: an
// opencode.cmd that passes its arguments to an opencode.exe (here, the stub).
func stubOpencodeCmd(t *testing.T, env map[string]string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "opencode.exe"), b, 0o755); err != nil {
		t.Fatal(err)
	}
	// The shape of npm's shim: hand every argument on with %*.
	shim := "@ECHO off\r\nSETLOCAL\r\n\"%~dp0\\bin\\opencode.exe\"   %*\r\n"
	if err := os.WriteFile(filepath.Join(dir, "opencode.cmd"), []byte(shim), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("STUB_OPENCODE", "1")
	for k, v := range env {
		t.Setenv(k, v)
	}
}

// The synthesis prompt runs over several lines: the instruction, the
// question, the responses. It must reach opencode whole through opencode.cmd.
func TestRun_PassesAMultiLinePromptWholeThroughOpencodeCmd(t *testing.T) {
	stdinFile := filepath.Join(t.TempDir(), "stdin")
	stubOpencodeCmd(t, map[string]string{"STUB_STDOUT": "ok", "STUB_STDIN_FILE": stdinFile})

	prompt := "Summarize the answers.\n\nQuestion: what is 2+2?\n\nResponses:\n- alice: 4\n- bob: \"four\" & 100% sure"
	if _, err := agent.NewDefaultRunner().Run(context.Background(), prompt); err != nil {
		t.Fatal(err)
	}
	if in, _ := os.ReadFile(stdinFile); string(in) != prompt {
		t.Errorf("opencode got the prompt %q,\nwant %q", in, prompt)
	}
}
