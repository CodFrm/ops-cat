package group_svc

import (
	"context"
	"time"

	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/pkg/sortutil"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/group_repo"
)

// GroupSvc 分组业务接口
type GroupSvc interface {
	Get(ctx context.Context, id int64) (*group_entity.Group, error)
	List(ctx context.Context) ([]*group_entity.Group, error)
	Create(ctx context.Context, group *group_entity.Group) error
	Update(ctx context.Context, group *group_entity.Group) error
	Delete(ctx context.Context, id int64, deleteAssets bool) error
	Move(ctx context.Context, id int64, direction string) error
}

type groupSvc struct{}

var defaultGroup = &groupSvc{}

// Group 获取 GroupSvc 实例
func Group() GroupSvc {
	return defaultGroup
}

func (s *groupSvc) Get(ctx context.Context, id int64) (*group_entity.Group, error) {
	return group_repo.Group().Find(ctx, id)
}

func (s *groupSvc) List(ctx context.Context) ([]*group_entity.Group, error) {
	return group_repo.Group().List(ctx)
}

func (s *groupSvc) Create(ctx context.Context, group *group_entity.Group) error {
	if err := group.Validate(); err != nil {
		return err
	}
	now := time.Now().Unix()
	group.Createtime = now
	group.Updatetime = now
	return group_repo.Group().Create(ctx, group)
}

func (s *groupSvc) Update(ctx context.Context, group *group_entity.Group) error {
	if err := group.Validate(); err != nil {
		return err
	}
	group.Updatetime = time.Now().Unix()
	return group_repo.Group().Update(ctx, group)
}

// Delete 删除分组
// deleteAssets: true 删除分组下的资产，false 移动到未分组
func (s *groupSvc) Delete(ctx context.Context, id int64, deleteAssets bool) error {
	// 获取分组信息，用于将子分组挂到父分组
	group, err := group_repo.Group().Find(ctx, id)
	if err != nil {
		return err
	}
	// 子分组挂到被删分组的父级
	if err := group_repo.Group().ReparentChildren(ctx, id, group.ParentID); err != nil {
		return err
	}
	// 处理分组下的资产
	if deleteAssets {
		if err := asset_repo.Asset().DeleteByGroupID(ctx, id); err != nil {
			return err
		}
	} else {
		if err := asset_repo.Asset().MoveToGroup(ctx, id, 0); err != nil {
			return err
		}
	}
	return group_repo.Group().Delete(ctx, id)
}

// Move 移动分组排序（up/down/top）
func (s *groupSvc) Move(ctx context.Context, id int64, direction string) error {
	group, err := group_repo.Group().Find(ctx, id)
	if err != nil {
		return err
	}
	allGroups, err := group_repo.Group().List(ctx)
	if err != nil {
		return err
	}
	var siblings []*group_entity.Group
	for _, g := range allGroups {
		if g.ParentID == group.ParentID {
			siblings = append(siblings, g)
		}
	}
	return sortutil.MoveItem(id, direction, siblings,
		func(item *group_entity.Group) int64 { return item.ID },
		func(item *group_entity.Group) int { return item.SortOrder },
		func(itemID int64, order int) error {
			return group_repo.Group().UpdateSortOrder(ctx, itemID, order)
		},
	)
}
