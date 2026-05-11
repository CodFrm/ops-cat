package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202605080010
//   - conversations 加 thread_id / state_values（cago Session 平铺）
//   - conversation_messages 删 content / tool_calls / tool_call_id，加
//     partial_reason / partial_detail，建 (conversation_id, sort_order) 唯一索引；
//     DROP 之前先把空 blocks 行的 content 折成 [{type:"text", text:...}]
//
// SQLite 3.35+ 原生支持 ALTER TABLE DROP COLUMN，不需要 CTAS 临时表。
func migration202605080010() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605080010",
		Migrate: func(tx *gorm.DB) error {
			stmts := []string{
				`ALTER TABLE conversations ADD COLUMN thread_id VARCHAR(255) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversations ADD COLUMN state_values TEXT`,

				// backfill：空 blocks 但 content 非空的行，把 content 折成
				// [{"type":"text","text":...}]，与前端 deserializeBlocks 的 TextBlock 协议一致。
				// 必须在 DROP COLUMN content 之前跑。
				`UPDATE conversation_messages
				 SET blocks = json_array(json_object('type', 'text', 'text', content))
				 WHERE (blocks IS NULL OR blocks = '') AND COALESCE(content, '') != ''`,

				`ALTER TABLE conversation_messages DROP COLUMN content`,
				`ALTER TABLE conversation_messages DROP COLUMN tool_calls`,
				`ALTER TABLE conversation_messages DROP COLUMN tool_call_id`,

				`ALTER TABLE conversation_messages ADD COLUMN partial_reason TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE conversation_messages ADD COLUMN partial_detail TEXT NOT NULL DEFAULT ''`,

				`CREATE UNIQUE INDEX idx_conv_msg_unique
				 ON conversation_messages(conversation_id, sort_order)`,
			}
			for _, s := range stmts {
				if err := tx.Exec(s).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP INDEX IF EXISTS idx_conv_msg_unique`).Error
		},
	}
}
