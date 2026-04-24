package snippet_svc

import (
	"context"
	"errors"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/snippet_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/asset_repo/mock_asset_repo"
	"github.com/opskat/opskat/internal/repository/snippet_repo"
	"github.com/opskat/opskat/internal/repository/snippet_repo/mock_snippet_repo"
)

type svcFixture struct {
	ctx      context.Context
	svc      SnippetSvc
	snippets *mock_snippet_repo.MockSnippetRepo
	assets   *mock_asset_repo.MockAssetRepo
}

func setupSvcTest(t *testing.T) *svcFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	snippets := mock_snippet_repo.NewMockSnippetRepo(ctrl)
	assets := mock_asset_repo.NewMockAssetRepo(ctrl)
	snippet_repo.RegisterSnippet(snippets)
	asset_repo.RegisterAsset(assets)

	return &svcFixture{
		ctx:      context.Background(),
		svc:      NewSnippetSvc(NewCategoryRegistry()),
		snippets: snippets,
		assets:   assets,
	}
}

func TestSnippetSvc_Create(t *testing.T) {
	convey.Convey("Create 片段", t, func() {
		convey.Convey("合法片段创建成功", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, s *snippet_entity.Snippet) error {
					s.ID = 42
					return nil
				},
			)
			got, err := f.svc.Create(f.ctx, CreateReq{
				Name: "ls", Category: snippet_entity.CategoryShell, Content: "ls -al",
			})
			assert.NoError(t, err)
			assert.EqualValues(t, 42, got.ID)
			assert.Equal(t, snippet_entity.SourceUser, got.Source)
			assert.Equal(t, snippet_entity.StatusActive, got.Status)
		})

		convey.Convey("名称为空拒绝", func() {
			f := setupSvcTest(t)
			_, err := f.svc.Create(f.ctx, CreateReq{
				Name: " ", Category: snippet_entity.CategoryShell, Content: "ls",
			})
			assert.Error(t, err)
		})

		convey.Convey("非法分类拒绝", func() {
			f := setupSvcTest(t)
			_, err := f.svc.Create(f.ctx, CreateReq{
				Name: "x", Category: "bogus", Content: "ls",
			})
			assert.Error(t, err)
		})

		convey.Convey("内容为空拒绝", func() {
			f := setupSvcTest(t)
			_, err := f.svc.Create(f.ctx, CreateReq{
				Name: "x", Category: snippet_entity.CategoryShell, Content: " ",
			})
			assert.Error(t, err)
		})
	})
}

func TestSnippetSvc_Update(t *testing.T) {
	convey.Convey("Update 片段", t, func() {
		convey.Convey("合法更新成功", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().GetByID(gomock.Any(), int64(10)).Return(&snippet_entity.Snippet{
				ID: 10, Name: "old", Category: snippet_entity.CategoryShell, Content: "ls",
				Source: snippet_entity.SourceUser, Status: snippet_entity.StatusActive,
			}, nil)
			f.snippets.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)

			got, err := f.svc.Update(f.ctx, UpdateReq{
				ID: 10, Name: "new", Content: "pwd",
			})
			assert.NoError(t, err)
			assert.Equal(t, "new", got.Name)
			assert.Equal(t, "pwd", got.Content)
			assert.Equal(t, snippet_entity.CategoryShell, got.Category, "category must be preserved")
		})

		convey.Convey("扩展来源只读拒绝", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().GetByID(gomock.Any(), int64(10)).Return(&snippet_entity.Snippet{
				ID: 10, Name: "ext", Category: snippet_entity.CategoryShell, Content: "ls",
				Source: "ext:foo", Status: snippet_entity.StatusActive,
			}, nil)
			_, err := f.svc.Update(f.ctx, UpdateReq{ID: 10, Name: "new", Content: "pwd"})
			assert.Error(t, err)
		})
	})
}

func TestSnippetSvc_Delete(t *testing.T) {
	convey.Convey("Delete 片段", t, func() {
		convey.Convey("软删除成功", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().GetByID(gomock.Any(), int64(1)).Return(&snippet_entity.Snippet{
				ID: 1, Source: snippet_entity.SourceUser,
			}, nil)
			f.snippets.EXPECT().SoftDelete(gomock.Any(), int64(1)).Return(nil)
			assert.NoError(t, f.svc.Delete(f.ctx, 1))
		})
		convey.Convey("扩展来源只读拒绝", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().GetByID(gomock.Any(), int64(1)).Return(&snippet_entity.Snippet{
				ID: 1, Source: "ext:foo",
			}, nil)
			assert.Error(t, f.svc.Delete(f.ctx, 1))
		})
	})
}

func TestSnippetSvc_Duplicate(t *testing.T) {
	convey.Convey("Duplicate 片段", t, func() {
		convey.Convey("克隆带 (copy) 后缀并复位来源", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().GetByID(gomock.Any(), int64(1)).Return(&snippet_entity.Snippet{
				ID: 1, Name: "orig", Category: snippet_entity.CategoryShell, Content: "ls",
				Source: "ext:foo", SourceRef: "ref-1", Status: snippet_entity.StatusActive,
			}, nil)
			f.snippets.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, s *snippet_entity.Snippet) error {
					s.ID = 2
					return nil
				},
			)

			got, err := f.svc.Duplicate(f.ctx, 1)
			assert.NoError(t, err)
			assert.Equal(t, "orig (copy)", got.Name)
			assert.Equal(t, snippet_entity.SourceUser, got.Source)
			assert.Equal(t, "", got.SourceRef)
		})
	})
}

