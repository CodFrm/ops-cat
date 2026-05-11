package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/asset_repo/mock_asset_repo"
	"github.com/opskat/opskat/internal/repository/group_repo"
	"go.uber.org/mock/gomock"
)

// stubGroupRepo 简易 group repo stub，返回预设的 group 链
type stubGroupRepo struct {
	group_repo.GroupRepo
	groups map[int64]*group_entity.Group
}

func (s *stubGroupRepo) Find(_ context.Context, id int64) (*group_entity.Group, error) {
	g, ok := s.groups[id]
	if !ok {
		return nil, fmt.Errorf("group not found: %d", id)
	}
	return g, nil
}

func (s *stubGroupRepo) List(_ context.Context) ([]*group_entity.Group, error) { return nil, nil }
func (s *stubGroupRepo) Create(_ context.Context, _ *group_entity.Group) error { return nil }
func (s *stubGroupRepo) Update(_ context.Context, _ *group_entity.Group) error { return nil }
func (s *stubGroupRepo) Delete(_ context.Context, _ int64) error               { return nil }
func (s *stubGroupRepo) ReparentChildren(_ context.Context, _, _ int64) error  { return nil }
func (s *stubGroupRepo) UpdateSortOrder(_ context.Context, _ int64, _ int) error {
	return nil
}

// setupPolicyTest 创建 mock asset repo + stub group repo，返回 cleanup 函数
func setupPolicyTest(t *testing.T) (context.Context, *mock_asset_repo.MockAssetRepo, *stubGroupRepo) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockAsset := mock_asset_repo.NewMockAssetRepo(ctrl)
	origAsset := asset_repo.Asset()
	asset_repo.RegisterAsset(mockAsset)

	stubGroup := &stubGroupRepo{groups: make(map[int64]*group_entity.Group)}
	origGroup := group_repo.Group()
	group_repo.RegisterGroup(stubGroup)

	t.Cleanup(func() {
		if origAsset != nil {
			asset_repo.RegisterAsset(origAsset)
		}
		if origGroup != nil {
			group_repo.RegisterGroup(origGroup)
		}
	})

	return context.Background(), mockAsset, stubGroup
}

// mustJSON 将结构体序列化为 JSON 字符串
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
