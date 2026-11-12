package e2e

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// runWithDefaults starts the real server in dir with none of the path flags,
// and the user directories pointed at xdg. The home directory variables point
// at a scratch directory so that a bug can never write into the real one. It
// returns once the server accepts connections.
func runWithDefaults(t *testing.T, dir, xdg string) (cmd *exec.Cmd, stdout *bytes.Buffer) {
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
		"XDG_DATA_HOME="+filepath.Join(xdg, "share"),
		"XDG_STATE_HOME="+filepath.Join(xdg, "state"),
		"HOME="+scratchHome, "USERPROFILE="+scratchHome, "LOCALAPPDATA="+scratchHome)
	prepareInterruptable(cmd)
	cmd.Stdout, cmd.Stderr = stdout, stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	for i := 0; i < 100; i++ {
		if c, err := net.Dial("tcp", "127.0.0.1:"+port); err == nil {
			c.Close()
			return cmd, stdout
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server never started listening; output:\n%s", stdout)
	return
}

// stop asks the server to shut down and waits for it to finish.
func stop(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if err := interrupt(cmd); err != nil {
		t.Skipf("cannot signal the server on this machine: %v", err)
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

// With no path flags the host key, the transcript and the log file go to the
// user's data and state directories, nothing is left in the working directory,
// and the same host key is used wherever the server is started from.
func TestDefaultLocations_FollowTheUserDirectoriesNotTheWorkingDirectory(t *testing.T) {
	xdg := t.TempDir()
	dataDir := filepath.Join(xdg, "share", "haive-mind")
	logFile := filepath.Join(xdg, "state", "haive-mind", "haive-mind.log")
	keyFile := filepath.Join(dataDir, "id_ed25519")

	firstDir := t.TempDir()
	cmd, stdout := runWithDefaults(t, firstDir, xdg)
	stop(t, cmd)

	key, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("no host key in the data directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "transcript.json")); err != nil {
		t.Errorf("no transcript saved without --data-dir: %v", err)
	}
	if b, err := os.ReadFile(logFile); err != nil || len(b) == 0 {
		t.Errorf("no log written to the default log file: %v", err)
	}
	if got := filesIn(t, firstDir); len(got) != 0 {
		t.Errorf("the working directory should stay empty, found %v", got)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout is not a terminal, so logs should only go to the file, got:\n%s", stdout)
	}

	secondDir := t.TempDir()
	cmd, _ = runWithDefaults(t, secondDir, xdg)
	stop(t, cmd)
	again, err := os.ReadFile(keyFile)
	if err != nil || !bytes.Equal(key, again) {
		t.Errorf("a server started from another directory should reuse the host key (err %v)", err)
	}
	if got := filesIn(t, secondDir); len(got) != 0 {
		t.Errorf("the working directory should stay empty, found %v", got)
	}
}
