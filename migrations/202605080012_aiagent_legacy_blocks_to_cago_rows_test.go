package migrations

import (
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// runMigrationsUpTo applies all registered migrations up to (and including) targetID.
// 复用 production 用的 gormigrate 包，所以测的就是发出去的版本。
func runMigrationsUpTo(t *testing.T, targetID string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	all := allMigrationsForTest()
	upto := []*gormigrate.Migration{}
	for _, m := range all {
		upto = append(upto, m)
		if m.ID == targetID {
			break
		}
	}
	m := gormigrate.New(db, gormigrate.DefaultOptions, upto)
	require.NoError(t, m.Migrate())
	return db
}

// allMigrationsForTest 返回与 migrations.go 同序的全量迁移列表。test-only。
// 维护性：如果新增迁移记得追加。
func allMigrationsForTest() []*gormigrate.Migration {
	return []*gormigrate.Migration{
		migration202603220001(),
		migration202603260001(),
		migration202603270001(),
		migration202603290001(),
		migration202603300001(),
		migration202603300002(),
		migration202603310001(),
		migration202604050001(),
		migration202604140001(),
		migration202604160001(),
		migration202604170001(),
		migration202604220001(),
		migration202604230001(),
		migration202604270001(),
		migration202605010001(),
		migration202605060001(),
		migration202605070001(),
		migration202605080001(),
		migration202605080010(),
		migration202605080012(),
	}
}

// 测试数据迁移 202605080012：把前 cago 时代由前端 SaveConversationMessages 写入的
// 单行多 Block 消息（kind=”/cago_id=”），按 Block 边界展开为 cago 形态的多行
// （text / tool_call / tool_result）。一个 assistant 回合"原本一气泡多 Block"在
// 渲染端会被聚合还原。
//
// 兼容前提：202605080011 已经把 conversations 上有 cago session_data 的会话覆盖过
// 了；本迁移只动剩下的 legacy 行（kind=” 且 cago_id=”）。
func TestMigrate202605080012_LegacyAssistantTurnWithToolExpands(t *testing.T) {
	db := runMigrationsUpTo(t, "202605080010")

	conv := &conversation_entity.Conversation{
		Title: "legacy", ProviderType: "anthropic", Status: conversation_entity.StatusActive,
		Createtime: 1000, Updatetime: 1000,
	}
	require.NoError(t, db.Create(conv).Error)

	// 用户行：legacy 形态（kind/cago_id 全空），带 mentions
	userMentions, _ := json.Marshal([]conversation_entity.MentionRef{{AssetID: 7, Name: "srv"}})
	require.NoError(t, db.Create(&conversation_entity.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "hi @srv",
		Mentions:       string(userMentions),
		SortOrder:      0,
		Createtime:     1100,
	}).Error)

	// Assistant 行：legacy 单行多 Block + token_usage
	asstBlocks, _ := json.Marshal([]conversation_entity.ContentBlock{
		{Type: "text", Content: "let me check"},
		{Type: "tool", ToolName: "list_assets", ToolInput: `{"x":1}`, ToolCallID: "call-1", Status: "completed", Content: "ok-result"},
		{Type: "text", Content: "done"},
	})
	usage, _ := json.Marshal(conversation_entity.TokenUsage{InputTokens: 10, OutputTokens: 20})
	require.NoError(t, db.Create(&conversation_entity.Message{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "",
		Blocks:         string(asstBlocks),
		TokenUsage:     string(usage),
		SortOrder:      1,
		Createtime:     1200,
	}).Error)

	next := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{migration202605080012()})
	require.NoError(t, next.Migrate())

	var got []conversation_entity.Message
	require.NoError(t, db.Where("conversation_id = ?", conv.ID).Order("sort_order ASC").Find(&got).Error)

	// 5 行：user_text + asst_text + tool_call + tool_result + asst_text
	require.Len(t, got, 5, "1 user legacy + 1 asst legacy(3 blocks) → 1 + 4 = 5 cago 行")

	// [0] user 文本行
	assert.Equal(t, "text", got[0].Kind)
	assert.Equal(t, "user", got[0].Role)
	assert.Equal(t, "user", got[0].Origin)
	assert.Equal(t, "hi @srv", got[0].Content)
	assert.NotEmpty(t, got[0].CagoID, "legacy 迁移必须给每行铸新的 cago_id；前端 in-memory 渲染靠它做 key")
	assert.True(t, got[0].Persist)
	assert.NotEmpty(t, got[0].Mentions, "user 行的 mentions 应保留在迁出后的 user text 行上")

	// [1] assistant 第一段文本
	assert.Equal(t, "text", got[1].Kind)
	assert.Equal(t, "assistant", got[1].Role)
	assert.Equal(t, "model", got[1].Origin)
	assert.Equal(t, "let me check", got[1].Content)
	assert.NotEmpty(t, got[1].CagoID)

	// [2] tool_call
	assert.Equal(t, "tool_call", got[2].Kind)
	assert.Equal(t, "assistant", got[2].Role)
	assert.Equal(t, "model", got[2].Origin)
	var tc struct {
		ID   string          `json:"id"`
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	}
	require.NoError(t, json.Unmarshal([]byte(got[2].ToolCallJSON), &tc))
	assert.Equal(t, "call-1", tc.ID)
	assert.Equal(t, "list_assets", tc.Name)
	assert.JSONEq(t, `{"x":1}`, string(tc.Args))

	// [3] tool_result
	assert.Equal(t, "tool_result", got[3].Kind)
	assert.Equal(t, "tool", got[3].Role)
	assert.Equal(t, "tool", got[3].Origin)
	var tr struct {
		Result any    `json:"result,omitempty"`
		Err    string `json:"err,omitempty"`
	}
	require.NoError(t, json.Unmarshal([]byte(got[3].ToolResultJSON), &tr))
	assert.Equal(t, "ok-result", tr.Result)
	assert.Empty(t, tr.Err)

	// [4] assistant 收尾文本，token_usage 跟到最后一行 cago 消息（与 cago bridge.lastAssistantMsgID 语义一致）
	assert.Equal(t, "text", got[4].Kind)
	assert.Equal(t, "assistant", got[4].Role)
	assert.Equal(t, "done", got[4].Content)
	assert.NotEmpty(t, got[4].TokenUsage, "token_usage 应保留在 assistant 回合最后一行 cago 消息上")

	// 所有行 sort_order 必须重排为 0..N-1
	for i, m := range got {
		assert.Equal(t, i, m.SortOrder, "sort_order 必须连续重排")
		assert.NotEmpty(t, m.CagoID, "row[%d] 缺 cago_id", i)
	}
	// 5 行 cago_id 必须互不相同（前端 React key + repo Upsert 自然键依赖）
	seen := map[string]bool{}
	for i, m := range got {
		require.False(t, seen[m.CagoID], "row[%d] 复用了 cago_id %q", i, m.CagoID)
		seen[m.CagoID] = true
	}
}

