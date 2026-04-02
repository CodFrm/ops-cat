package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration202603310001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603310001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS extension_state (
				id         INTEGER PRIMARY KEY AUTOINCREMENT,
				name       VARCHAR(255) NOT NULL,
				enabled    INTEGER NOT NULL DEFAULT 1,
				createtime INTEGER NOT NULL,
				updatetime INTEGER NOT NULL
			)`).Error; err != nil {
				return err
			}
			return tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ext_state_name ON extension_state (name)`).Error
		},
	}
}
