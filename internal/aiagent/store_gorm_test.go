package aiagent

import (
	"context"
	"strconv"
	"testing"

	"github.com/cago-frame/agents/agent"
	agentstore "github.com/cago-frame/agents/agent/store"
	"github.com/cago-frame/cago/database/db"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
)

func setupGormStore(t *testing.T) (context.Context, agentstore.Store) {
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(
		&conversation_entity.Message{},
		&conversation_entity.Conversation{},
	))
	db.SetDefault(gdb)
	conversation_repo.RegisterConversation(conversation_repo.NewConversation())
	// Seed conversation row so foreign-key style invariants are happy
	require.NoError(t, gdb.Exec(`INSERT INTO conversations (id, title, provider_type) VALUES (1, 'test', 'mock')`).Error)
	return context.Background(), NewGormStore()
}

// mustEncode encodes a typed agent.Message to the on-wire StoredMessage form
// the Store contract consumes. Tests write agent.Message because it's the
// ergonomic typed shape; the store sees the JSON-discriminator form.
func mustEncode(t *testing.T, m agent.Message) agentstore.StoredMessage {
	t.Helper()
	sm, err := agentstore.EncodeMessage(m)
	require.NoError(t, err)
	return sm
}

// loadDecoded is the inverse: read the store, then decode StoredMessage back
// to typed agent.Message so tests can type-assert blocks directly.
func loadDecoded(t *testing.T, ctx context.Context, s agentstore.Store, sessionID string) []agent.Message {
	t.Helper()
	sms, _, err := s.LoadConversation(ctx, sessionID)
	require.NoError(t, err)
	msgs, err := agentstore.DecodeMessages(sms)
	require.NoError(t, err)
	return msgs
}

func TestGormStore_AppendAndLoad(t *testing.T) {
	ctx, s := setupGormStore(t)
	err := s.AppendMessage(ctx, "1", 0, mustEncode(t, agent.Message{
		Role:    agent.RoleUser,
		Content: []agent.ContentBlock{agent.TextBlock{Text: "hello"}},
	}))
	require.NoError(t, err)

	msgs := loadDecoded(t, ctx, s, "1")
	require.Len(t, msgs, 1)
	assert.Equal(t, agent.RoleUser, msgs[0].Role)
	tb, ok := msgs[0].Content[0].(agent.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "hello", tb.Text)
}

func TestGormStore_UpdateMessage_FillsPartialAndUsage(t *testing.T) {
	ctx, s := setupGormStore(t)
	require.NoError(t, s.AppendMessage(ctx, "1", 0, mustEncode(t, agent.Message{
		Role: agent.RoleAssistant, PartialReason: agent.PartialStreaming,
	})))
	usage := agent.Usage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15}
	err := s.UpdateMessage(ctx, "1", 0, mustEncode(t, agent.Message{
		Role:          agent.RoleAssistant,
		Content:       []agent.ContentBlock{agent.TextBlock{Text: "done"}},
		PartialReason: agent.PartialErrored,
		PartialDetail: "upstream busy: 503",
		Usage:         &usage,
	}))
	require.NoError(t, err)

	msgs := loadDecoded(t, ctx, s, "1")
	assert.Equal(t, agent.PartialErrored, msgs[0].PartialReason)
	assert.Equal(t, "upstream busy: 503", msgs[0].PartialDetail, "error detail must round-trip through DB")
	require.NotNil(t, msgs[0].Usage)
	assert.Equal(t, 15, msgs[0].Usage.TotalTokens)
}

func TestGormStore_TruncateAfter(t *testing.T) {
	ctx, s := setupGormStore(t)
	for i := 0; i < 4; i++ {
		require.NoError(t, s.AppendMessage(ctx, "1", i, mustEncode(t, agent.Message{
			Role: agent.RoleUser, Content: []agent.ContentBlock{agent.TextBlock{Text: strconv.Itoa(i)}},
		})))
	}
	require.NoError(t, s.TruncateAfter(ctx, "1", 2))
	msgs := loadDecoded(t, ctx, s, "1")
	assert.Len(t, msgs, 2)
}

