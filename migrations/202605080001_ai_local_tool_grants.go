package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202605080001 建 ai_local_tool_grants：记录 cago built-in 本地工具
// (write / edit) 的会话级"始终放行"。bash 不写入此表（每次都要确认）。
func migration202605080001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605080001",
		Migrate: func(tx *gorm.DB) error {
			stmts := []string{
				`CREATE TABLE ai_local_tool_grants (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					session_id TEXT NOT NULL,
					tool_name TEXT NOT NULL,
					createtime INTEGER NOT NULL
				)`,
				`CREATE UNIQUE INDEX uq_local_tool_grant_session_tool
					ON ai_local_tool_grants(session_id, tool_name)`,
			}
			for _, stmt := range stmts {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS ai_local_tool_grants`).Error
		},
	}
}
