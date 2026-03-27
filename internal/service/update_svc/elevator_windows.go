//go:build windows

package update_svc

import (
	"fmt"
	"os/exec"

	"github.com/opskat/opskat/internal/pkg/executil"
)

// runInstaller 运行 NSIS 安装程序（用户级安装，无需 UAC 提权）
func runInstaller(exePath string, args ...string) error {
	cmd := exec.Command(exePath, args...)
	executil.HideWindow(cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("run installer failed: %s: %w", string(output), err)
	}
	return nil
}