func TestGormStore_LoadFixesStreamingTail(t *testing.T) {
	ctx, s := setupGormStore(t)
	require.NoError(t, s.AppendMessage(ctx, "1", 0, mustEncode(t, agent.Message{
		Role: agent.RoleUser, Content: []agent.ContentBlock{agent.TextBlock{Text: "hi"}},
	})))
	require.NoError(t, s.AppendMessage(ctx, "1", 1, mustEncode(t, agent.Message{
		Role: agent.RoleAssistant, PartialReason: agent.PartialStreaming,
		Content: []agent.ContentBlock{agent.TextBlock{Text: "half "}},
	})))

	msgs := loadDecoded(t, ctx, s, "1")
	require.Len(t, msgs, 2)
	assert.Equal(t, agent.PartialErrored, msgs[1].PartialReason, "streaming tail should be rewritten to errored on load")
	assert.NotEmpty(t, msgs[1].PartialDetail, "crash-recovery should backfill a detail string so UI can render the error state")
	tb, ok := msgs[1].Content[0].(agent.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "half ", tb.Text, "partial content preserved")
}

// TestGormStore_PreservesDisplayTextBlock 验证 cago Audience 体系下 user 消息
// 的 raw 显示文本（@srv1 status 这种）以 DisplayTextBlock 形态持久化 + 完整 round-trip。
// 这是历史回放 + 重启后前端 UserMessage chip 渲染的依据。
func TestGormStore_PreservesDisplayTextBlock(t *testing.T) {
	ctx, s := setupGormStore(t)
	require.NoError(t, s.AppendMessage(ctx, "1", 0, mustEncode(t, agent.Message{
		Role: agent.RoleUser,
		Content: []agent.ContentBlock{
			agent.TextBlock{Text: "expanded body"},
			agent.DisplayTextBlock{Text: "@srv1 status"},
		},
	})))
	msgs := loadDecoded(t, ctx, s, "1")
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Content, 2, "TextBlock + DisplayTextBlock both kept")
	tb, ok := msgs[0].Content[0].(agent.TextBlock)
	require.True(t, ok, "block 0 should be TextBlock (LLM body)")
	assert.Equal(t, "expanded body", tb.Text)
	dt, ok := msgs[0].Content[1].(agent.DisplayTextBlock)
	require.True(t, ok, "block 1 should be DisplayTextBlock (UI raw)")
	assert.Equal(t, "@srv1 status", dt.Text)
}

func TestGormStore_ToolUseAndToolResultRoundTrip(t *testing.T) {
	ctx, s := setupGormStore(t)

	toolUse := agent.ToolUseBlock{
		ID:      "toolu_01abc",
		Name:    "list_servers",
		Input:   map[string]any{"region": "us-west", "limit": float64(5)},
		RawArgs: `{"region":"us-west","limit":5}`,
	}
	require.NoError(t, s.AppendMessage(ctx, "1", 0, mustEncode(t, agent.Message{
		Role:    agent.RoleAssistant,
		Content: []agent.ContentBlock{toolUse},
	})))

	toolResult := agent.ToolResultBlock{
		ToolUseID: "toolu_01abc",
		IsError:   false,
		Content:   []agent.ContentBlock{agent.TextBlock{Text: "found 3 servers"}},
	}
	require.NoError(t, s.AppendMessage(ctx, "1", 1, mustEncode(t, agent.Message{
		Role:    agent.RoleUser,
		Content: []agent.ContentBlock{toolResult},
	})))

	msgs := loadDecoded(t, ctx, s, "1")
	require.Len(t, msgs, 2)

	gotUse, ok := msgs[0].Content[0].(agent.ToolUseBlock)
	require.True(t, ok)
	assert.Equal(t, "toolu_01abc", gotUse.ID)
	assert.Equal(t, "list_servers", gotUse.Name)
	assert.Equal(t, "us-west", gotUse.Input["region"])
	assert.Equal(t, float64(5), gotUse.Input["limit"])
	assert.Equal(t, `{"region":"us-west","limit":5}`, gotUse.RawArgs)

	gotResult, ok := msgs[1].Content[0].(agent.ToolResultBlock)
	require.True(t, ok)
	assert.Equal(t, "toolu_01abc", gotResult.ToolUseID)
	assert.False(t, gotResult.IsError)
	require.Len(t, gotResult.Content, 1)
	innerText, ok := gotResult.Content[0].(agent.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "found 3 servers", innerText.Text)
}

func TestGormStore_LoadEmpty_ReturnsNil(t *testing.T) {
	ctx, s := setupGormStore(t)
	// No AppendMessage calls; conversation row exists (seeded by setup) but no messages.
	sms, branch, err := s.LoadConversation(ctx, "1")
	require.NoError(t, err)
	assert.Nil(t, sms, "empty conv should return nil messages per cago Store contract")
	assert.Equal(t, agentstore.BranchInfo{}, branch)
}

