package snippet_repo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cago-frame/cago/database/db"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/snippet_entity"
)

func setupRepo(t *testing.T) (context.Context, SnippetRepo) {
	t.Helper()
	// 每个测试使用独立的匿名内存库
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&snippet_entity.Snippet{}))
	db.SetDefault(gdb)
	return context.Background(), NewSnippet()
}

func newSnippet(name, category string) *snippet_entity.Snippet {
	return &snippet_entity.Snippet{
		Name:     name,
		Category: category,
		Content:  "echo " + name,
		Source:   snippet_entity.SourceUser,
		Status:   snippet_entity.StatusActive,
	}
}

func TestSnippetRepo_CreateAndGetByID(t *testing.T) {
	ctx, r := setupRepo(t)
	s := newSnippet("a", snippet_entity.CategoryShell)
	require.NoError(t, r.Create(ctx, s))
	assert.NotZero(t, s.ID)

	got, err := r.GetByID(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, "a", got.Name)
	assert.Equal(t, snippet_entity.CategoryShell, got.Category)
	assert.Equal(t, snippet_entity.StatusActive, got.Status)
}

func TestSnippetRepo_Update(t *testing.T) {
	ctx, r := setupRepo(t)
	s := newSnippet("a", snippet_entity.CategoryShell)
	require.NoError(t, r.Create(ctx, s))

	s.Name = "a-renamed"
	s.Description = "desc"
	require.NoError(t, r.Update(ctx, s))

	got, err := r.GetByID(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, "a-renamed", got.Name)
	assert.Equal(t, "desc", got.Description)
}

func TestSnippetRepo_Update_IgnoresProtectedFields(t *testing.T) {
	ctx, r := setupRepo(t)
	// 用户片段，带初始 category / source
	s := newSnippet("a", snippet_entity.CategoryShell)
	require.NoError(t, r.Create(ctx, s))

	// 尝试在传入实体上篡改受保护字段
	s.Name = "renamed"
	s.Category = snippet_entity.CategorySQL // 不应生效
	s.Source = "ext:evil"                   // 不应生效
	s.SourceRef = "hijacked"                // 不应生效
	require.NoError(t, r.Update(ctx, s))

	got, err := r.GetByID(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed", got.Name)
	assert.Equal(t, snippet_entity.CategoryShell, got.Category, "category must be immutable via Update")
	assert.Equal(t, snippet_entity.SourceUser, got.Source, "source must be immutable via Update")
	assert.Equal(t, "", got.SourceRef, "source_ref must be immutable via Update")
}

func TestSnippetRepo_SoftDelete(t *testing.T) {
	ctx, r := setupRepo(t)
	s := newSnippet("a", snippet_entity.CategoryShell)
	require.NoError(t, r.Create(ctx, s))

	require.NoError(t, r.SoftDelete(ctx, s.ID))

	_, err := r.GetByID(ctx, s.ID)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound), "expected ErrRecordNotFound, got %v", err)
}

func TestSnippetRepo_Find_CategoriesFilter(t *testing.T) {
	ctx, r := setupRepo(t)
	require.NoError(t, r.Create(ctx, newSnippet("a", snippet_entity.CategoryShell)))
	require.NoError(t, r.Create(ctx, newSnippet("b", snippet_entity.CategorySQL)))
	require.NoError(t, r.Create(ctx, newSnippet("c", snippet_entity.CategoryPrompt)))

	list, err := r.Find(ctx, SnippetQuery{Categories: []string{snippet_entity.CategoryShell, snippet_entity.CategorySQL}})
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestSnippetRepo_Find_KeywordSources(t *testing.T) {
	ctx, r := setupRepo(t)
	s1 := newSnippet("alpha", snippet_entity.CategoryShell)
	s1.Description = "first"
	require.NoError(t, r.Create(ctx, s1))

	s2 := newSnippet("beta", snippet_entity.CategoryShell)
	s2.Description = "second alpha-ish"
	require.NoError(t, r.Create(ctx, s2))

	s3 := newSnippet("gamma", snippet_entity.CategoryShell)
	s3.Source = "ext:foo"
	require.NoError(t, r.Create(ctx, s3))

	// Keyword hits name and description
	list, err := r.Find(ctx, SnippetQuery{Keyword: "alpha"})
	require.NoError(t, err)
	assert.Len(t, list, 2)

	// Sources filter: user only
	list, err = r.Find(ctx, SnippetQuery{Sources: []string{snippet_entity.SourceUser}})
	require.NoError(t, err)
	assert.Len(t, list, 2)

	// Sources filter: extension only
	list, err = r.Find(ctx, SnippetQuery{Sources: []string{"ext:foo"}})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "gamma", list[0].Name)
}

