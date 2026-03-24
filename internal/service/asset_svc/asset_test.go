package asset_svc

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/asset_repo/mock_asset_repo"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func setupTest(t *testing.T) (context.Context, *mock_asset_repo.MockAssetRepo) {
	mockCtrl := gomock.NewController(t)
	t.Cleanup(func() { mockCtrl.Finish() })
	ctx := context.Background()
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
