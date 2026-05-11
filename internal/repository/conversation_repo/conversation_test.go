package conversation_repo

import (
	"context"
	"testing"

	"github.com/cago-frame/cago/database/db"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

const testTokenUsageJSON = `{"total":42}` //nolint:gosec // false positive: test fixture JSON, not a credential

// setupTest 每个测试一份独立的内存 SQLite，并通过 db.SetDefault 绑定到 cago db.Ctx(ctx)，
// 与同仓库的 snippet_repo 测试一致。
func setupTest(t *testing.T) (context.Context, ConversationRepo) {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&conversation_entity.Message{}, &conversation_entity.Conversation{}))
	db.SetDefault(gdb)
	return context.Background(), NewConversation()
}

func TestAppendAt_InsertsRow(t *testing.T) {
	ctx, repo := setupTest(t)
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

func TestAppendAt_DuplicateSortOrderRejected(t *testing.T) {
	ctx, repo := setupTest(t)
	require.NoError(t, repo.AppendAt(ctx, 1, 0, &conversation_entity.Message{
		ConversationID: 1, Role: "user", SortOrder: 0,
	}))
	err := repo.AppendAt(ctx, 1, 0, &conversation_entity.Message{
		ConversationID: 1, Role: "user", SortOrder: 0,
	})
	assert.Error(t, err, "AppendAt with duplicate (conv_id, sort_order) should fail per unique index")
}

func TestUpdateAt_UpdatesRow(t *testing.T) {
	ctx, repo := setupTest(t)
	require.NoError(t, repo.AppendAt(ctx, 1, 0, &conversation_entity.Message{ConversationID: 1, Role: "assistant", Blocks: `[]`, SortOrder: 0}))
	err := repo.UpdateAt(ctx, 1, 0, &conversation_entity.Message{
		ConversationID: 1, Role: "assistant", Blocks: `[{"type":"text","text":"done"}]`,
		PartialReason: "errored", TokenUsage: testTokenUsageJSON, SortOrder: 0,
	})
	require.NoError(t, err)
	got, err := repo.LoadOrdered(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "errored", got[0].PartialReason)
	assert.Contains(t, got[0].TokenUsage, "42")
}

func TestTruncateFrom_DeletesTail(t *testing.T) {
	ctx, repo := setupTest(t)
	for i := 0; i < 4; i++ {
		require.NoError(t, repo.AppendAt(ctx, 1, i, &conversation_entity.Message{ConversationID: 1, Role: "user", SortOrder: i}))
	}
	err := repo.TruncateFrom(ctx, 1, 2)
	require.NoError(t, err)
	got, err := repo.LoadOrdered(ctx, 1)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, 0, got[0].SortOrder)
	assert.Equal(t, 1, got[1].SortOrder)
}

func TestLoadOrdered_OrderedBySortOrder(t *testing.T) {
	ctx, repo := setupTest(t)
	require.NoError(t, repo.AppendAt(ctx, 1, 2, &conversation_entity.Message{ConversationID: 1, Role: "a", SortOrder: 2}))
	require.NoError(t, repo.AppendAt(ctx, 1, 0, &conversation_entity.Message{ConversationID: 1, Role: "b", SortOrder: 0}))
	require.NoError(t, repo.AppendAt(ctx, 1, 1, &conversation_entity.Message{ConversationID: 1, Role: "c", SortOrder: 1}))
	got, err := repo.LoadOrdered(ctx, 1)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"b", "c", "a"}, []string{got[0].Role, got[1].Role, got[2].Role})
}