func TestSnippetRepo_Find_Ordering(t *testing.T) {
	ctx, r := setupRepo(t)

	s1 := newSnippet("a", snippet_entity.CategoryShell)
	require.NoError(t, r.Create(ctx, s1))
	s2 := newSnippet("b", snippet_entity.CategoryShell)
	require.NoError(t, r.Create(ctx, s2))
	s3 := newSnippet("c", snippet_entity.CategoryShell)
	require.NoError(t, r.Create(ctx, s3))

	// 使 s2 的 use_count=5, s3 的 use_count=1; s1 的 last_used_at 最新
	for i := 0; i < 5; i++ {
		require.NoError(t, r.TouchUsage(ctx, s2.ID))
	}
	require.NoError(t, r.TouchUsage(ctx, s3.ID))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, r.TouchUsage(ctx, s1.ID))

	// use_count_desc: s2 (5), s3 (1), s1 (1)
	list, err := r.Find(ctx, SnippetQuery{OrderBy: "use_count_desc"})
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, "b", list[0].Name)

	// default: 最近使用优先，s1 应排第一
	list, err = r.Find(ctx, SnippetQuery{})
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, "a", list[0].Name)
}

func TestSnippetRepo_FindBySourceRef(t *testing.T) {
	ctx, r := setupRepo(t)
	s := newSnippet("ext-seed", snippet_entity.CategoryShell)
	s.Source = "ext:foo"
	s.SourceRef = "seed-1"
	require.NoError(t, r.Create(ctx, s))

	got, err := r.FindBySourceRef(ctx, "ext:foo", "seed-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, s.ID, got.ID)

	missing, err := r.FindBySourceRef(ctx, "ext:foo", "missing")
	require.NoError(t, err)
	assert.Nil(t, missing)
}

func TestSnippetRepo_TouchUsage(t *testing.T) {
	ctx, r := setupRepo(t)
	s := newSnippet("a", snippet_entity.CategoryShell)
	require.NoError(t, r.Create(ctx, s))
	assert.EqualValues(t, 0, s.UseCount)

	require.NoError(t, r.TouchUsage(ctx, s.ID))

	got, err := r.GetByID(ctx, s.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, got.UseCount)
	require.NotNil(t, got.LastUsedAt)
}

func TestSnippetRepo_SetLastAssets(t *testing.T) {
	t.Run("writes comma-separated IDs", func(t *testing.T) {
		ctx, r := setupRepo(t)
		s := newSnippet("a", snippet_entity.CategoryShell)
		require.NoError(t, r.Create(ctx, s))

		require.NoError(t, r.SetLastAssets(ctx, s.ID, []int64{3, 1, 2}))

		got, err := r.GetByID(ctx, s.ID)
		require.NoError(t, err)
		assert.Equal(t, "3,1,2", got.LastAssetIDs)
	})

	t.Run("empty slice clears last_asset_ids", func(t *testing.T) {
		ctx, r := setupRepo(t)
		s := newSnippet("b", snippet_entity.CategoryShell)
		require.NoError(t, r.Create(ctx, s))
		require.NoError(t, r.SetLastAssets(ctx, s.ID, []int64{5}))

		require.NoError(t, r.SetLastAssets(ctx, s.ID, []int64{}))

		got, err := r.GetByID(ctx, s.ID)
		require.NoError(t, err)
		assert.Equal(t, "", got.LastAssetIDs)
	})

	t.Run("rejects id=0", func(t *testing.T) {
		ctx, r := setupRepo(t)
		err := r.SetLastAssets(ctx, 0, []int64{1})
		assert.Error(t, err)
	})

	t.Run("no-op on soft-deleted snippet", func(t *testing.T) {
		ctx, r := setupRepo(t)
		s := newSnippet("c", snippet_entity.CategoryShell)
		require.NoError(t, r.Create(ctx, s))
		require.NoError(t, r.SoftDelete(ctx, s.ID))

		// SetLastAssets on soft-deleted row should not error but also not update
		require.NoError(t, r.SetLastAssets(ctx, s.ID, []int64{99}))

		// Verify the deleted row still has empty last_asset_ids (can't GetByID — it's deleted)
		var check snippet_entity.Snippet
		db.Ctx(ctx).Unscoped().Where("id = ?", s.ID).First(&check)
		assert.Equal(t, "", check.LastAssetIDs, "soft-deleted snippet should not be updated")
	})
}

func TestSnippetRepo_UpsertExtensionSeed_Insert(t *testing.T) {
	ctx, r := setupRepo(t)
	s := &snippet_entity.Snippet{
		Name: "List topics", Category: "kafka", Content: "kafka-topics --list",
		Source: "ext:kafka-ext", SourceRef: "list-topics", Status: snippet_entity.StatusActive,
	}
	require.NoError(t, r.UpsertExtensionSeed(ctx, s))
	assert.NotZero(t, s.ID)

	got, err := r.FindBySourceRef(ctx, "ext:kafka-ext", "list-topics")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "List topics", got.Name)
	assert.Equal(t, "kafka", got.Category)
}

