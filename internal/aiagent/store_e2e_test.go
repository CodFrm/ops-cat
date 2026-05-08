package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/cago/database/db"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
	"github.com/opskat/opskat/internal/service/conversation_svc"
)

func setupE2E(t *testing.T) (context.Context, *gorm.DB) {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(
		&conversation_entity.Conversation{},
		&conversation_entity.Message{},
	))
	db.SetDefault(gdb)
	// 真实 repo 注入到 service 单例使用的全局变量
	conversation_repo.RegisterConversation(conversation_repo.NewConversation())
	return context.Background(), gdb
}

func newE2EConv(t *testing.T, ctx context.Context) *conversation_entity.Conversation {
	t.Helper()
	c := &conversation_entity.Conversation{
		Title:        "e2e",
		ProviderType: "anthropic",
		Status:       conversation_entity.StatusActive,
	}
	require.NoError(t, conversation_svc.Conversation().Create(ctx, c))
	return c
}

func TestGormStoreE2E_RoundTrip(t *testing.T) {
	ctx, _ := setupE2E(t)
	c := newE2EConv(t, ctx)

	// mentions/usage providers 都传 nil；本测试只验证 cago messages + state 的真实链路。
	store := NewGormStore(nil, nil)
	sid := fmt.Sprintf("conv_%d", c.ID)

	data := agent.SessionData{
		Messages: []agent.Message{
			{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Origin: agent.MessageOriginUser, Text: "hi", Persist: true},
			{ID: "m2", Kind: agent.MessageKindToolCall, Role: agent.RoleAssistant, Origin: agent.MessageOriginModel, Persist: true,
				ToolCall: &agent.ToolCall{ID: "call-1", Name: "tool", Args: json.RawMessage(`{"x":1}`)}},
			{ID: "m3", Kind: agent.MessageKindToolResult, Role: agent.RoleTool, Origin: agent.MessageOriginTool, Persist: true,
				ToolResult: &agent.ToolResult{Result: "ok"}},
			{ID: "m4", Kind: agent.MessageKindText, Role: agent.RoleAssistant, Origin: agent.MessageOriginModel, Text: "summary", Persist: true},
		},
		State: agent.State{ThreadID: "thread-xyz", Values: map[string]string{"k": "v"}},
	}
	require.NoError(t, store.Save(ctx, sid, data))

	got, err := store.Load(ctx, sid)
	require.NoError(t, err)
	require.Len(t, got.Messages, 4)
	assert.Equal(t, "m1", got.Messages[0].ID)
	assert.Equal(t, "hi", got.Messages[0].Text)
	require.NotNil(t, got.Messages[1].ToolCall)
	assert.Equal(t, "tool", got.Messages[1].ToolCall.Name)
	require.NotNil(t, got.Messages[2].ToolResult)
	assert.Equal(t, "ok", got.Messages[2].ToolResult.Result)
	assert.Equal(t, "summary", got.Messages[3].Text)
	assert.Equal(t, "thread-xyz", got.State.ThreadID)
	assert.Equal(t, "v", got.State.Values["k"])
}

func TestGormStoreE2E_UpsertReplacesOnSecondSave(t *testing.T) {
	ctx, _ := setupE2E(t)
	c := newE2EConv(t, ctx)

	store := NewGormStore(nil, nil)
	sid := fmt.Sprintf("conv_%d", c.ID)

	first := agent.SessionData{Messages: []agent.Message{
		{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Origin: agent.MessageOriginUser, Text: "v1", Persist: true},
		{ID: "m2", Kind: agent.MessageKindText, Role: agent.RoleAssistant, Origin: agent.MessageOriginModel, Text: "a1", Persist: true},
	}}
	require.NoError(t, store.Save(ctx, sid, first))

	second := agent.SessionData{Messages: []agent.Message{
		{ID: "m1", Kind: agent.MessageKindText, Role: agent.RoleUser, Origin: agent.MessageOriginUser, Text: "v1-edited", Persist: true},
		{ID: "m3", Kind: agent.MessageKindText, Role: agent.RoleAssistant, Origin: agent.MessageOriginModel, Text: "a2", Persist: true},
	}}
	require.NoError(t, store.Save(ctx, sid, second))

	got, err := store.Load(ctx, sid)
	require.NoError(t, err)
	require.Len(t, got.Messages, 2)
	assert.Equal(t, "m1", got.Messages[0].ID)
	assert.Equal(t, "v1-edited", got.Messages[0].Text)
	assert.Equal(t, "m3", got.Messages[1].ID)
}

// fakeMentionsProvider 单次 pop 一组 mentions，用于验证 gormStore.Save 把 pending
// mentions 正确写入 conversation_messages.mentions 列。
// 实现 aiagent 包内未导出的 pendingMentionsProvider 接口（结构性匹配）。
type fakeMentionsProvider struct {
	pending []ai.MentionedAsset
	popped  bool
}

func (f *fakeMentionsProvider) popPendingMentions() []ai.MentionedAsset {
	if f.popped {
		return nil
	}
	f.popped = true
	return f.pending
}

func TestGormStoreE2E_DrainsMentionsToUserRow(t *testing.T) {
	ctx, gdb := setupE2E(t)
	c := newE2EConv(t, ctx)

	fmp := &fakeMentionsProvider{pending: []ai.MentionedAsset{{AssetID: 99, Name: "edge-prod"}}}
	store := NewGormStore(fmp, nil)
	sid := fmt.Sprintf("conv_%d", c.ID)

	require.NoError(t, store.Save(ctx, sid, agent.SessionData{
		Messages: []agent.Message{
			{ID: "u1", Kind: agent.MessageKindText, Role: agent.RoleUser, Origin: agent.MessageOriginUser, Text: "@edge-prod hi", Persist: true},
		},
	}))

	var rows []conversation_entity.Message
	require.NoError(t, gdb.Where("conversation_id = ?", c.ID).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0].Mentions, "edge-prod", "mentions JSON should contain the asset name")
	assert.Contains(t, rows[0].Mentions, "99", "mentions JSON should contain the asset id")
}
