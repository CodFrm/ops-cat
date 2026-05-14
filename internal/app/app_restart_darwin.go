//go:build darwin

package app

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

func startRelaunchHelper(pid int, target relaunchTarget) error {
	launchPath := target.executablePath
	mode := "exec"
	if target.appBundlePath != "" {
		launchPath = target.appBundlePath
		mode = "app"
	}
	if launchPath == "" {
		return fmt.Errorf("restart target path is empty")
	}

	script := `while kill -0 "$1" 2>/dev/null; do sleep 0.1; done
sleep 0.3
if [ "$3" = "app" ]; then
  open -n "$2"
else
  nohup "$2" >/dev/null 2>&1 &
fi`
	// #nosec G204 -- script is a constant string literal; launchPath is the app's own executable/bundle path resolved internally, not user input
	cmd := exec.Command("/bin/sh", "-c", script, "opskat-restart", strconv.Itoa(pid), launchPath, mode)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
