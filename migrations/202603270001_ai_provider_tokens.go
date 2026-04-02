package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration202603270001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603270001",
		Migrate: func(tx *gorm.DB) error {
			if !tx.Migrator().HasColumn("ai_providers", "max_output_tokens") {
				if err := tx.Exec("ALTER TABLE ai_providers ADD COLUMN max_output_tokens INTEGER DEFAULT 0").Error; err != nil {
					return err
				}
			}
			if !tx.Migrator().HasColumn("ai_providers", "context_window") {
				if err := tx.Exec("ALTER TABLE ai_providers ADD COLUMN context_window INTEGER DEFAULT 0").Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
