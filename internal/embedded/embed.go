package embedded

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// opsctlBinary 由 embed_prod.go (embed_opsctl tag) 或 embed_dev.go 设置
var opsctlBinary []byte

// HasEmbeddedOpsctl 检查是否嵌入了 opsctl 二进制
func HasEmbeddedOpsctl() bool {
	return len(opsctlBinary) > 0
}

// DefaultInstallDir 返回默认安装目录
func DefaultInstallDir() string {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, _ := os.UserHomeDir()
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "opskat", "bin")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

// InstallOpsctl 将嵌入的 opsctl 二进制写入指定目录
func InstallOpsctl(targetDir string) (string, error) {
	if len(opsctlBinary) == 0 {
		return "", fmt.Errorf("no embedded opsctl binary")
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("create directory failed: %w", err)
	}

	binName := "opsctl"
	if runtime.GOOS == "windows" {
		binName = "opsctl.exe"
	}
	targetPath := filepath.Join(targetDir, binName)

	if err := os.WriteFile(targetPath, opsctlBinary, 0755); err != nil {
		if runtime.GOOS == "windows" && os.IsPermission(err) {
			return "", fmt.Errorf("write binary failed (file may be in use, please close opsctl and retry): %w", err)
		}
		return "", fmt.Errorf("write binary failed: %w", err)
	}

	// Windows: 将安装目录添加到用户 PATH
	if err := addToUserPath(targetDir); err != nil {
		return targetPath, fmt.Errorf("installed successfully but failed to add to PATH: %w", err)
	}

	return targetPath, nil
}