func TestSnippetRepo_UpsertExtensionSeed_UpdatePreservesCounters(t *testing.T) {
	ctx, r := setupRepo(t)

	// 初次插入
	first := &snippet_entity.Snippet{
		Name: "n1", Category: "kafka", Content: "v1",
		Source: "ext:kafka-ext", SourceRef: "seed-1", Status: snippet_entity.StatusActive,
	}
	require.NoError(t, r.UpsertExtensionSeed(ctx, first))

	// 模拟使用数
	require.NoError(t, r.TouchUsage(ctx, first.ID))
	require.NoError(t, r.TouchUsage(ctx, first.ID))

	// 再次 upsert：name/content 变更，use_count/last_used_at 必须保留
	second := &snippet_entity.Snippet{
		Name: "n1-updated", Category: "kafka", Content: "v2", Description: "new desc",
		Source: "ext:kafka-ext", SourceRef: "seed-1", Status: snippet_entity.StatusActive,
	}
	require.NoError(t, r.UpsertExtensionSeed(ctx, second))
	assert.Equal(t, first.ID, second.ID, "ID should be the existing row's ID")

	got, err := r.FindBySourceRef(ctx, "ext:kafka-ext", "seed-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "n1-updated", got.Name)
	assert.Equal(t, "v2", got.Content)
	assert.Equal(t, "new desc", got.Description)
	assert.EqualValues(t, 2, got.UseCount, "use_count must be preserved across upsert")
	assert.NotNil(t, got.LastUsedAt)
}

func TestSnippetRepo_DeleteExtensionSeedsMissing(t *testing.T) {
	ctx, r := setupRepo(t)
	// 3 条扩展 seed + 1 条用户片段
	for _, key := range []string{"k1", "k2", "k3"} {
		s := &snippet_entity.Snippet{
			Name: key, Category: snippet_entity.CategoryShell, Content: "x",
			Source: "ext:foo", SourceRef: key, Status: snippet_entity.StatusActive,
		}
		require.NoError(t, r.Create(ctx, s))
	}
	user := newSnippet("user-one", snippet_entity.CategoryShell)
	require.NoError(t, r.Create(ctx, user))

	// 只保留 k1
	require.NoError(t, r.DeleteExtensionSeedsMissing(ctx, "ext:foo", []string{"k1"}))

	remaining, err := r.Find(ctx, SnippetQuery{})
	require.NoError(t, err)
	names := map[string]bool{}
	for _, s := range remaining {
		names[s.Name] = true
	}
	assert.True(t, names["k1"], "k1 should remain")
	assert.False(t, names["k2"], "k2 should be deleted")
	assert.False(t, names["k3"], "k3 should be deleted")
	assert.True(t, names["user-one"], "user-created snippet must not be affected")
}

func TestSnippetRepo_DeleteExtensionSeedsMissing_EmptyKeepClearsAll(t *testing.T) {
	ctx, r := setupRepo(t)
	for _, key := range []string{"k1", "k2"} {
		s := &snippet_entity.Snippet{
			Name: key, Category: snippet_entity.CategoryShell, Content: "x",
			Source: "ext:foo", SourceRef: key, Status: snippet_entity.StatusActive,
		}
		require.NoError(t, r.Create(ctx, s))
	}
	require.NoError(t, r.DeleteExtensionSeedsMissing(ctx, "ext:foo", nil))

	list, err := r.Find(ctx, SnippetQuery{Sources: []string{"ext:foo"}})
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestSnippetRepo_HardDeleteBySource(t *testing.T) {
	ctx, r := setupRepo(t)
	s1 := newSnippet("a", snippet_entity.CategoryShell)
	s1.Source = "ext:foo"
	s1.SourceRef = "r1"
	require.NoError(t, r.Create(ctx, s1))

	s2 := newSnippet("b", snippet_entity.CategoryShell)
	s2.Source = "ext:foo"
	s2.SourceRef = "r2"
	require.NoError(t, r.Create(ctx, s2))

	s3 := newSnippet("c", snippet_entity.CategoryShell)
	require.NoError(t, r.Create(ctx, s3))

	require.NoError(t, r.HardDeleteBySource(ctx, "ext:foo"))

	list, err := r.Find(ctx, SnippetQuery{})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "c", list[0].Name)
}
