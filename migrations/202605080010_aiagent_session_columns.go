package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202605080010 把 cago agent.Message + State 的字段平铺到表上，
// 为后续把 conversations.session_data 单源化到 conversation_messages 做准备。
//
// 加列：conversation_messages.{cago_id, parent_id, kind, origin, thinking,
// tool_call_json, tool_result_json, persist, raw, msg_time}；
// conversations.{thread_id, state_values}。
// 加索引：(conversation_id, cago_id) 用于 gormStore 行级 upsert 查找。
//
// 数据迁移在 202605080011；这里只动 schema。
func migration202605080010() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605080010",
		Migrate: func(tx *gorm.DB) error {
			stmts := []string{
				`ALTER TABLE conversation_messages ADD COLUMN cago_id VARCHAR(64) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversation_messages ADD COLUMN parent_id VARCHAR(64) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversation_messages ADD COLUMN kind VARCHAR(32) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversation_messages ADD COLUMN origin VARCHAR(32) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversation_messages ADD COLUMN thinking TEXT`,
				`ALTER TABLE conversation_messages ADD COLUMN tool_call_json TEXT`,
				`ALTER TABLE conversation_messages ADD COLUMN tool_result_json TEXT`,
				`ALTER TABLE conversation_messages ADD COLUMN persist BOOLEAN NOT NULL DEFAULT 1`,
				`ALTER TABLE conversation_messages ADD COLUMN raw TEXT`,
				`ALTER TABLE conversation_messages ADD COLUMN msg_time INTEGER NOT NULL DEFAULT 0`,
				`CREATE INDEX IF NOT EXISTS idx_conv_msg_cago_id ON conversation_messages(conversation_id, cago_id)`,
				`ALTER TABLE conversations ADD COLUMN thread_id VARCHAR(255) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversations ADD COLUMN state_values TEXT`,
			}
			for _, s := range stmts {
				if err := tx.Exec(s).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// SQLite 不支持 DROP COLUMN/INDEX 整批回滚；保留列在回滚时无害。
			return nil
		},
	}
}
