package migrations

import (
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupPreMigrationSchema 创建迁移前的表结构，含即将被删的列。
func setupPreMigrationSchema(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE conversations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT,
			provider_type TEXT NOT NULL DEFAULT '',
			status INTEGER DEFAULT 1,
			createtime INTEGER,
			updatetime INTEGER
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE conversation_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER NOT NULL,
			role TEXT NOT NULL,
			content TEXT,
			tool_calls TEXT,
			tool_call_id TEXT,
			blocks TEXT,
			mentions TEXT,
			token_usage TEXT,
			sort_order INTEGER DEFAULT 0,
			createtime INTEGER,
			cago_id TEXT,
			parent_id TEXT,
			kind TEXT,
			origin TEXT,
			thinking TEXT,
			tool_call_json TEXT,
			tool_result_json TEXT,
			persist INTEGER DEFAULT 1,
			raw TEXT,
			msg_time INTEGER
		)
	`).Error)
	return db
}

// Test202605080010_DropsColumnsAndAddsNewOnes 验证迁移一步完成：
// 删全部冗余列，加 partial_reason + partial_detail，建唯一索引。
func Test202605080010_DropsColumnsAndAddsNewOnes(t *testing.T) {
	db := setupPreMigrationSchema(t)

	require.NoError(t, db.Exec(`
		INSERT INTO conversation_messages
		(conversation_id, role, blocks, sort_order, createtime)
		VALUES (1, 'user', '[{"type":"text","content":"hello"}]', 0, 1000)
	`).Error)

	mig := migration202605080010()
	require.NoError(t, mig.Migrate(db))

	// 全量校验列都被删
	droppedCols := []string{
		"cago_id", "parent_id", "kind", "origin", "persist",
		"tool_call_id", "tool_calls", "thinking",
		"tool_call_json", "tool_result_json", "raw", "content", "msg_time",
	}
	for _, col := range droppedCols {
		var n int
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM pragma_table_info('conversation_messages') WHERE name=?`, col).Scan(&n).Error)
		assert.Equal(t, 0, n, "column %q should be dropped", col)
	}

	// 验证新列存在
	for _, col := range []string{"partial_reason", "partial_detail"} {
		var n int
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM pragma_table_info('conversation_messages') WHERE name=?`, col).Scan(&n).Error)
		assert.Equal(t, 1, n, "column %q should exist", col)
	}

	// 验证唯一索引
	var idxName string
	require.NoError(t, db.Raw(`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='conversation_messages' AND name=?`, "idx_conv_msg_unique").Scan(&idxName).Error)
	assert.Equal(t, "idx_conv_msg_unique", idxName)

	// 验证数据保留
	var blocks, role string
	var sortOrder int
	require.NoError(t, db.Raw(`SELECT blocks, role, sort_order FROM conversation_messages WHERE conversation_id=1`).Row().Scan(&blocks, &role, &sortOrder))
	assert.Contains(t, blocks, "hello")
	assert.Equal(t, "user", role)
	assert.Equal(t, 0, sortOrder)
}

// Test202605080010_BackfillsContentToBlocks 验证空 blocks 行的 content 被正确折叠。
// JSON key 必须是 "text"（与前端 deserializeBlocks 协议一致）。
func Test202605080010_BackfillsContentToBlocks(t *testing.T) {
	db := setupPreMigrationSchema(t)

	require.NoError(t, db.Exec(`
		INSERT INTO conversation_messages (conversation_id, role, content, blocks, sort_order)
		VALUES (1, 'assistant', 'fallback text', '', 0)
	`).Error)

	mig := migration202605080010()
	require.NoError(t, mig.Migrate(db))

	var blocksJSON string
	require.NoError(t, db.Raw(`SELECT blocks FROM conversation_messages WHERE conversation_id=1`).Row().Scan(&blocksJSON))

	var blocks []map[string]any
	require.NoError(t, json.Unmarshal([]byte(blocksJSON), &blocks))
	require.Len(t, blocks, 1, "should be one backfilled block")
	assert.Equal(t, "text", blocks[0]["type"])
	assert.Equal(t, "fallback text", blocks[0]["text"], "key MUST be 'text' (matches deserializeBlocks contract), not 'content'")
}

// Test202605080010_AddsConversationsColumns 验证 conversations 加了 thread_id / state_values。
func Test202605080010_AddsConversationsColumns(t *testing.T) {
	db := setupPreMigrationSchema(t)

	mig := migration202605080010()
	require.NoError(t, mig.Migrate(db))

	for _, col := range []string{"thread_id", "state_values"} {
		var n int
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM pragma_table_info('conversations') WHERE name=?`, col).Scan(&n).Error)
		assert.Equal(t, 1, n, "conversations.%s should exist", col)
	}
}

// Test202605080010_MultiBlockPreserved 验证含 tool Block 的行 blocks 数据完整保留。
func Test202605080010_MultiBlockPreserved(t *testing.T) {
	db := setupPreMigrationSchema(t)

	blocks, _ := json.Marshal([]map[string]any{
		{"type": "text", "content": "let me check"},
		{"type": "tool", "toolName": "bash", "content": "output"},
		{"type": "text", "content": "done"},
	})
	require.NoError(t, db.Exec(`
		INSERT INTO conversation_messages (conversation_id, role, blocks, sort_order)
		VALUES (1, 'assistant', ?, 0)
	`, string(blocks)).Error)

	mig := migration202605080010()
	require.NoError(t, mig.Migrate(db))

	var got string
	require.NoError(t, db.Raw(`SELECT blocks FROM conversation_messages WHERE conversation_id=1`).Row().Scan(&got))
	assert.JSONEq(t, string(blocks), got, "multi-block data must be preserved as-is")
}
