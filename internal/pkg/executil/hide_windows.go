//go:build windows

package executil

import (
	"os/exec"
	"syscall"
)

// HideWindow 设置子进程不创建控制台窗口，防止黑窗一闪而过
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
