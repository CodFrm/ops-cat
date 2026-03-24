package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// AppConfig 应用持久化配置（config.json）
type AppConfig struct {
	UpdateChannel string `json:"update_channel,omitempty"` // stable, beta, nightly
	KDFSalt       string `json:"kdf_salt,omitempty"`       // base64 编码的 Argon2id salt
}

var (
	appConfig     *AppConfig
	appConfigOnce sync.Once
	configPath    string
)

// LoadConfig 加载应用配置，首次调用时自动生成默认值
// 必须在 Init 之后调用（依赖 dataDir）
func LoadConfig(dataDir string) (*AppConfig, error) {
	var loadErr error
	appConfigOnce.Do(func() {
		if dataDir == "" {
			dataDir = AppDataDir()
		}
		configPath = filepath.Join(dataDir, "config.json")

		data, err := os.ReadFile(configPath) //nolint:gosec // path from app data directory
		if err != nil {
			appConfig = &AppConfig{}
			loadErr = saveConfigFile()
			return
		}

		var cfg AppConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			appConfig = &AppConfig{}
			loadErr = saveConfigFile()
			return
		}

		appConfig = &cfg
	})
	return appConfig, loadErr
}

// GetConfig 获取当前配置（LoadConfig 之后调用）
func GetConfig() *AppConfig {
	return appConfig
}

// SaveConfig 保存配置到文件
func SaveConfig(cfg *AppConfig) error {
	appConfig = cfg
	return saveConfigFile()
}

func saveConfigFile() error {
	data, err := json.MarshalIndent(appConfig, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}
