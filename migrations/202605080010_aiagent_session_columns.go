package migrations

import (
	"encoding/json"
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202605080010 重整 conversation_messages：删冗余列（cago_id / parent_id /
// kind / origin / persist / tool_call_id / tool_calls / thinking / tool_call_json /
// tool_result_json / raw / content / msg_time），加 partial_reason + partial_detail，
// 建 (conversation_id, sort_order) 唯一索引；空 blocks 行的 content backfill 进 blocks。
// 同时给 conversations 加 thread_id / state_values。
//
// 单行多 Block 数据仍以 []ContentBlock JSON 存储，前端按回合聚合渲染。
//
// SQLite 的 ALTER TABLE 限制：不能直接 DROP COLUMN；走
// "建临时表 → copy → drop → rename" 的 CTAS 套路。
func migration202605080010() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605080010",
		Migrate: func(tx *gorm.DB) error {
			// 1. conversations 加列
			stmts := []string{
				`ALTER TABLE conversations ADD COLUMN thread_id VARCHAR(255) NOT NULL DEFAULT ''`,
				`ALTER TABLE conversations ADD COLUMN state_values TEXT`,
			}
			for _, s := range stmts {
				if err := tx.Exec(s).Error; err != nil {
					return err
				}
			}

			// 2. backfill：blocks 为空但 content 非空的行，把 content 折叠进 blocks。
			// JSON key 必须是 "text"（与前端 deserializeBlocks 的 TextBlock 协议一致）。
			rows, err := tx.Raw(`
				SELECT id, content
				FROM conversation_messages
				WHERE (blocks IS NULL OR blocks = '') AND content != ''
			`).Rows()
			if err != nil {
				return fmt.Errorf("backfill scan: %w", err)
			}
			defer func() { _ = rows.Close() }()
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

			// 3. CTAS：建目标表
			if err := tx.Exec(`
				CREATE TABLE __cm (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					conversation_id INTEGER NOT NULL,
					role TEXT NOT NULL,
					blocks TEXT,
					mentions TEXT,
					token_usage TEXT,
					partial_reason TEXT NOT NULL DEFAULT '',
					partial_detail TEXT NOT NULL DEFAULT '',
					sort_order INTEGER NOT NULL DEFAULT 0,
					createtime INTEGER
				)
			`).Error; err != nil {
				return fmt.Errorf("create table: %w", err)
			}

			// 4. copy
			if err := tx.Exec(`
				INSERT INTO __cm
				(id, conversation_id, role, blocks, mentions, token_usage, partial_reason, partial_detail, sort_order, createtime)
				SELECT id, conversation_id, role,
				       COALESCE(blocks, ''),
				       COALESCE(mentions, ''),
				       COALESCE(token_usage, ''),
				       '' as partial_reason,
				       '' as partial_detail,
				       COALESCE(sort_order, 0),
				       COALESCE(createtime, 0)
				FROM conversation_messages
			`).Error; err != nil {
				return fmt.Errorf("copy: %w", err)
			}

			// 5. swap
			if err := tx.Exec(`DROP TABLE conversation_messages`).Error; err != nil {
				return fmt.Errorf("drop: %w", err)
			}
			if err := tx.Exec(`ALTER TABLE __cm RENAME TO conversation_messages`).Error; err != nil {
				return fmt.Errorf("rename: %w", err)
			}

			// 6. 唯一索引
			if err := tx.Exec(`
				CREATE UNIQUE INDEX idx_conv_msg_unique
				ON conversation_messages(conversation_id, sort_order)
			`).Error; err != nil {
				return fmt.Errorf("create index: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP INDEX IF EXISTS idx_conv_msg_unique`).Error
		},
	}
}