func TestSnippetSvc_RecordUse(t *testing.T) {
	f := setupSvcTest(t)
	f.snippets.EXPECT().TouchUsage(gomock.Any(), int64(1)).Return(nil)
	assert.NoError(t, f.svc.RecordUse(f.ctx, 1))
}

func TestSnippetSvc_RecordUse_Errors(t *testing.T) {
	f := setupSvcTest(t)
	f.snippets.EXPECT().TouchUsage(gomock.Any(), int64(1)).Return(errors.New("boom"))
	assert.Error(t, f.svc.RecordUse(f.ctx, 1))
}

func TestSnippetSvc_SetGetLastAssets(t *testing.T) {
	convey.Convey("SetLastAssets / GetLastAssets", t, func() {
		convey.Convey("happy-path: SetLastAssets delegates to repo", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().SetLastAssets(gomock.Any(), int64(5), []int64{1, 2, 3}).Return(nil)
			assert.NoError(t, f.svc.SetLastAssets(f.ctx, 5, []int64{1, 2, 3}))
		})

		convey.Convey("SetLastAssets rejects id=0", func() {
			f := setupSvcTest(t)
			assert.Error(t, f.svc.SetLastAssets(f.ctx, 0, []int64{1}))
		})

		convey.Convey("GetLastAssets: happy-path returns live asset IDs", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().GetByID(gomock.Any(), int64(5)).Return(&snippet_entity.Snippet{
				ID: 5, Category: snippet_entity.CategoryShell, LastAssetIDs: "1,2",
				Status: snippet_entity.StatusActive,
			}, nil)
			f.assets.EXPECT().Find(gomock.Any(), int64(1)).Return(&asset_entity.Asset{
				ID: 1, Type: asset_entity.AssetTypeSSH, Status: asset_entity.StatusActive,
			}, nil)
			f.assets.EXPECT().Find(gomock.Any(), int64(2)).Return(&asset_entity.Asset{
				ID: 2, Type: asset_entity.AssetTypeSSH, Status: asset_entity.StatusActive,
			}, nil)
			ids, err := f.svc.GetLastAssets(f.ctx, 5)
			assert.NoError(t, err)
			assert.Equal(t, []int64{1, 2}, ids)
		})

		convey.Convey("GetLastAssets: stale ID filtered out (not found)", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().GetByID(gomock.Any(), int64(5)).Return(&snippet_entity.Snippet{
				ID: 5, Category: snippet_entity.CategoryShell, LastAssetIDs: "1,999",
				Status: snippet_entity.StatusActive,
			}, nil)
			f.assets.EXPECT().Find(gomock.Any(), int64(1)).Return(&asset_entity.Asset{
				ID: 1, Type: asset_entity.AssetTypeSSH, Status: asset_entity.StatusActive,
			}, nil)
			f.assets.EXPECT().Find(gomock.Any(), int64(999)).Return(nil, gorm.ErrRecordNotFound)
			ids, err := f.svc.GetLastAssets(f.ctx, 5)
			assert.NoError(t, err)
			assert.Equal(t, []int64{1}, ids)
		})

		convey.Convey("GetLastAssets: type-mismatch filtered out", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().GetByID(gomock.Any(), int64(5)).Return(&snippet_entity.Snippet{
				ID: 5, Category: snippet_entity.CategoryShell, LastAssetIDs: "1",
				Status: snippet_entity.StatusActive,
			}, nil)
			f.assets.EXPECT().Find(gomock.Any(), int64(1)).Return(&asset_entity.Asset{
				ID: 1, Type: asset_entity.AssetTypeDatabase, Status: asset_entity.StatusActive,
			}, nil)
			ids, err := f.svc.GetLastAssets(f.ctx, 5)
			assert.NoError(t, err)
			assert.Empty(t, ids)
		})

		convey.Convey("GetLastAssets rejects id=0", func() {
			f := setupSvcTest(t)
			_, err := f.svc.GetLastAssets(f.ctx, 0)
			assert.Error(t, err)
		})

		convey.Convey("GetLastAssets: StatusDeleted asset filtered out", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().GetByID(gomock.Any(), int64(5)).Return(&snippet_entity.Snippet{
				ID: 5, Category: snippet_entity.CategoryShell, LastAssetIDs: "1,2",
				Status: snippet_entity.StatusActive,
			}, nil)
			f.assets.EXPECT().Find(gomock.Any(), int64(1)).Return(&asset_entity.Asset{
				ID: 1, Type: asset_entity.AssetTypeSSH, Status: asset_entity.StatusDeleted,
			}, nil)
			f.assets.EXPECT().Find(gomock.Any(), int64(2)).Return(&asset_entity.Asset{
				ID: 2, Type: asset_entity.AssetTypeSSH, Status: asset_entity.StatusActive,
			}, nil)
			ids, err := f.svc.GetLastAssets(f.ctx, 5)
			assert.NoError(t, err)
			assert.Equal(t, []int64{2}, ids)
		})

		convey.Convey("SetLastAssets caps at 50 IDs", func() {
			f := setupSvcTest(t)
			ids60 := make([]int64, 60)
			for i := range ids60 {
				ids60[i] = int64(i + 1)
			}
			f.snippets.EXPECT().SetLastAssets(gomock.Any(), int64(5), gomock.Any()).
				Do(func(_ context.Context, _ int64, got []int64) {
					assert.Len(t, got, 50)
					assert.Equal(t, ids60[:50], got)
				}).Return(nil)
			assert.NoError(t, f.svc.SetLastAssets(f.ctx, 5, ids60))
		})
	})
}