// Pure text legacy 行（Blocks 为空，只有 Content）也要展开成一条 text-kind cago 行。
// 这是更常见的 case：legacy 写入路径下大部分用户/助手消息都没 tool。
func TestMigrate202605080012_LegacyTextOnlyExpands(t *testing.T) {
	db := runMigrationsUpTo(t, "202605080010")
	conv := &conversation_entity.Conversation{
		Title: "x", ProviderType: "anthropic", Status: conversation_entity.StatusActive,
	}
	require.NoError(t, db.Create(conv).Error)

	require.NoError(t, db.Create(&conversation_entity.Message{
		ConversationID: conv.ID, Role: "user", Content: "hi", SortOrder: 0,
	}).Error)
	require.NoError(t, db.Create(&conversation_entity.Message{
		ConversationID: conv.ID, Role: "assistant", Content: "hello back", SortOrder: 1,
	}).Error)

	next := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{migration202605080012()})
	require.NoError(t, next.Migrate())

	var got []conversation_entity.Message
	require.NoError(t, db.Where("conversation_id = ?", conv.ID).Order("sort_order ASC").Find(&got).Error)
	require.Len(t, got, 2)
	assert.Equal(t, "text", got[0].Kind)
	assert.Equal(t, "user", got[0].Origin)
	assert.Equal(t, "hi", got[0].Content)
	assert.Equal(t, "text", got[1].Kind)
	assert.Equal(t, "model", got[1].Origin)
	assert.Equal(t, "hello back", got[1].Content)
}

