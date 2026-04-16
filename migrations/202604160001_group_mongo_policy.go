package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202604160001 为 groups 表添加 mongo_policy 字段
func migration202604160001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604160001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE groups ADD COLUMN mongo_policy TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			// SQLite 不支持 DROP COLUMN，需要重建表
			return nil
		},
	}
}
