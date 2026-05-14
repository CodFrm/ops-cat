//go:build windows

package app

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/opskat/opskat/internal/pkg/executil"
)

func startRelaunchHelper(pid int, target relaunchTarget) error {
	if target.executablePath == "" {
		return fmt.Errorf("restart target executable is empty")
	}

	script := "try { Wait-Process -Id " + strconv.Itoa(pid) + " -ErrorAction SilentlyContinue } catch {}; " +
		"Start-Sleep -Milliseconds 300; " +
		"Start-Process -FilePath " + quotePowerShellSingle(target.executablePath)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", script)
	executil.HideWindow(cmd)
	return cmd.Start()
}

func quotePowerShellSingle(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
