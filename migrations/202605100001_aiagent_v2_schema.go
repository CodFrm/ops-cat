package migrations

import (
	"encoding/json"
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202605100001 把 conversation_messages 从 cago v1 平铺形态升到
// v2：删 v1 字段（cago_id / parent_id / kind / origin / persist /
// tool_call_id / tool_calls / thinking / tool_call_json / tool_result_json /
// raw / content / msg_time），加 partial_reason，建 (conversation_id,
// sort_order) 唯一索引；空 blocks 行的 content 字段 backfill 进 blocks。
//
// SQLite 的 ALTER TABLE 限制：不能直接 DROP COLUMN（取决于版本）；这里走
// "建新表 → copy 数据 → drop 旧表 → rename" 的 CTAS 套路。
func migration202605100001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605100001_aiagent_v2_schema",
		Migrate: func(tx *gorm.DB) error {
			// 1. backfill：blocks 为空时把 content 折叠进去。
			// JSON key 必须是 "text"（与 Task 9 deserializeBlocks 的 TextBlock 协议
			// 一致）；早先用 "content" 会导致每行回放成空 TextBlock。
			rows, err := tx.Raw(`
				SELECT id, content
				FROM conversation_messages
				WHERE (blocks IS NULL OR blocks = '') AND content != ''
			`).Rows()
			if err != nil {
				return fmt.Errorf("backfill scan: %w", err)
			}
			defer rows.Close()
			type pair struct {
				id      int64
				content string
			}
			var todo []pair
			for rows.Next() {
				var p pair
				if err := rows.Scan(&p.id, &p.content); err != nil {
					return fmt.Errorf("backfill row: %w", err)
				}
				todo = append(todo, p)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("backfill iter: %w", err)
			}
			for _, p := range todo {
				blob, err := json.Marshal([]map[string]string{
					{"type": "text", "text": p.content},
				})
				if err != nil {
					return fmt.Errorf("backfill marshal id=%d: %w", p.id, err)
				}
				if err := tx.Exec(`UPDATE conversation_messages SET blocks=? WHERE id=?`, string(blob), p.id).Error; err != nil {
					return fmt.Errorf("backfill update id=%d: %w", p.id, err)
				}
			}

			// 2. CTAS：新表 conversation_messages_v2
			if err := tx.Exec(`
				CREATE TABLE conversation_messages_v2 (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					conversation_id INTEGER NOT NULL,
					role TEXT NOT NULL,
					blocks TEXT,
					mentions TEXT,
					token_usage TEXT,
					partial_reason TEXT NOT NULL DEFAULT '',
					sort_order INTEGER NOT NULL DEFAULT 0,
					createtime INTEGER
				)
			`).Error; err != nil {
				return fmt.Errorf("create v2 table: %w", err)
			}

			// 3. copy 数据
			if err := tx.Exec(`
				INSERT INTO conversation_messages_v2
				(id, conversation_id, role, blocks, mentions, token_usage, partial_reason, sort_order, createtime)
				SELECT id, conversation_id, role,
				       COALESCE(blocks, ''),
				       COALESCE(mentions, ''),
				       COALESCE(token_usage, ''),
				       '' as partial_reason,
				       COALESCE(sort_order, 0),
				       COALESCE(createtime, 0)
				FROM conversation_messages
			`).Error; err != nil {
				return fmt.Errorf("copy: %w", err)
			}

			// 4. drop 旧表 + rename
			if err := tx.Exec(`DROP TABLE conversation_messages`).Error; err != nil {
				return fmt.Errorf("drop old: %w", err)
			}
			if err := tx.Exec(`ALTER TABLE conversation_messages_v2 RENAME TO conversation_messages`).Error; err != nil {
				return fmt.Errorf("rename: %w", err)
			}

			// 5. 唯一索引
			if err := tx.Exec(`
				CREATE UNIQUE INDEX idx_conv_msg_unique
				ON conversation_messages(conversation_id, sort_order)
			`).Error; err != nil {
				return fmt.Errorf("create index: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 简单回滚：删唯一索引；列不还原（迁移本身有损）
			return tx.Exec(`DROP INDEX IF EXISTS idx_conv_msg_unique`).Error
		},
	}
}
