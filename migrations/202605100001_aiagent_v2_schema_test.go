package migrations

import (
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func Test202605100001_DropsLegacyColumnsAndAddsPartialReason(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// 模拟一份 v1 schema：conversation_messages 含老列
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

	require.NoError(t, db.Exec(`
		INSERT INTO conversation_messages
		(conversation_id, role, content, blocks, sort_order, cago_id, kind, origin)
		VALUES (1, 'user', 'hello', '[{"type":"text","text":"hello"}]', 0, 'old-cago-id-1', 'message', 'user')
	`).Error)

	mig := migration202605100001()
	require.NoError(t, mig.Migrate(db))

	// 全量校验 13 个 v1 列都被删
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

	var count int
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM pragma_table_info('conversation_messages') WHERE name=?`, "partial_reason").Scan(&count).Error)
	assert.Equal(t, 1, count, "partial_reason should exist")

	var idxName string
	require.NoError(t, db.Raw(`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='conversation_messages' AND name=?`, "idx_conv_msg_unique").Scan(&idxName).Error)
	assert.Equal(t, "idx_conv_msg_unique", idxName)

	var blocks, role string
	var sortOrder int
	require.NoError(t, db.Raw(`SELECT blocks, role, sort_order FROM conversation_messages WHERE conversation_id=1`).Row().Scan(&blocks, &role, &sortOrder))
	assert.Contains(t, blocks, "hello")
	assert.Equal(t, "user", role)
	assert.Equal(t, 0, sortOrder)
}

func Test202605100001_BackfillsContentToBlocks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

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

	require.NoError(t, db.Exec(`
		INSERT INTO conversation_messages (conversation_id, role, content, blocks, sort_order)
		VALUES (1, 'assistant', 'fallback text', '', 0)
	`).Error)

	mig := migration202605100001()
	require.NoError(t, mig.Migrate(db))

	var blocksJSON string
	require.NoError(t, db.Raw(`SELECT blocks FROM conversation_messages WHERE conversation_id=1`).Row().Scan(&blocksJSON))

	var blocks []map[string]any
	require.NoError(t, json.Unmarshal([]byte(blocksJSON), &blocks))
	require.Len(t, blocks, 1, "should be one backfilled block")
	assert.Equal(t, "text", blocks[0]["type"])
	assert.Equal(t, "fallback text", blocks[0]["text"], "key MUST be 'text' (matches Task 9 deserializeBlocks contract), not 'content'")
}
