package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202603240001 审计日志添加决策信息字段
func migration202603240001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603240001",
		Migrate: func(tx *gorm.DB) error {
			type AuditLog struct{}
			columns := []struct {
				name string
				sql  string
			}{
				{"session_id", "ALTER TABLE audit_logs ADD COLUMN session_id VARCHAR(64) DEFAULT ''"},
				{"decision", "ALTER TABLE audit_logs ADD COLUMN decision VARCHAR(10) DEFAULT ''"},
				{"decision_source", "ALTER TABLE audit_logs ADD COLUMN decision_source VARCHAR(30) DEFAULT ''"},
				{"matched_pattern", "ALTER TABLE audit_logs ADD COLUMN matched_pattern VARCHAR(500) DEFAULT ''"},
			}
			for _, col := range columns {
				if !tx.Migrator().HasColumn("audit_logs", col.name) {
					if err := tx.Exec(col.sql).Error; err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}
