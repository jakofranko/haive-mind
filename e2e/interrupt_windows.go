//go:build windows

package e2e

import (
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	ctrlBreakEvent        = 1
)

// prepareInterruptable puts the server in its own process group, which is what
// lets a Ctrl+Break be aimed at it alone.
func prepareInterruptable(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}

// interrupt sends Ctrl+Break, which Go delivers to the server as os.Interrupt.
func interrupt(cmd *exec.Cmd) error {
	dll, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return err
	}
	proc, err := dll.FindProc("GenerateConsoleCtrlEvent")
	if err != nil {
		return err
	}
	if r, _, err := proc.Call(ctrlBreakEvent, uintptr(cmd.Process.Pid)); r == 0 {
		return err
	}
	return nil
}
