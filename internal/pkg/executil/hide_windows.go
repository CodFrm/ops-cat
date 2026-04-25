//go:build windows

package executil

import (
	"os/exec"
	"syscall"
)

// HideWindow 给 console 子系统的子进程加 CREATE_NO_WINDOW，避免黑窗一闪而过；
// 对 GUI 程序（如 explorer.exe）无副作用。注意：不要加 SysProcAttr.HideWindow，
// 那会通过 SW_HIDE 把 GUI 程序的主窗口也藏起来。
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
