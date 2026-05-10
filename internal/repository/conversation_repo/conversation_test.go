package conversation_repo

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

func setupTestDB(t *testing.T) (context.Context, *gorm.DB, ConversationRepo) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&conversation_entity.Message{}, &conversation_entity.Conversation{}))
	return context.Background(), db, NewConversation(db)
}

func TestAppendAt_InsertsRow(t *testing.T) {
	ctx, _, repo := setupTestDB(t)
	err := repo.AppendAt(ctx, 1, 0, &conversation_entity.Message{
		ConversationID: 1,
		Role:           "user",
		Blocks:         `[{"type":"text","text":"hi"}]`,
		PartialReason:  "",
		SortOrder:      0,
	})
	require.NoError(t, err)

	got, err := repo.LoadOrdered(ctx, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "user", got[0].Role)
	assert.Equal(t, 0, got[0].SortOrder)
}

func TestUpdateAt_UpdatesRow(t *testing.T) {
	ctx, _, repo := setupTestDB(t)
	require.NoError(t, repo.AppendAt(ctx, 1, 0, &conversation_entity.Message{ConversationID: 1, Role: "assistant", Blocks: `[]`, SortOrder: 0}))
	err := repo.UpdateAt(ctx, 1, 0, &conversation_entity.Message{
		ConversationID: 1, Role: "assistant", Blocks: `[{"type":"text","text":"done"}]`,
		PartialReason: "errored", TokenUsage: `{"total":42}`, SortOrder: 0,
	})
	require.NoError(t, err)
	got, _ := repo.LoadOrdered(ctx, 1)
	assert.Equal(t, "errored", got[0].PartialReason)
	assert.Contains(t, got[0].TokenUsage, "42")
}

func TestTruncateFrom_DeletesTail(t *testing.T) {
	ctx, _, repo := setupTestDB(t)
	for i := 0; i < 4; i++ {
		require.NoError(t, repo.AppendAt(ctx, 1, i, &conversation_entity.Message{ConversationID: 1, Role: "user", SortOrder: i}))
	}
	err := repo.TruncateFrom(ctx, 1, 2)
	require.NoError(t, err)
	got, _ := repo.LoadOrdered(ctx, 1)
	require.Len(t, got, 2)
	assert.Equal(t, 0, got[0].SortOrder)
	assert.Equal(t, 1, got[1].SortOrder)
}

func TestLoadOrdered_OrderedBySortOrder(t *testing.T) {
	ctx, _, repo := setupTestDB(t)
	require.NoError(t, repo.AppendAt(ctx, 1, 2, &conversation_entity.Message{ConversationID: 1, Role: "a", SortOrder: 2}))
	require.NoError(t, repo.AppendAt(ctx, 1, 0, &conversation_entity.Message{ConversationID: 1, Role: "b", SortOrder: 0}))
	require.NoError(t, repo.AppendAt(ctx, 1, 1, &conversation_entity.Message{ConversationID: 1, Role: "c", SortOrder: 1}))
	got, _ := repo.LoadOrdered(ctx, 1)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"b", "c", "a"}, []string{got[0].Role, got[1].Role, got[2].Role})
}
