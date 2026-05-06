package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202605010001 为 groups 表添加 kafka_policy 字段
func migration202605010001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605010001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE groups ADD COLUMN kafka_policy TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			// SQLite 不支持 DROP COLUMN，需要重建表
			return nil
		},
	}
}
