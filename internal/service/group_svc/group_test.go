package group_svc

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/asset_repo/mock_asset_repo"
	"github.com/opskat/opskat/internal/repository/group_repo"
	"github.com/opskat/opskat/internal/repository/group_repo/mock_group_repo"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func setupTest(t *testing.T) (context.Context, *mock_group_repo.MockGroupRepo, *mock_asset_repo.MockAssetRepo) {
	mockCtrl := gomock.NewController(t)
	t.Cleanup(func() { mockCtrl.Finish() })
	ctx := context.Background()
	mockGroupRepo := mock_group_repo.NewMockGroupRepo(mockCtrl)
	mockAssetRepo := mock_asset_repo.NewMockAssetRepo(mockCtrl)
	group_repo.RegisterGroup(mockGroupRepo)
	asset_repo.RegisterAsset(mockAssetRepo)
	return ctx, mockGroupRepo, mockAssetRepo
}

func TestGroupSvc_Create(t *testing.T) {
	ctx, mockGroupRepo, _ := setupTest(t)

	convey.Convey("创建分组", t, func() {
		convey.Convey("合法分组创建成功，设置时间戳", func() {
			group := &group_entity.Group{Name: "生产环境"}
			mockGroupRepo.EXPECT().Create(gomock.Any(), group).Return(nil)

			err := Group().Create(ctx, group)
			assert.NoError(t, err)
			assert.Greater(t, group.Createtime, int64(0))
			assert.Greater(t, group.Updatetime, int64(0))
		})

		convey.Convey("名称为空时 Validate 拦截，不调用 repo.Create", func() {
			group := &group_entity.Group{Name: ""}

			err := Group().Create(ctx, group)
			assert.Error(t, err)
		})
	})
}

func TestGroupSvc_Update(t *testing.T) {
	ctx, mockGroupRepo, _ := setupTest(t)

	convey.Convey("更新分组", t, func() {
		convey.Convey("合法更新成功，设置 updatetime", func() {
			group := &group_entity.Group{ID: 1, Name: "测试分组"}
			mockGroupRepo.EXPECT().Update(gomock.Any(), group).Return(nil)

			err := Group().Update(ctx, group)
			assert.NoError(t, err)
			assert.Greater(t, group.Updatetime, int64(0))
		})

		convey.Convey("名称为空时 Validate 拦截，不调用 repo.Update", func() {
			group := &group_entity.Group{ID: 1, Name: ""}

			err := Group().Update(ctx, group)
			assert.Error(t, err)
		})
	})
}

func TestGroupSvc_Delete(t *testing.T) {
	ctx, mockGroupRepo, mockAssetRepo := setupTest(t)

	convey.Convey("删除分组", t, func() {
		convey.Convey("deleteAssets=false 时，资产移到未分组（MoveToGroup）", func() {
			group := &group_entity.Group{ID: 10, ParentID: 0}
			mockGroupRepo.EXPECT().Find(gomock.Any(), int64(10)).Return(group, nil)
			mockGroupRepo.EXPECT().ReparentChildren(gomock.Any(), int64(10), int64(0)).Return(nil)
			mockAssetRepo.EXPECT().MoveToGroup(gomock.Any(), int64(10), int64(0)).Return(nil)
			mockGroupRepo.EXPECT().Delete(gomock.Any(), int64(10)).Return(nil)

			err := Group().Delete(ctx, 10, false)
			assert.NoError(t, err)
		})

		convey.Convey("deleteAssets=true 时，删除分组下资产（DeleteByGroupID）", func() {
			group := &group_entity.Group{ID: 20, ParentID: 5}
			mockGroupRepo.EXPECT().Find(gomock.Any(), int64(20)).Return(group, nil)
			mockGroupRepo.EXPECT().ReparentChildren(gomock.Any(), int64(20), int64(5)).Return(nil)
			mockAssetRepo.EXPECT().DeleteByGroupID(gomock.Any(), int64(20)).Return(nil)
			mockGroupRepo.EXPECT().Delete(gomock.Any(), int64(20)).Return(nil)

			err := Group().Delete(ctx, 20, true)
			assert.NoError(t, err)
		})
	})
}

