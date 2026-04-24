package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// AppConfig 应用持久化配置（config.json）
type AppConfig struct {
	UpdateChannel   string `json:"update_channel,omitempty"`    // stable, beta, nightly
	DownloadMirror  string `json:"download_mirror,omitempty"`   // 下载镜像 URL 前缀，空表示直连 GitHub
	KDFSalt         string `json:"kdf_salt,omitempty"`          // base64 编码的 Argon2id salt
	AIProviderType  string `json:"ai_provider_type,omitempty"`  // openai, local_cli
	AIAPIBase       string `json:"ai_api_base,omitempty"`       // API base URL 或 CLI 路径
	AIAPIKey        string `json:"ai_api_key,omitempty"`        // 加密后的 API Key
	AIModel         string `json:"ai_model,omitempty"`          // 模型名或 CLI 类型
	GitHubToken     string `json:"github_token,omitempty"`      // 加密后的 GitHub token
	GitHubUser      string `json:"github_user,omitempty"`       // GitHub 用户名（非敏感）
	LastUpdateCheck int64  `json:"last_update_check,omitempty"` // 上次自动检查更新的 Unix 时间戳
	DebugMode       bool   `json:"debug_mode,omitempty"`        // 开启后日志级别降为 debug
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
	return os.WriteFile(configPath, data, 0600)
}