// 已经是 cago 形态的会话（kind/cago_id 都填了）必须原样不动 —— 防止 idempotency 退化。
func TestMigrate202605080012_AlreadyCagoShape_NoOp(t *testing.T) {
	db := runMigrationsUpTo(t, "202605080010")
	conv := &conversation_entity.Conversation{
		Title: "cago", ProviderType: "anthropic", Status: conversation_entity.StatusActive, ThreadID: "tid-1",
	}
	require.NoError(t, db.Create(conv).Error)

	require.NoError(t, db.Create(&conversation_entity.Message{
		ConversationID: conv.ID, Role: "user", Content: "hi",
		Kind: "text", Origin: "user", CagoID: "m1", Persist: true,
		SortOrder: 0, Createtime: 1100,
	}).Error)

	next := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{migration202605080012()})
	require.NoError(t, next.Migrate())

	var got []conversation_entity.Message
	require.NoError(t, db.Where("conversation_id = ?", conv.ID).Find(&got).Error)
	require.Len(t, got, 1)
	assert.Equal(t, "m1", got[0].CagoID)
	assert.Equal(t, "hi", got[0].Content)
}

// Tool block 的 Status="error" → tool_result 的 err 字段而不是 result。
func TestMigrate202605080012_LegacyErrorToolResult(t *testing.T) {
	db := runMigrationsUpTo(t, "202605080010")
	conv := &conversation_entity.Conversation{
		Title: "err", ProviderType: "anthropic", Status: conversation_entity.StatusActive,
	}
	require.NoError(t, db.Create(conv).Error)

	require.NoError(t, db.Create(&conversation_entity.Message{
		ConversationID: conv.ID, Role: "user", Content: "do it", SortOrder: 0,
	}).Error)
	asstBlocks, _ := json.Marshal([]conversation_entity.ContentBlock{
		{Type: "tool", ToolName: "exec", ToolInput: `{}`, ToolCallID: "c-1", Status: "error", Content: "boom"},
	})
	require.NoError(t, db.Create(&conversation_entity.Message{
		ConversationID: conv.ID, Role: "assistant", Blocks: string(asstBlocks), SortOrder: 1,
	}).Error)

	next := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{migration202605080012()})
	require.NoError(t, next.Migrate())

	var got []conversation_entity.Message
	require.NoError(t, db.Where("conversation_id = ?", conv.ID).Order("sort_order ASC").Find(&got).Error)
	require.Len(t, got, 3) // user + tool_call + tool_result
	assert.Equal(t, "tool_result", got[2].Kind)
	var tr struct {
		Result any    `json:"result,omitempty"`
		Err    string `json:"err,omitempty"`
	}
	require.NoError(t, json.Unmarshal([]byte(got[2].ToolResultJSON), &tr))
	assert.Equal(t, "boom", tr.Err)
	assert.Nil(t, tr.Result)
}

// Tool block 没有 result（Status=running，被中断的 tool）→ 不写 tool_result 行，
// 渲染端把这种孤儿 tool_call 显示为 "running" 占位。
func TestMigrate202605080012_LegacyRunningToolNoResult(t *testing.T) {
	db := runMigrationsUpTo(t, "202605080010")
	conv := &conversation_entity.Conversation{
		Title: "runn", ProviderType: "anthropic", Status: conversation_entity.StatusActive,
	}
	require.NoError(t, db.Create(conv).Error)

	require.NoError(t, db.Create(&conversation_entity.Message{
		ConversationID: conv.ID, Role: "user", Content: "go", SortOrder: 0,
	}).Error)
	asstBlocks, _ := json.Marshal([]conversation_entity.ContentBlock{
		{Type: "tool", ToolName: "exec", ToolInput: `{}`, ToolCallID: "c-1", Status: "running"},
	})
	require.NoError(t, db.Create(&conversation_entity.Message{
		ConversationID: conv.ID, Role: "assistant", Blocks: string(asstBlocks), SortOrder: 1,
	}).Error)

	next := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{migration202605080012()})
	require.NoError(t, next.Migrate())

	var got []conversation_entity.Message
	require.NoError(t, db.Where("conversation_id = ?", conv.ID).Order("sort_order ASC").Find(&got).Error)
	require.Len(t, got, 2) // user + tool_call only（无 result）
	assert.Equal(t, "tool_call", got[1].Kind)
	assert.Empty(t, got[1].ToolResultJSON)
}