func TestGroupSvc_Get(t *testing.T) {
	ctx, mockGroupRepo, _ := setupTest(t)

	convey.Convey("获取分组", t, func() {
		convey.Convey("委托给 repo.Find，返回对应分组", func() {
			expected := &group_entity.Group{ID: 1, Name: "运维组"}
			mockGroupRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(expected, nil)

			got, err := Group().Get(ctx, 1)
			assert.NoError(t, err)
			assert.Equal(t, expected.Name, got.Name)
		})
	})
}

func TestGroupSvc_List(t *testing.T) {
	ctx, mockGroupRepo, _ := setupTest(t)

	convey.Convey("列出分组", t, func() {
		convey.Convey("委托给 repo.List，返回分组列表", func() {
			expected := []*group_entity.Group{
				{ID: 1, Name: "生产环境"},
				{ID: 2, Name: "测试环境"},
			}
			mockGroupRepo.EXPECT().List(gomock.Any()).Return(expected, nil)

			got, err := Group().List(ctx)
			assert.NoError(t, err)
			assert.Len(t, got, 2)
		})
	})
}

func TestGroupSvc_Reorder(t *testing.T) {
	convey.Convey("Reorder：同父级排序", t, func() {
		ctx, mockGroupRepo, _ := setupTest(t)
		moving := &group_entity.Group{ID: 3, ParentID: 0, SortOrder: 30}
		all := []*group_entity.Group{
			{ID: 1, ParentID: 0, SortOrder: 10},
			{ID: 2, ParentID: 0, SortOrder: 20},
			moving,
		}
		mockGroupRepo.EXPECT().Find(gomock.Any(), int64(3)).Return(moving, nil)
		mockGroupRepo.EXPECT().List(gomock.Any()).Return(all, nil)
		// 把 3 拖到 1 之前 → [3, 1, 2]：3→10, 1→20, 2→30
		mockGroupRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(3), 10).Return(nil)
		mockGroupRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(1), 20).Return(nil)
		mockGroupRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(2), 30).Return(nil)

		err := Group().Reorder(ctx, 3, 0, 1)
		assert.NoError(t, err)
	})

	convey.Convey("Reorder：改父级", t, func() {
		ctx, mockGroupRepo, _ := setupTest(t)
		moving := &group_entity.Group{ID: 5, ParentID: 0, SortOrder: 10}
		all := []*group_entity.Group{
			moving,
			{ID: 10, ParentID: 0, SortOrder: 20},
			{ID: 11, ParentID: 10, SortOrder: 10},
		}
		mockGroupRepo.EXPECT().Find(gomock.Any(), int64(5)).Return(moving, nil)
		mockGroupRepo.EXPECT().List(gomock.Any()).Return(all, nil)
		// 拖到 ID=10 下，beforeID=0（末尾）
		mockGroupRepo.EXPECT().UpdateParentID(gomock.Any(), int64(5), int64(10)).Return(nil)
		// 目标父级 10 下兄弟（不含 5）：[11]；插入 5 在末尾 → [11, 5]
		// 11: sort_order 已经是 10，跳过；5: 写 20
		mockGroupRepo.EXPECT().UpdateSortOrder(gomock.Any(), int64(5), 20).Return(nil)

		err := Group().Reorder(ctx, 5, 10, 0)
		assert.NoError(t, err)
	})

	convey.Convey("Reorder：拖到自身下被拒", t, func() {
		ctx, _, _ := setupTest(t)
		err := Group().Reorder(ctx, 7, 7, 0)
		assert.Error(t, err)
	})

	convey.Convey("Reorder：拖到自己子孙下成环被拒", t, func() {
		ctx, mockGroupRepo, _ := setupTest(t)
		moving := &group_entity.Group{ID: 1, ParentID: 0}
		all := []*group_entity.Group{
			moving,
			{ID: 2, ParentID: 1}, // 2 是 1 的子
			{ID: 3, ParentID: 2}, // 3 是 1 的孙
		}
		mockGroupRepo.EXPECT().Find(gomock.Any(), int64(1)).Return(moving, nil)
		mockGroupRepo.EXPECT().List(gomock.Any()).Return(all, nil)
		// 尝试把 1 拖到 3 下 → 应被拒绝
		err := Group().Reorder(ctx, 1, 3, 0)
		assert.Error(t, err)
	})
}
