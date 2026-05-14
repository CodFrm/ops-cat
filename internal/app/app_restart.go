package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type relaunchTarget struct {
	executablePath string
	appBundlePath  string
}

// RestartApp relaunches OpsKat after the current process exits.
func (a *App) RestartApp() error {
	if a.ctx == nil {
		return fmt.Errorf("app context is not ready")
	}

	target, err := currentRelaunchTarget()
	if err != nil {
		return err
	}
	if err := startRelaunchHelper(os.Getpid(), target); err != nil {
		return err
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		wailsRuntime.Quit(a.ctx)
	}()
	return nil
}

func currentRelaunchTarget() (relaunchTarget, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return relaunchTarget{}, fmt.Errorf("get executable path failed: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(executablePath); err == nil {
		executablePath = resolved
	}
	return resolveRelaunchTargetFromExecutablePath(runtime.GOOS, executablePath)
}

func resolveRelaunchTargetFromExecutablePath(goos, executablePath string) (relaunchTarget, error) {
	if strings.TrimSpace(executablePath) == "" {
		return relaunchTarget{}, fmt.Errorf("executable path is empty")
	}

	target := relaunchTarget{
		executablePath: normalizeExecutablePathForRelaunch(goos, executablePath),
	}
	if goos == "darwin" {
		if appBundlePath, ok := findMacAppBundlePath(executablePath); ok {
			target.appBundlePath = appBundlePath
		}
	}
	return target, nil
}

func normalizeExecutablePathForRelaunch(goos, executablePath string) string {
	path := strings.TrimSuffix(executablePath, " (deleted)")
	switch goos {
	case "windows":
		if strings.HasSuffix(strings.ToLower(path), ".exe.old") {
			return path[:len(path)-len(".old")]
		}
	case "darwin", "linux":
		if strings.HasSuffix(path, ".backup") {
			return strings.TrimSuffix(path, ".backup")
		}
	}
	return path
}

func findMacAppBundlePath(executablePath string) (string, bool) {
	path := strings.TrimSuffix(executablePath, " (deleted)")
	for current := filepath.Clean(path); current != "." && current != string(filepath.Separator); current = filepath.Dir(current) {
		base := filepath.Base(current)
		if strings.HasSuffix(base, ".app") {
			return current, true
		}
		if strings.HasSuffix(base, ".app.backup") {
			return filepath.Join(filepath.Dir(current), strings.TrimSuffix(base, ".backup")), true
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return "", false
}
