package asset_svc

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/pkg/dbutil"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/asset_repo/mock_asset_repo"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func setupTest(t *testing.T) (context.Context, *mock_asset_repo.MockAssetRepo) {
	mockCtrl := gomock.NewController(t)
	t.Cleanup(func() { mockCtrl.Finish() })
	ctx := dbutil.WithTransactionRunner(context.Background(), func(ctx context.Context, fn func(context.Context) error) error {
		return fn(ctx)
	})
	mockRepo := mock_asset_repo.NewMockAssetRepo(mockCtrl)
	asset_repo.RegisterAsset(mockRepo)
	return ctx, mockRepo
}

func TestAssetSvc_Create(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("创建资产", t, func() {
		convey.Convey("创建合法SSH资产成功", func() {
			asset := &asset_entity.Asset{Name: "web-01", Type: asset_entity.AssetTypeSSH}
			_ = asset.SetSSHConfig(&asset_entity.SSHConfig{
				Host: "10.0.0.1", Port: 22, Username: "root", AuthType: asset_entity.AuthTypePassword,
			})

			mockRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

			err := Asset().Create(ctx, asset)
			assert.NoError(t, err)
			assert.Equal(t, asset_entity.StatusActive, asset.Status)
			assert.Greater(t, asset.Createtime, int64(0))
		})

		convey.Convey("创建无效资产失败（Validate拦截）", func() {
			asset := &asset_entity.Asset{Name: "", Type: asset_entity.AssetTypeSSH}

			// 不应调用repo.Create
			err := Asset().Create(ctx, asset)
			assert.Error(t, err)
		})
	})
}

func TestAssetSvc_Get(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("获取资产", t, func() {
		convey.Convey("存在的资产返回成功", func() {
			expected := &asset_entity.Asset{ID: 1, Name: "web-01", Type: asset_entity.AssetTypeSSH}
			mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(expected, nil)

			got, err := Asset().Get(ctx, 1)
			assert.NoError(t, err)
			assert.Equal(t, expected.Name, got.Name)
		})
	})
}

func TestAssetSvc_List(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("列出资产", t, func() {
		convey.Convey("按类型过滤", func() {
			expected := []*asset_entity.Asset{
				{ID: 1, Name: "web-01", Type: asset_entity.AssetTypeSSH},
			}
			mockRepo.EXPECT().List(gomock.Any(), asset_repo.ListOptions{
				Type: asset_entity.AssetTypeSSH,
			}).Return(expected, nil)

			got, err := Asset().List(ctx, asset_entity.AssetTypeSSH, 0)
			assert.NoError(t, err)
			assert.Len(t, got, 1)
		})
	})
}

func TestAssetSvc_Delete(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("删除资产", t, func() {
		convey.Convey("软删除成功", func() {
			mockRepo.EXPECT().Delete(gomock.Any(), int64(1)).Return(nil)

			err := Asset().Delete(ctx, 1)
			assert.NoError(t, err)
		})
	})
}

func TestAssetSvc_Reorder(t *testing.T) {
	convey.Convey("Reorder：同分组插入到 beforeID 之前", t, func() {
		ctx, mockRepo := setupTest(t)
		// 原始顺序：[1, 2, 3, 4]，把 4 拖到 2 之前 → [1, 4, 2, 3]
		moving := &asset_entity.Asset{ID: 4, GroupID: 10, SortOrder: 40}
		siblings := []*asset_entity.Asset{
			{ID: 1, GroupID: 10, SortOrder: 10},
			{ID: 2, GroupID: 10, SortOrder: 20},
			{ID: 3, GroupID: 10, SortOrder: 30},
			moving,
		}
		mockRepo.EXPECT().Find(gomock.Any(), int64(4)).Return(moving, nil)
		mockRepo.EXPECT().List(gomock.Any(), asset_repo.ListOptions{GroupID: 10, ExactGroupID: true}).Return(siblings, nil)
		// 新顺序 [1, 4, 2, 3] → sort_order = 10/20/30/40
		mockRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(4), 20).Return(nil)
		mockRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(2), 30).Return(nil)
		mockRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(3), 40).Return(nil)
		// id=1 已经是 10，跳过

		err := Asset().Reorder(ctx, 4, 10, 2)
		assert.NoError(t, err)
	})

	convey.Convey("Reorder：beforeID==0 追加到末尾", t, func() {
		ctx, mockRepo := setupTest(t)
		moving := &asset_entity.Asset{ID: 1, GroupID: 10, SortOrder: 10}
		siblings := []*asset_entity.Asset{
			moving,
			{ID: 2, GroupID: 10, SortOrder: 20},
			{ID: 3, GroupID: 10, SortOrder: 30},
		}
		mockRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(moving, nil)
		mockRepo.EXPECT().List(gomock.Any(), asset_repo.ListOptions{GroupID: 10, ExactGroupID: true}).Return(siblings, nil)
		// 新顺序 [2, 3, 1]：2→10, 3→20, 1→30
		mockRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(2), 10).Return(nil)
		mockRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(3), 20).Return(nil)
		mockRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(1), 30).Return(nil)

		err := Asset().Reorder(ctx, 1, 10, 0)
		assert.NoError(t, err)
	})

	convey.Convey("Reorder：跨分组迁移 + 重排", t, func() {
		ctx, mockRepo := setupTest(t)
		moving := &asset_entity.Asset{ID: 5, GroupID: 10, SortOrder: 10}
		// 切换到目标分组后 List 时移动的资产已 GroupID=20
		targetSiblings := []*asset_entity.Asset{
			{ID: 7, GroupID: 20, SortOrder: 10},
			{ID: 8, GroupID: 20, SortOrder: 20},
			{ID: 5, GroupID: 20, SortOrder: 10}, // 假设 DB 返回，未必精确
		}
		mockRepo.EXPECT().Find(gomock.Any(), int64(5)).Return(moving, nil)
		mockRepo.EXPECT().UpdateGroupID(gomock.Any(), int64(5), int64(20)).Return(nil)
		mockRepo.EXPECT().List(gomock.Any(), asset_repo.ListOptions{GroupID: 20, ExactGroupID: true}).Return(targetSiblings, nil)
		// 把 5 插到 8 之前 → [7, 5, 8]：7→10(skip), 5→20, 8→30
		mockRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(5), 20).Return(nil)
		mockRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(8), 30).Return(nil)

		err := Asset().Reorder(ctx, 5, 20, 8)
		assert.NoError(t, err)
	})
}
