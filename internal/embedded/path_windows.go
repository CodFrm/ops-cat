//go:build windows

package embedded

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

func addToUserPath(dir string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry key: %w", err)
	}
	defer func() { _ = k.Close() }()

	currentPath, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("read PATH: %w", err)
	}

	// 检查是否已在 PATH 中
	for _, p := range strings.Split(currentPath, ";") {
		if strings.EqualFold(strings.TrimSpace(p), dir) {
			return nil
		}
	}

	// 追加到 PATH
	newPath := currentPath
	if newPath != "" && !strings.HasSuffix(newPath, ";") {
		newPath += ";"
	}
	newPath += dir

	if err := k.SetStringValue("Path", newPath); err != nil {
		return fmt.Errorf("set PATH: %w", err)
	}

	// 广播 WM_SETTINGCHANGE 通知其他进程刷新环境变量
	broadcastSettingChange()
	return nil
}

func broadcastSettingChange() {
	env, _ := syscall.UTF16PtrFromString("Environment")
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("SendMessageTimeoutW")
	_, _, _ = proc.Call(
		uintptr(0xFFFF),              // HWND_BROADCAST
		uintptr(0x001A),              // WM_SETTINGCHANGE
		0,                            // wParam
		uintptr(unsafe.Pointer(env)), //nolint:gosec // required by SendMessageTimeoutW lParam
		uintptr(0x0002),              // SMTO_ABORTIFHUNG
		uintptr(5000),                // timeout ms
		0,                            // lpdwResult
	)
}
