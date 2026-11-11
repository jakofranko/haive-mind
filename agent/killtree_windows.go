//go:build windows

package agent

import (
	"os/exec"
	"strconv"
)

// killTreeOnCancel makes cancelling cmd end opencode and every process it
// started: npm's opencode.cmd runs opencode.exe, which a plain kill would miss.
func killTreeOnCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
}
