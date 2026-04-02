package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration202603300002() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603300002",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS extension_data (
				id             INTEGER PRIMARY KEY AUTOINCREMENT,
				extension_name VARCHAR(255) NOT NULL,
				key            VARCHAR(255) NOT NULL,
				value          BLOB,
				updatetime     INTEGER NOT NULL
			)`).Error; err != nil {
				return err
			}
			return tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ext_key ON extension_data (extension_name, key)`).Error
		},
	}
}
