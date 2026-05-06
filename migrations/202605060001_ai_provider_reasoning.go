package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration202605060001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605060001",
		Migrate: func(tx *gorm.DB) error {
			if !tx.Migrator().HasColumn("ai_providers", "reasoning_enabled") {
				if err := tx.Exec("ALTER TABLE ai_providers ADD COLUMN reasoning_enabled INTEGER DEFAULT 0").Error; err != nil {
					return err
				}
			}

			if !tx.Migrator().HasColumn("ai_providers", "reasoning_effort") {
				if err := tx.Exec("ALTER TABLE ai_providers ADD COLUMN reasoning_effort VARCHAR(20) DEFAULT ''").Error; err != nil {
					return err
				}
			}

			return nil
		},
	}
}