func TestSnippetSvc_ListCategories(t *testing.T) {
	svc := NewSnippetSvc(NewCategoryRegistry())
	assert.Len(t, svc.ListCategories(), 5)
}

func TestSnippetSvc_KnownCategoryIDs(t *testing.T) {
	reg := NewCategoryRegistry()
	reg.SetExtensionProvider(ExtensionCategoryProviderFunc(func() []ExtensionCategory {
		return []ExtensionCategory{
			{ID: "kafka", AssetType: "kafka", Label: "Kafka", ExtensionRef: "kafka-ext"},
		}
	}))
	reg.RefreshFromExtensions()
	svc := NewSnippetSvc(reg)
	ids := svc.KnownCategoryIDs()
	assert.Contains(t, ids, snippet_entity.CategoryShell)
	assert.Contains(t, ids, "kafka")
}

func TestSnippetSvc_RefreshCategories(t *testing.T) {
	reg := NewCategoryRegistry()
	var supplied []ExtensionCategory
	reg.SetExtensionProvider(ExtensionCategoryProviderFunc(func() []ExtensionCategory {
		return supplied
	}))
	svc := NewSnippetSvc(reg)
	assert.Len(t, svc.ListCategories(), 5)

	supplied = []ExtensionCategory{{ID: "kafka", AssetType: "kafka", Label: "Kafka", ExtensionRef: "kx"}}
	svc.RefreshCategories()
	assert.Len(t, svc.ListCategories(), 6)

	supplied = nil
	svc.RefreshCategories()
	assert.Len(t, svc.ListCategories(), 5)
}

func TestSnippetSvc_SyncExtensionSeeds(t *testing.T) {
	convey.Convey("SyncExtensionSeeds 幂等同步", t, func() {
		convey.Convey("新建 + upsert + 清理 missing", func() {
			f := setupSvcTest(t)
			// 先匹配 3 次 upsert
			f.snippets.EXPECT().UpsertExtensionSeed(gomock.Any(), gomock.Any()).Return(nil).Times(3)
			// 再匹配一次 prune
			f.snippets.EXPECT().DeleteExtensionSeedsMissing(gomock.Any(),
				"ext:foo", gomock.InAnyOrder([]string{"k1", "k2", "k3"})).Return(nil)

			err := f.svc.SyncExtensionSeeds(context.Background(), "foo", []SeedDef{
				{Key: "k1", Name: "n1", Category: snippet_entity.CategoryShell, Content: "echo 1"},
				{Key: "k2", Name: "n2", Category: snippet_entity.CategoryShell, Content: "echo 2"},
				{Key: "k3", Name: "n3", Category: snippet_entity.CategoryShell, Content: "echo 3"},
			})
			assert.NoError(t, err)
		})

		convey.Convey("空 seed 列表也会触发 prune（清空）", func() {
			f := setupSvcTest(t)
			f.snippets.EXPECT().DeleteExtensionSeedsMissing(gomock.Any(), "ext:foo", gomock.Any()).Return(nil)
			assert.NoError(t, f.svc.SyncExtensionSeeds(context.Background(), "foo", nil))
		})

		convey.Convey("extName 为空拒绝", func() {
			f := setupSvcTest(t)
			assert.Error(t, f.svc.SyncExtensionSeeds(context.Background(), "", nil))
		})

		convey.Convey("seed 校验失败中断", func() {
			f := setupSvcTest(t)
			// 无期望的 repo 调用；seed.Key 为空直接拒绝
			err := f.svc.SyncExtensionSeeds(context.Background(), "foo", []SeedDef{
				{Key: "", Name: "n", Category: snippet_entity.CategoryShell, Content: "x"},
			})
			assert.Error(t, err)
		})
	})
}

func TestSnippetSvc_RemoveExtensionSeeds(t *testing.T) {
	f := setupSvcTest(t)
	f.snippets.EXPECT().HardDeleteBySource(gomock.Any(), "ext:foo").Return(nil)
	assert.NoError(t, f.svc.RemoveExtensionSeeds(context.Background(), "foo"))

	// extName 为空拒绝
	f2 := setupSvcTest(t)
	assert.Error(t, f2.svc.RemoveExtensionSeeds(context.Background(), ""))
}
