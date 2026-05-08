package app

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultConversationWorkDirName = ".opskat"

func defaultConversationWorkDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户主目录失败: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("获取用户主目录失败: 目录为空")
	}

	dir := filepath.Join(home, defaultConversationWorkDirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("创建默认工作目录失败: %w", err)
	}
	return dir, nil
}
