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
	return context.Background(), NewGormStore(nil)
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
		Usage:         &usage,
	}))
	require.NoError(t, err)

	msgs := loadDecoded(t, ctx, s, "1")
	assert.Equal(t, agent.PartialErrored, msgs[0].PartialReason)
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
