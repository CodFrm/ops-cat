package asset_repo

import (
	"context"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"

	"github.com/cago-frame/cago/database/db"
)

// AssetRepo 资产数据访问接口
type AssetRepo interface {
	Find(ctx context.Context, id int64) (*asset_entity.Asset, error)
	List(ctx context.Context, opts ListOptions) ([]*asset_entity.Asset, error)
	Create(ctx context.Context, asset *asset_entity.Asset) error
	Update(ctx context.Context, asset *asset_entity.Asset) error
	Delete(ctx context.Context, id int64) error
	MoveToGroup(ctx context.Context, fromGroupID, toGroupID int64) error
	DeleteByGroupID(ctx context.Context, groupID int64) error
	FindByCredentialID(ctx context.Context, credentialID int64) ([]*asset_entity.Asset, error)
	UpdateSortOrder(ctx context.Context, id int64, sortOrder int) error
	UpdateGroupID(ctx context.Context, id, groupID int64) error
	CountByTypes(ctx context.Context, types []string) (int64, error)
}

// ListOptions 列表查询选项
type ListOptions struct {
	Type         string
	GroupID      int64
	ExactGroupID bool // 精确匹配 GroupID（包括 0），用于获取未分组资产
}

var defaultAsset AssetRepo

// Asset 获取AssetRepo实例
func Asset() AssetRepo {
	return defaultAsset
}

// RegisterAsset 注册AssetRepo实现
func RegisterAsset(i AssetRepo) {
	defaultAsset = i
}

// assetRepo 默认实现
type assetRepo struct{}

// NewAsset 创建默认实现
func NewAsset() AssetRepo {
	return &assetRepo{}
}

func (r *assetRepo) Find(ctx context.Context, id int64) (*asset_entity.Asset, error) {
	var asset asset_entity.Asset
	if err := db.Ctx(ctx).Where("id = ? AND status = ?", id, asset_entity.StatusActive).First(&asset).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func (r *assetRepo) List(ctx context.Context, opts ListOptions) ([]*asset_entity.Asset, error) {
	var assets []*asset_entity.Asset
	query := db.Ctx(ctx).Where("status = ?", asset_entity.StatusActive)
	if opts.Type != "" {
		query = query.Where("type = ?", opts.Type)
	}
	if opts.ExactGroupID {
		query = query.Where("group_id = ?", opts.GroupID)
	} else if opts.GroupID > 0 {
		query = query.Where("group_id = ?", opts.GroupID)
	}
	if err := query.Order("sort_order ASC, id ASC").Find(&assets).Error; err != nil {
		return nil, err
	}
	return assets, nil
}

func (r *assetRepo) Create(ctx context.Context, asset *asset_entity.Asset) error {
	return db.Ctx(ctx).Create(asset).Error
}

func (r *assetRepo) Update(ctx context.Context, asset *asset_entity.Asset) error {
	return db.Ctx(ctx).Save(asset).Error
}

func (r *assetRepo) Delete(ctx context.Context, id int64) error {
	return db.Ctx(ctx).Model(&asset_entity.Asset{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":         asset_entity.StatusDeleted,
			"config":         "", // 清除敏感配置（含加密密码/密钥）
			"command_policy": "",
		}).Error
}

func (r *assetRepo) MoveToGroup(ctx context.Context, fromGroupID, toGroupID int64) error {
	return db.Ctx(ctx).Model(&asset_entity.Asset{}).
		Where("group_id = ? AND status = ?", fromGroupID, asset_entity.StatusActive).
		Update("group_id", toGroupID).Error
}

func (r *assetRepo) DeleteByGroupID(ctx context.Context, groupID int64) error {
	return db.Ctx(ctx).Model(&asset_entity.Asset{}).
		Where("group_id = ? AND status = ?", groupID, asset_entity.StatusActive).
		Updates(map[string]interface{}{
			"status":         asset_entity.StatusDeleted,
			"config":         "",
			"command_policy": "",
		}).Error
}

func (r *assetRepo) UpdateSortOrder(ctx context.Context, id int64, sortOrder int) error {
	return db.Ctx(ctx).Model(&asset_entity.Asset{}).Where("id = ?", id).Update("sort_order", sortOrder).Error
}

func (r *assetRepo) UpdateGroupID(ctx context.Context, id, groupID int64) error {
	return db.Ctx(ctx).Model(&asset_entity.Asset{}).Where("id = ?", id).Update("group_id", groupID).Error
}

func (r *assetRepo) FindByCredentialID(ctx context.Context, credentialID int64) ([]*asset_entity.Asset, error) {
	var assets []*asset_entity.Asset
	if err := db.Ctx(ctx).Where("status = ? AND json_extract(config, '$.credential_id') = ?", asset_entity.StatusActive, credentialID).
		Find(&assets).Error; err != nil {
		return nil, err
	}
	return assets, nil
}

func (r *assetRepo) CountByTypes(ctx context.Context, types []string) (int64, error) {
	if len(types) == 0 {
		return 0, nil
	}
	var count int64
	err := db.Ctx(ctx).Model(&asset_entity.Asset{}).
		Where("type IN ? AND status = ?", types, asset_entity.StatusActive).
		Count(&count).Error
	return count, err
}
