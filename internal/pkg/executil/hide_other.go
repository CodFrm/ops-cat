//go:build !windows

package executil

import "os/exec"

// HideWindow 非 Windows 平台无需处理
func HideWindow(_ *exec.Cmd) {}
