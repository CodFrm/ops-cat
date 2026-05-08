package conversation_repo

import (
	"context"
	"testing"
	"time"

	"github.com/cago-frame/cago/database/db"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

func setupConvRepo(t *testing.T) (context.Context, ConversationRepo, *gorm.DB) {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(
		&conversation_entity.Conversation{},
		&conversation_entity.Message{},
	))
	db.SetDefault(gdb)
	return context.Background(), NewConversation(), gdb
}

func newConv(t *testing.T, ctx context.Context, r ConversationRepo) *conversation_entity.Conversation {
	t.Helper()
	c := &conversation_entity.Conversation{
		Title:        "test",
		ProviderType: "anthropic",
		Model:        "claude-x",
		Status:       conversation_entity.StatusActive,
		Createtime:   time.Now().Unix(),
		Updatetime:   time.Now().Unix(),
	}
	require.NoError(t, r.Create(ctx, c))
	require.NotZero(t, c.ID)
	return c
}

func msg(convID int64, cagoID, role, content string, sortOrder int) *conversation_entity.Message {
	return &conversation_entity.Message{
		ConversationID: convID,
		CagoID:         cagoID,
		Role:           role,
		Content:        content,
		Kind:           "text",
		Origin:         "user",
		Persist:        true,
		SortOrder:      sortOrder,
		MsgTime:        time.Now().Unix(),
		Createtime:     time.Now().Unix(),
	}
}

func TestUpsertMessagesByCagoID_FirstSaveCreatesRows(t *testing.T) {
	ctx, r, _ := setupConvRepo(t)
	c := newConv(t, ctx, r)

	m1 := msg(c.ID, "cago-1", "user", "hello", 1)
	m2 := msg(c.ID, "cago-2", "assistant", "hi there", 2)

	require.NoError(t, r.UpsertMessagesByCagoID(ctx, c.ID, []*conversation_entity.Message{m1, m2}))

	got, err := r.ListMessages(ctx, c.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "cago-1", got[0].CagoID)
	assert.Equal(t, "hello", got[0].Content)
	assert.Equal(t, "cago-2", got[1].CagoID)
	assert.Equal(t, "hi there", got[1].Content)
}

func TestUpsertMessagesByCagoID_SecondSaveUpsertsAndRemovesAbsent(t *testing.T) {
	ctx, r, _ := setupConvRepo(t)
	c := newConv(t, ctx, r)

	m1 := msg(c.ID, "cago-1", "user", "hello", 1)
	m2 := msg(c.ID, "cago-2", "assistant", "hi there", 2)
	require.NoError(t, r.UpsertMessagesByCagoID(ctx, c.ID, []*conversation_entity.Message{m1, m2}))

	// 第二次：m1 内容修改、m2 缺席（应被删除）、m3 新增
	m1Edited := msg(c.ID, "cago-1", "user", "hello-edited", 1)
	m3 := msg(c.ID, "cago-3", "assistant", "third", 2)
	require.NoError(t, r.UpsertMessagesByCagoID(ctx, c.ID, []*conversation_entity.Message{m1Edited, m3}))

	got, err := r.ListMessages(ctx, c.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	// 按 sort_order 升序：cago-1 然后 cago-3
	assert.Equal(t, "cago-1", got[0].CagoID)
	assert.Equal(t, "hello-edited", got[0].Content)
	assert.Equal(t, "cago-3", got[1].CagoID)
	assert.Equal(t, "third", got[1].Content)
}

func TestUpsertMessagesByCagoID_PreservesExtensionColumns(t *testing.T) {
	ctx, r, gdb := setupConvRepo(t)
	c := newConv(t, ctx, r)

	m1 := msg(c.ID, "cago-1", "user", "hello", 1)
	require.NoError(t, r.UpsertMessagesByCagoID(ctx, c.ID, []*conversation_entity.Message{m1}))

	// 模拟 System pending 缓存写入 mentions / token_usage 扩展列
	require.NoError(t, gdb.Model(&conversation_entity.Message{}).
		Where("conversation_id = ? AND cago_id = ?", c.ID, "cago-1").
		Updates(map[string]any{
			"mentions":    `[{"assetId":1,"name":"a","start":0,"end":2}]`,
			"token_usage": `{"inputTokens":10,"outputTokens":20}`,
		}).Error)

	// 第二次 upsert：同一 cago_id，content 变化，但 mentions/token_usage 不应被清空
	m1Edited := msg(c.ID, "cago-1", "user", "hello-edited", 1)
	require.NoError(t, r.UpsertMessagesByCagoID(ctx, c.ID, []*conversation_entity.Message{m1Edited}))

	got, err := r.ListMessages(ctx, c.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "hello-edited", got[0].Content)
	assert.Equal(t, `[{"assetId":1,"name":"a","start":0,"end":2}]`, got[0].Mentions, "mentions must be preserved")
	assert.Equal(t, `{"inputTokens":10,"outputTokens":20}`, got[0].TokenUsage, "token_usage must be preserved")
}

func TestUpsertMessagesByCagoID_EmptyClearsAll(t *testing.T) {
	ctx, r, _ := setupConvRepo(t)
	c := newConv(t, ctx, r)

	m1 := msg(c.ID, "cago-1", "user", "hello", 1)
	m2 := msg(c.ID, "cago-2", "assistant", "hi", 2)
	require.NoError(t, r.UpsertMessagesByCagoID(ctx, c.ID, []*conversation_entity.Message{m1, m2}))

	require.NoError(t, r.UpsertMessagesByCagoID(ctx, c.ID, nil))

	got, err := r.ListMessages(ctx, c.ID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestUpdateState(t *testing.T) {
	ctx, r, _ := setupConvRepo(t)
	c := newConv(t, ctx, r)
	originalUpdatetime := c.Updatetime

	// 等 1 秒以保证 updatetime 推进（unix 秒粒度）
	time.Sleep(1100 * time.Millisecond)

	threadID := "thread-xyz"
	stateJSON := `{"k":"v"}`
	require.NoError(t, r.UpdateState(ctx, c.ID, threadID, stateJSON))

	got, err := r.Find(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, threadID, got.ThreadID)
	assert.Equal(t, stateJSON, got.StateValues)
	assert.Greater(t, got.Updatetime, originalUpdatetime, "updatetime should be refreshed")

	// 空字符串视为清空
	require.NoError(t, r.UpdateState(ctx, c.ID, "", ""))
	got, err = r.Find(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "", got.ThreadID)
	assert.Equal(t, "", got.StateValues)
	values, err := got.GetStateValues()
	require.NoError(t, err)
	assert.Nil(t, values)
}

func TestUpdateMessageTokenUsage(t *testing.T) {
	ctx, r, _ := setupConvRepo(t)
	c := newConv(t, ctx, r)
	m1 := msg(c.ID, "cago-1", "user", "hi", 1)
	require.NoError(t, r.UpsertMessagesByCagoID(ctx, c.ID, []*conversation_entity.Message{m1}))

	require.NoError(t, r.UpdateMessageTokenUsage(ctx, c.ID, "cago-1", `{"inputTokens":12}`))

	got, err := r.ListMessages(ctx, c.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, `{"inputTokens":12}`, got[0].TokenUsage)

	// missing cago_id is a silent no-op
	require.NoError(t, r.UpdateMessageTokenUsage(ctx, c.ID, "missing", `{}`))
}
