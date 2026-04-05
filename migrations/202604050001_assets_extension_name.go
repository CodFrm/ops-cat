package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration202604050001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604050001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE assets ADD COLUMN extension_name VARCHAR(64) NOT NULL DEFAULT ''`).Error; err != nil {
				return err
			}
			return tx.Exec(`CREATE INDEX IF NOT EXISTS idx_asset_extension_name ON assets(extension_name)`).Error
		},
	}
}
