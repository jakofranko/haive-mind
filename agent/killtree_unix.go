//go:build !windows

package agent

import (
	"os/exec"
	"syscall"
)

// killTreeOnCancel makes cancelling cmd end opencode and every process it
// started, by running it in its own process group and killing the group.
func killTreeOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
