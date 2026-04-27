//go:build windows

package executil

import (
	"os/exec"
	"syscall"
)

// HideWindow 隐藏子进程窗口：console 子系统加 CREATE_NO_WINDOW 防止黑窗一闪而过，
// GUI 程序通过 SW_HIDE 隐藏主窗口。注意：对需要正常显示的 GUI 程序（如 explorer.exe）
// 不要调用本方法，否则其窗口会被一并隐藏。
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
