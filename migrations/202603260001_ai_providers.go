package migrations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration202603260001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603260001",
		Migrate: func(tx *gorm.DB) error {
			// 1. 创建 ai_providers 表
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS ai_providers (
				id         INTEGER PRIMARY KEY AUTOINCREMENT,
				name       VARCHAR(100) NOT NULL,
				type       VARCHAR(50)  NOT NULL,
				api_base   VARCHAR(500) NOT NULL,
				api_key    TEXT,
				model      VARCHAR(100),
				is_active  INTEGER DEFAULT 0,
				createtime INTEGER,
				updatetime INTEGER
			)`).Error; err != nil {
				return err
			}

			// 2. Conversation 表新增 provider_id 字段
			if !tx.Migrator().HasColumn("conversations", "provider_id") {
				if err := tx.Exec("ALTER TABLE conversations ADD COLUMN provider_id INTEGER DEFAULT 0").Error; err != nil {
					return err
				}
			}

			// 3. 从 config.json 迁移现有配置
			migrateConfigToDB(tx)

			return nil
		},
	}
}

// migrateConfigToDB 从 config.json 读取旧 AI 配置并迁移到数据库
func migrateConfigToDB(tx *gorm.DB) {
	dataDir := appDataDir()
	configPath := filepath.Join(dataDir, "config.json")

	data, err := os.ReadFile(configPath) //nolint:gosec // configPath 来自固定平台路径，非用户输入
	if err != nil {
		return
	}

	var cfg struct {
		AIProviderType string `json:"ai_provider_type"`
		AIAPIBase      string `json:"ai_api_base"`
		AIAPIKey       string `json:"ai_api_key"`
		AIModel        string `json:"ai_model"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return
	}

	// 只迁移 openai 类型的配置（local_cli 不再支持）
	if cfg.AIProviderType != "openai" || cfg.AIAPIBase == "" {
		return
	}

	now := time.Now().Unix()
	if err := tx.Exec(
		"INSERT INTO ai_providers (name, type, api_base, api_key, model, is_active, createtime, updatetime) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"OpenAI Compatible", "openai", cfg.AIAPIBase, cfg.AIAPIKey, cfg.AIModel, 1, now, now,
	).Error; err != nil {
		// 最佳努力迁移，config.json 不会丢失
		fmt.Printf("migrate AI config to database: %v\n", err)
	}
}

// appDataDir 获取应用数据目录（migration 中不依赖 bootstrap 包避免循环引用）
func appDataDir() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "opskat")
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			home, _ := os.UserHomeDir()
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "opskat")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "opskat")
	}
}
