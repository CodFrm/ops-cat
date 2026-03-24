package group_repo

import (
	"context"

	"github.com/opskat/opskat/internal/model/entity/group_entity"

	"github.com/cago-frame/cago/database/db"
)

// GroupRepo 分组数据访问接口
type GroupRepo interface {
	Find(ctx context.Context, id int64) (*group_entity.Group, error)
	List(ctx context.Context) ([]*group_entity.Group, error)
	Create(ctx context.Context, group *group_entity.Group) error
	Update(ctx context.Context, group *group_entity.Group) error
	Delete(ctx context.Context, id int64) error
	ReparentChildren(ctx context.Context, oldParentID, newParentID int64) error
	UpdateSortOrder(ctx context.Context, id int64, sortOrder int) error
}

var defaultGroup GroupRepo

// Group 获取GroupRepo实例
func Group() GroupRepo {
	return defaultGroup
}

// RegisterGroup 注册GroupRepo实现
func RegisterGroup(i GroupRepo) {
	defaultGroup = i
}

// groupRepo 默认实现
type groupRepo struct{}

// NewGroup 创建默认实现
func NewGroup() GroupRepo {
	return &groupRepo{}
}

func (r *groupRepo) Find(ctx context.Context, id int64) (*group_entity.Group, error) {
	var group group_entity.Group
	if err := db.Ctx(ctx).Where("id = ?", id).First(&group).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *groupRepo) List(ctx context.Context) ([]*group_entity.Group, error) {
	var groups []*group_entity.Group
	if err := db.Ctx(ctx).Order("sort_order ASC, id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *groupRepo) Create(ctx context.Context, group *group_entity.Group) error {
	return db.Ctx(ctx).Create(group).Error
}

func (r *groupRepo) Update(ctx context.Context, group *group_entity.Group) error {
	return db.Ctx(ctx).Save(group).Error
}

func (r *groupRepo) Delete(ctx context.Context, id int64) error {
	return db.Ctx(ctx).Where("id = ?", id).Delete(&group_entity.Group{}).Error
}

func (r *groupRepo) UpdateSortOrder(ctx context.Context, id int64, sortOrder int) error {
	return db.Ctx(ctx).Model(&group_entity.Group{}).Where("id = ?", id).Update("sort_order", sortOrder).Error
}

func (r *groupRepo) ReparentChildren(ctx context.Context, oldParentID, newParentID int64) error {
	return db.Ctx(ctx).Model(&group_entity.Group{}).
		Where("parent_id = ?", oldParentID).
		Update("parent_id", newParentID).Error
}