// TestGormStore_ParsesInlineMentionsFromTextBlock 回归"继续被渲染成 chip"那个 bug：
//  1. user 消息的 LLM body (TextBlock) 含 inline <mention> 标签 → row.Mentions
//     应解析出 assetId/name/start/end 全字段（取代旧 StashMentions 旁路）
//  2. 紧接着一条无 mention 的 user 消息 → row.Mentions 必须为 "[]"，不能继承上一条的
//     mention（旧旁路的 bug 就是这条没被弹空，落到了这条 row 上）
func TestGormStore_ParsesInlineMentionsFromTextBlock(t *testing.T) {
	ctx, s := setupGormStore(t)

	llmBody := `<mention asset-id="37" name="local-docker" type="ssh" host="192.168.8.141" group="本地" start="0" end="13">@local-docker</mention> 看看容器`
	require.NoError(t, s.AppendMessage(ctx, "1", 0, mustEncode(t, agent.Message{
		Role: agent.RoleUser,
		Content: []agent.ContentBlock{
			agent.TextBlock{Text: llmBody},
			agent.DisplayTextBlock{Text: "@local-docker 看看容器"},
		},
	})))

	rows, err := conversation_repo.Conversation().LoadOrdered(ctx, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0].Mentions, `"assetId":37`)
	assert.Contains(t, rows[0].Mentions, `"name":"local-docker"`)
	assert.Contains(t, rows[0].Mentions, `"start":0`)
	assert.Contains(t, rows[0].Mentions, `"end":13`)
}

func TestGormStore_NoMentionFallsBackToEmptyArray(t *testing.T) {
	ctx, s := setupGormStore(t)

	require.NoError(t, s.AppendMessage(ctx, "1", 0, mustEncode(t, agent.Message{
		Role:    agent.RoleUser,
		Content: []agent.ContentBlock{agent.TextBlock{Text: "plain question"}},
	})))

	rows, err := conversation_repo.Conversation().LoadOrdered(ctx, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "[]", rows[0].Mentions, "无 <mention> 标签应该写空数组，不沿用上条")
}

// 回归"继续"被渲染成 chip 按钮的根因：旧 StashMentions 旁路如果第二条 user 消息
// 没有清空 stash，会把第一条的 mention offset 打到第二条 row 上。新 XML inline 路径
// 不再有旁路，只读自身 TextBlock，应该天然无串扰。
func TestGormStore_SecondUserMsgWithoutMentionIsClean(t *testing.T) {
	ctx, s := setupGormStore(t)

	// msg 0: 带 mention
	require.NoError(t, s.AppendMessage(ctx, "1", 0, mustEncode(t, agent.Message{
		Role: agent.RoleUser,
		Content: []agent.ContentBlock{
			agent.TextBlock{Text: `<mention asset-id="37" name="local-docker" type="ssh" host="" group="" start="0" end="13">@local-docker</mention> 看`},
			agent.DisplayTextBlock{Text: "@local-docker 看"},
		},
	})))
	// msg 1: assistant
	require.NoError(t, s.AppendMessage(ctx, "1", 1, mustEncode(t, agent.Message{
		Role:    agent.RoleAssistant,
		Content: []agent.ContentBlock{agent.TextBlock{Text: "ok"}},
	})))
	// msg 2: 用户继续，无 mention
	require.NoError(t, s.AppendMessage(ctx, "1", 2, mustEncode(t, agent.Message{
		Role:    agent.RoleUser,
		Content: []agent.ContentBlock{agent.TextBlock{Text: "继续"}},
	})))

	rows, err := conversation_repo.Conversation().LoadOrdered(ctx, 1)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Contains(t, rows[0].Mentions, `"assetId":37`, "msg 0 应保留它自己的 mention")
	assert.Equal(t, "[]", rows[2].Mentions, "msg 2 不应继承 msg 0 的 mention")
}

func TestGormStore_ImageBlockInlineRoundTrip(t *testing.T) {
	ctx, s := setupGormStore(t)
	inline := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG magic bytes
	require.NoError(t, s.AppendMessage(ctx, "1", 0, mustEncode(t, agent.Message{
		Role: agent.RoleUser,
		Content: []agent.ContentBlock{
			agent.ImageBlock{
				MediaType: "image/png",
				Source:    agent.BlobSource{Inline: inline},
			},
		},
	})))
	msgs := loadDecoded(t, ctx, s, "1")
	require.Len(t, msgs, 1)
	img, ok := msgs[0].Content[0].(agent.ImageBlock)
	require.True(t, ok)
	assert.Equal(t, "image/png", img.MediaType)
	assert.Equal(t, "", img.Source.URL)
	assert.Equal(t, inline, img.Source.Inline, "inline bytes must round-trip cleanly")
}
