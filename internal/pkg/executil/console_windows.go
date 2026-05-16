//go:build windows

package executil

import (
	"os/exec"
	"syscall"
)

const createNoWindowFlag = 0x08000000 // CREATE_NO_WINDOW

// HideConsoleWindow prevents console-subsystem child processes from flashing a console window.
// It intentionally does not set HideWindow, so GUI editors launched by external edit remain visible.
func HideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNoWindowFlag
}
