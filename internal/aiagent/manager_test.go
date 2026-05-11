package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/provider/providertest"
	"github.com/cago-frame/cago/database/db"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
	"github.com/opskat/opskat/internal/repository/conversation_repo"
)

// noopMention always returns the input unchanged.
type noopMention struct{}

func (noopMention) Expand(_ context.Context, raw string) (string, []map[string]any, []string, error) {
	return raw, nil, nil, nil
}

type noopTabOpener struct{}

func (noopTabOpener) Open(_ context.Context, _ string) error { return nil }

type allowChecker struct{}

func (allowChecker) Check(_ context.Context, _ string, _ map[string]any) (bool, string, error) {
	return true, "", nil
}

type discardAudit struct{}

func (discardAudit) Write(_ context.Context, _, _, _ string, _ bool) error { return nil }

func setupManager(t *testing.T, prov provider.Provider) *Manager {
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&conversation_entity.Message{}, &conversation_entity.Conversation{}))
	require.NoError(t, gdb.Exec(`INSERT INTO conversations (id, title, provider_type) VALUES (1, 'test', 'mock'), (2, 'other', 'mock')`).Error)
	db.SetDefault(gdb)
	conversation_repo.RegisterConversation(conversation_repo.NewConversation())

	em := &handleCaptureEmitter{}
	resolver := newFakeResolver()

	return NewManager(ManagerOptions{
		Provider:      prov,
		System:        "you are helpful",
		Tools:         nil,
		MaxRounds:     50,
		Emitter:       em,
		Resolver:      resolver.Resolver(),
		LocalGrants:   &fakeGrants{},
		AuditWriter:   discardAudit{},
		PolicyChecker: allowChecker{},
		Mention:       noopMention{},
		TabOpener:     noopTabOpener{},
	})
}

func TestManager_Handle_LazyLoad(t *testing.T) {
	prov := providertest.New().QueueStream(
		provider.StreamChunk{ContentDelta: "hi"},
		provider.StreamChunk{FinishReason: provider.FinishStop},
	)
	m := setupManager(t, prov)
	defer func() { _ = m.Close() }()

	h1, err := m.Handle(context.Background(), 1)
	require.NoError(t, err)
	h2, err := m.Handle(context.Background(), 1)
	require.NoError(t, err)
	assert.Same(t, h1, h2, "second call returns the cached handle")
}

func TestManager_DifferentConvsIndependent(t *testing.T) {
	prov := providertest.New().
		QueueStream(provider.StreamChunk{ContentDelta: "a"}, provider.StreamChunk{FinishReason: provider.FinishStop}).
		QueueStream(provider.StreamChunk{ContentDelta: "b"}, provider.StreamChunk{FinishReason: provider.FinishStop})
	m := setupManager(t, prov)
	defer func() { _ = m.Close() }()

	h1, err := m.Handle(context.Background(), 1)
	require.NoError(t, err)
	h2, err := m.Handle(context.Background(), 2)
	require.NoError(t, err)
	assert.NotSame(t, h1, h2)

	require.NoError(t, h1.Send(context.Background(), "x", "x"))
	require.NoError(t, h2.Send(context.Background(), "y", "y"))
}

func TestManager_Handle_LoadsExistingConv(t *testing.T) {
	prov := providertest.New().QueueStream(
		provider.StreamChunk{ContentDelta: "ack"},
		provider.StreamChunk{FinishReason: provider.FinishStop},
	)
	m := setupManager(t, prov)
	defer func() { _ = m.Close() }()

	// Pre-seed message via repo (simulate restart with prior history)
	repo := conversation_repo.Conversation()
	require.NoError(t, repo.AppendAt(context.Background(), 1, 0, &conversation_entity.Message{
		ConversationID: 1, Role: "user", SortOrder: 0,
		Blocks: `[{"type":"text","text":"earlier"}]`,
	}))

	h, err := m.Handle(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, h.Conv().Len(), "conv should be loaded with the pre-existing message")
	first, err := h.Conv().MessageAt(0)
	require.NoError(t, err)
	require.Equal(t, agent.RoleUser, first.Role)
	tb, ok := first.Content[0].(agent.TextBlock)
	require.True(t, ok)
	assert.Equal(t, "earlier", tb.Text)

	// Sanity: prevent unused-import warning
	_ = ai.StreamEvent{}
}
