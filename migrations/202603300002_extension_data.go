package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration202603300002() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603300002",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`CREATE TABLE extension_data (
				extension_name VARCHAR(255) NOT NULL,
				key            VARCHAR(255) NOT NULL,
				value          BLOB,
				updatetime     INTEGER NOT NULL,
				PRIMARY KEY (extension_name, key)
			)`).Error
		},
	}
}
