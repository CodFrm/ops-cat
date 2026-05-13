package group_svc

import (
	"context"
	"fmt"
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
	Reorder(ctx context.Context, id, targetParentID, beforeID int64) error
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

// Reorder 把 id 移动到 targetParentID 下 beforeID 之前。beforeID == 0 表示插到末尾。
// 校验：不能把分组拖进自己或自己的子孙（成环）。
// 跨父级时同步 ParentID。重排目标父级下所有兄弟的 sort_order，等间距 (10/20/30...)。
func (s *groupSvc) Reorder(ctx context.Context, id, targetParentID, beforeID int64) error {
	if id == targetParentID {
		return fmt.Errorf("不能把分组拖到自身下")
	}

	group, err := group_repo.Group().Find(ctx, id)
	if err != nil {
		return err
	}

	allGroups, err := group_repo.Group().List(ctx)
	if err != nil {
		return err
	}

	// 成环校验：从 targetParentID 沿 ParentID 链向上走，若经过 id 则成环。
	if targetParentID != 0 {
		parents := make(map[int64]int64, len(allGroups))
		for _, g := range allGroups {
			parents[g.ID] = g.ParentID
		}
		cursor := targetParentID
		for cursor != 0 {
			if cursor == id {
				return fmt.Errorf("不能把分组拖到自己的子孙下")
			}
			next, ok := parents[cursor]
			if !ok {
				break
			}
			cursor = next
		}
	}

	// 跨父级迁移
	if group.ParentID != targetParentID {
		if err := group_repo.Group().UpdateParentID(ctx, id, targetParentID); err != nil {
			return err
		}
		group.ParentID = targetParentID
	}

	var siblings []*group_entity.Group
	for _, g := range allGroups {
		if g.ID == id {
			continue
		}
		if g.ParentID == targetParentID {
			siblings = append(siblings, g)
		}
	}
	siblings = append(siblings, group)

	reordered := sortutil.ReorderSiblings(siblings, id, beforeID,
		func(item *group_entity.Group) int64 { return item.ID },
	)

	for i, item := range reordered {
		newOrder := (i + 1) * 10
		if item.SortOrder == newOrder && item.ID != id {
			continue
		}
		if err := group_repo.Group().UpdateSortOrder(ctx, item.ID, newOrder); err != nil {
			return err
		}
	}
	return nil
}
