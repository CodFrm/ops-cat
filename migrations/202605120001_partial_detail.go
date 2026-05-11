package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202605120001 给 conversation_messages 加 partial_detail 列：用来配合
// partial_reason，承载错误/取消/超时的可读详情（PartialReason 是状态枚举，
// PartialDetail 是该状态对应的人类可读文本）。
//
// 老数据全部 backfill 为空串：历史行此前流式出错时没有保留具体错误信息（仅在前端
// 内存中拼了一段 "**Error:**"），没法回填，留空。新写入的 errored 行会带详情。
func migration202605120001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605120001_partial_detail",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE conversation_messages
				ADD COLUMN partial_detail TEXT NOT NULL DEFAULT ''
			`).Error; err != nil {
				return fmt.Errorf("add partial_detail column: %w", err)
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// SQLite 不支持直接 DROP COLUMN；与 202605100001 一致采用 CTAS。
			if err := tx.Exec(`
				CREATE TABLE conversation_messages_pre_partial_detail (
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
				return err
			}
			if err := tx.Exec(`
				INSERT INTO conversation_messages_pre_partial_detail
				(id, conversation_id, role, blocks, mentions, token_usage, partial_reason, sort_order, createtime)
				SELECT id, conversation_id, role, blocks, mentions, token_usage, partial_reason, sort_order, createtime
				FROM conversation_messages
			`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE conversation_messages`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE conversation_messages_pre_partial_detail RENAME TO conversation_messages`).Error; err != nil {
				return err
			}
			return tx.Exec(`
				CREATE UNIQUE INDEX idx_conv_msg_unique
				ON conversation_messages(conversation_id, sort_order)
			`).Error
		},
	}
}
