//go:build !windows

package executil

import "os/exec"

// HideConsoleWindow is only needed on Windows.
func HideConsoleWindow(_ *exec.Cmd) {}
