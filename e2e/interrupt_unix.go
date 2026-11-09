//go:build !windows

package e2e

import (
	"os/exec"
	"syscall"
)

// prepareInterruptable readies cmd so interrupt can reach it.
func prepareInterruptable(cmd *exec.Cmd) {}

// interrupt asks the server to shut down gracefully, as Ctrl+C or a service
// manager would.
func interrupt(cmd *exec.Cmd) error { return cmd.Process.Signal(syscall.SIGTERM) }
