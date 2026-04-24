package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202604230001 创建 snippets 表
func migration202604230001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604230001",
		Migrate: func(tx *gorm.DB) error {
			stmts := []string{
				`CREATE TABLE snippets (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL,
					category TEXT NOT NULL,
					content TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					last_asset_ids TEXT NOT NULL DEFAULT '',
					source TEXT NOT NULL DEFAULT 'user',
					source_ref TEXT NOT NULL DEFAULT '',
					use_count INTEGER NOT NULL DEFAULT 0,
					last_used_at DATETIME,
					status INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME,
					updated_at DATETIME
				)`,
				`CREATE INDEX idx_snippets_category_status ON snippets(category, status)`,
				`CREATE INDEX idx_snippets_source ON snippets(source)`,
				`CREATE UNIQUE INDEX uq_snippets_source_ref ON snippets(source, source_ref) WHERE source_ref != ''`,
			}
			for _, stmt := range stmts {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS snippets`).Error
		},
	}
}
