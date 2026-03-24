package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202603240002 计划项添加资产组字段
func migration202603240002() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603240002",
		Migrate: func(tx *gorm.DB) error {
			columns := []struct {
				name string
				sql  string
			}{
				{"group_id", "ALTER TABLE plan_items ADD COLUMN group_id BIGINT DEFAULT 0"},
				{"group_name", "ALTER TABLE plan_items ADD COLUMN group_name VARCHAR(255) DEFAULT ''"},
			}
			for _, col := range columns {
				if !tx.Migrator().HasColumn("plan_items", col.name) {
					if err := tx.Exec(col.sql).Error; err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}
