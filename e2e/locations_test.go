package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// runWithDefaults starts the real server in dir with none of the path flags,
// and the user directories pointed at xdg. The home directory variables point
// at a scratch directory so that a bug can never write into the real one. It
// returns once the server accepts connections.
func runWithDefaults(t *testing.T, dir, xdg string) (cmd *exec.Cmd, stdout *bytes.Buffer, addr string) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the real binary; skipped with -short")
	}
	bin := binaries(t)
	port := freePort(t)
	scratchHome := t.TempDir()

	stdout = &bytes.Buffer{}
	cmd = exec.Command(filepath.Join(bin, exe("haive-mind")), "--bind", "127.0.0.1", "--port", port)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_AGENT_LOG="+filepath.Join(t.TempDir(), "agent.log"),
		"XDG_DATA_HOME="+filepath.Join(xdg, "share"),
		"XDG_STATE_HOME="+filepath.Join(xdg, "state"),
		"HOME="+scratchHome, "USERPROFILE="+scratchHome, "LOCALAPPDATA="+scratchHome)
	prepareInterruptable(cmd)
	cmd.Stdout, cmd.Stderr = stdout, stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	addr = "127.0.0.1:" + port
	waitListening(t, addr, stdout)
	return cmd, stdout, addr
}

// stop asks the server to shut down and waits for it to finish.
func stop(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if err := interrupt(cmd); err != nil {
		t.Fatalf("cannot signal the server: %v", err)
	}
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
}

func filesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// defaultRun is one server run with no path flags, stopped again, in a fresh
// working directory.
type defaultRun struct {
	workDir string
	stdout  *bytes.Buffer
}

func runAndStopWithDefaults(t *testing.T, xdg string) defaultRun {
	t.Helper()
	dir := t.TempDir()
	cmd, stdout, _ := runWithDefaults(t, dir, xdg)
	stop(t, cmd)
	return defaultRun{workDir: dir, stdout: stdout}
}

// With no path flags the host key, the transcript and the log file go to the
// user's data and state directories, nothing is left in the working directory,
// and the same host key is used wherever the server is started from.
func TestDefaultLocations_HostKeyIsPrivateAndInTheDataDirectory(t *testing.T) {
	xdg := t.TempDir()
	dataDir := filepath.Join(xdg, "share", "haive-mind")
	keyFile := filepath.Join(dataDir, "id_ed25519")

	runAndStopWithDefaults(t, xdg)

	if _, err := os.ReadFile(keyFile); err != nil {
		t.Fatalf("no host key in the data directory: %v", err)
	}
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{dataDir: 0o700, keyFile: 0o600} {
			if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != want {
				t.Errorf("%s: mode %v (err %v), want %o", path, fi, err, want)
			}
		}
	}
}

func TestDefaultLocations_TranscriptGoesToTheDataDirectory(t *testing.T) {
	xdg := t.TempDir()

	runAndStopWithDefaults(t, xdg)

	if _, err := os.Stat(filepath.Join(xdg, "share", "haive-mind", "transcript.json")); err != nil {
		t.Errorf("no transcript saved without --data-dir: %v", err)
	}
}

func TestDefaultLocations_LogGoesToTheStateDirectory(t *testing.T) {
	xdg := t.TempDir()

	runAndStopWithDefaults(t, xdg)

	logFile := filepath.Join(xdg, "state", "haive-mind", "haive-mind.log")
	if b, err := os.ReadFile(logFile); err != nil || len(b) == 0 {
		t.Errorf("no log written to the default log file: %v", err)
	}
}

func TestDefaultLocations_LeaveTheWorkingDirectoryEmpty(t *testing.T) {
	run := runAndStopWithDefaults(t, t.TempDir())

	if got := filesIn(t, run.workDir); len(got) != 0 {
		t.Errorf("the working directory should stay empty, found %v", got)
	}
}

func TestDefaultLocations_StdoutStaysQuietWhenNotATerminal(t *testing.T) {
	run := runAndStopWithDefaults(t, t.TempDir())

	if run.stdout.Len() != 0 {
		t.Errorf("stdout is not a terminal, so logs should only go to the file, got:\n%s", run.stdout)
	}
}

func TestDefaultLocations_AnotherWorkingDirectoryReusesTheHostKey(t *testing.T) {
	xdg := t.TempDir()
	keyFile := filepath.Join(xdg, "share", "haive-mind", "id_ed25519")

	runAndStopWithDefaults(t, xdg)
	key, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("no host key in the data directory: %v", err)
	}

	second := runAndStopWithDefaults(t, xdg)

	again, err := os.ReadFile(keyFile)
	if err != nil || !bytes.Equal(key, again) {
		t.Errorf("a server started from another directory should reuse the host key (err %v)", err)
	}
	if got := filesIn(t, second.workDir); len(got) != 0 {
		t.Errorf("the working directory should stay empty, found %v", got)
	}
}

// The host's export lands in the data directory without --data-dir.
func TestDefaultLocations_ExportGoesToTheDataDirectory(t *testing.T) {
	xdg := t.TempDir()
	cmd, _, addr := runWithDefaults(t, t.TempDir(), xdg)
	t.Cleanup(func() { cmd.Process.Kill() })

	host := connect(t, addr, "host")
	host.joinAs(t, "Hosty")
	host.send(t, "\t\t") // the directive input
	host.send(t, "\t")   // the buttons
	for i := 0; i < 3; i++ {
		host.send(t, "l") // one key event per press
	}
	host.send(t, "\r") // Export
	host.waitFor(t, "Exported to")

	files, _ := filepath.Glob(filepath.Join(xdg, "share", "haive-mind", "transcript_*.json"))
	if len(files) != 1 {
		t.Errorf("want one exported transcript in the default data directory, got %v", files)
	}
}
