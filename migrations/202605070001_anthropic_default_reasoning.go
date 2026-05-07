package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// 升级前 Anthropic provider 的 thinking 是硬编码常开；引入 reasoning_enabled/effort
// 字段后，老数据均为 0/''，会让 thinking 静默关闭。这里把存量 anthropic 行兜底为
// enabled+medium，保留原产品观感。
func migration202605070001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605070001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(
				`UPDATE ai_providers
                 SET reasoning_enabled = 1, reasoning_effort = 'medium'
                 WHERE type = 'anthropic'
                   AND reasoning_enabled = 0
                   AND (reasoning_effort IS NULL OR reasoning_effort = '')`,
			).Error
		},
	}
}
