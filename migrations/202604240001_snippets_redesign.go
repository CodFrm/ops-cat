package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202604240001 drops the tags / asset_id columns from snippets and
// adds last_asset_ids (comma-separated int64 list; read-side filters stale IDs).
func migration202604240001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604240001",
		Migrate: func(tx *gorm.DB) error {
			stmts := []string{
				`DROP INDEX IF EXISTS idx_snippets_asset_id`,
				`ALTER TABLE snippets DROP COLUMN asset_id`,
				`ALTER TABLE snippets DROP COLUMN tags`,
				`ALTER TABLE snippets ADD COLUMN last_asset_ids TEXT NOT NULL DEFAULT ''`,
			}
			for _, stmt := range stmts {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			stmts := []string{
				`ALTER TABLE snippets DROP COLUMN last_asset_ids`,
				`ALTER TABLE snippets ADD COLUMN tags TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE snippets ADD COLUMN asset_id INTEGER`,
				`CREATE INDEX idx_snippets_asset_id ON snippets(asset_id)`,
			}
			for _, stmt := range stmts {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
