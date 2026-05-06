package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202604270001 为 groups 表添加 k8s_policy 字段
func migration202604270001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604270001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE groups ADD COLUMN k8s_policy TEXT
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
