//go:build !windows && !darwin

package app

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

func startRelaunchHelper(pid int, target relaunchTarget) error {
	launchPath := target.executablePath
	if launchPath == "" {
		return fmt.Errorf("restart target executable is empty")
	}

	script := `while kill -0 "$1" 2>/dev/null; do sleep 0.1; done
sleep 0.3
nohup "$2" >/dev/null 2>&1 &`
	// #nosec G204 -- script is a constant string literal; launchPath is the app's own executable path resolved internally, not user input
	cmd := exec.Command("/bin/sh", "-c", script, "opskat-restart", strconv.Itoa(pid), launchPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
