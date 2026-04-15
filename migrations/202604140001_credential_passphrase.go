package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202604140001 为 credentials 表添加 passphrase 字段
func migration202604140001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604140001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE credentials ADD COLUMN passphrase TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			// SQLite 不支持 DROP COLUMN，需要重建表
			return nil
		},
	}
}
