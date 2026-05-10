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

func TestGormStore_AppendAndLoad(t *testing.T) {
	ctx, s := setupGormStore(t)
	err := s.AppendMessage(ctx, "1", 0, agent.Message{
		Role:    agent.RoleUser,
		Content: []agent.ContentBlock{agent.TextBlock{Text: "hello"}},
	})
	require.NoError(t, err)

	msgs, _, err := s.LoadConversation(ctx, "1")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, agent.RoleUser, msgs[0].Role)
	tb, ok := msgs[0].Content[0].(agent.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "hello", tb.Text)
}

func TestGormStore_UpdateMessage_FillsPartialAndUsage(t *testing.T) {
	ctx, s := setupGormStore(t)
	require.NoError(t, s.AppendMessage(ctx, "1", 0, agent.Message{Role: agent.RoleAssistant, PartialReason: agent.PartialStreaming}))
	usage := agent.Usage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15}
	err := s.UpdateMessage(ctx, "1", 0, agent.Message{
		Role:          agent.RoleAssistant,
		Content:       []agent.ContentBlock{agent.TextBlock{Text: "done"}},
		PartialReason: agent.PartialErrored,
		Usage:         &usage,
	})
	require.NoError(t, err)

	msgs, _, err := s.LoadConversation(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, agent.PartialErrored, msgs[0].PartialReason)
	require.NotNil(t, msgs[0].Usage)
	assert.Equal(t, 15, msgs[0].Usage.TotalTokens)
}

func TestGormStore_TruncateAfter(t *testing.T) {
	ctx, s := setupGormStore(t)
	for i := 0; i < 4; i++ {
		require.NoError(t, s.AppendMessage(ctx, "1", i, agent.Message{
			Role: agent.RoleUser, Content: []agent.ContentBlock{agent.TextBlock{Text: strconv.Itoa(i)}},
		}))
	}
	require.NoError(t, s.TruncateAfter(ctx, "1", 2))
	msgs, _, err := s.LoadConversation(ctx, "1")
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
}

func TestGormStore_LoadFixesStreamingTail(t *testing.T) {
	ctx, s := setupGormStore(t)
	require.NoError(t, s.AppendMessage(ctx, "1", 0, agent.Message{
		Role: agent.RoleUser, Content: []agent.ContentBlock{agent.TextBlock{Text: "hi"}},
	}))
	require.NoError(t, s.AppendMessage(ctx, "1", 1, agent.Message{
		Role: agent.RoleAssistant, PartialReason: agent.PartialStreaming,
		Content: []agent.ContentBlock{agent.TextBlock{Text: "half "}},
	}))

	msgs, _, err := s.LoadConversation(ctx, "1")
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, agent.PartialErrored, msgs[1].PartialReason, "streaming tail should be rewritten to errored on load")
	tb, ok := msgs[1].Content[0].(agent.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "half ", tb.Text, "partial content preserved")
}

func TestGormStore_PreservesMetadataBlock(t *testing.T) {
	ctx, s := setupGormStore(t)
	require.NoError(t, s.AppendMessage(ctx, "1", 0, agent.Message{
		Role: agent.RoleUser,
		Content: []agent.ContentBlock{
			agent.TextBlock{Text: "expanded body"},
			agent.MetadataBlock{Key: "display", Value: "@srv1 status"},
		},
	}))
	msgs, _, err := s.LoadConversation(ctx, "1")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Content, 2)
	mb, ok := msgs[0].Content[1].(agent.MetadataBlock)
	require.True(t, ok)
	assert.Equal(t, "display", mb.Key)
	assert.Equal(t, "@srv1 status", mb.Value)
}
