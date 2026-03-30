package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration202603300003() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603300003",
		Migrate: func(tx *gorm.DB) error {
			stmts := []string{
				`CREATE TABLE extension_data_new (
					id             INTEGER PRIMARY KEY AUTOINCREMENT,
					extension_name VARCHAR(255) NOT NULL,
					key            VARCHAR(255) NOT NULL,
					value          BLOB,
					updatetime     INTEGER NOT NULL
				)`,
				`INSERT INTO extension_data_new (extension_name, key, value, updatetime)
					SELECT extension_name, key, value, updatetime FROM extension_data`,
				`DROP TABLE extension_data`,
				`ALTER TABLE extension_data_new RENAME TO extension_data`,
				`CREATE UNIQUE INDEX idx_ext_key ON extension_data (extension_name, key)`,
			}
			for _, s := range stmts {
				if err := tx.Exec(s).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
