package asset_svc

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy_group_entity"
	"github.com/opskat/opskat/internal/pkg/sortutil"
	"github.com/opskat/opskat/internal/repository/asset_repo"
)

// getExtensionDefaultPolicyJSON 获取扩展类型的默认策略 JSON
func getExtensionDefaultPolicyJSON(assetType string) string {
	extGroups := policy_group_entity.ExtensionGroups()
	var groupIDs []string
	for _, pg := range extGroups {
		if pg.PolicyType == assetType {
			groupIDs = append(groupIDs, pg.StringID)
		}
	}
	if len(groupIDs) == 0 {
		return ""
	}
	data, err := json.Marshal(map[string]any{"groups": groupIDs})
	if err != nil {
		return ""
	}
	return string(data)
}

// AssetSvc 资产业务接口
type AssetSvc interface {
	Get(ctx context.Context, id int64) (*asset_entity.Asset, error)
	List(ctx context.Context, assetType string, groupID int64) ([]*asset_entity.Asset, error)
	Create(ctx context.Context, asset *asset_entity.Asset) error
	Update(ctx context.Context, asset *asset_entity.Asset) error
	Delete(ctx context.Context, id int64) error
	Move(ctx context.Context, id int64, direction string) error
}

type assetSvc struct{}

var defaultAsset = &assetSvc{}

// Asset 获取AssetSvc实例
func Asset() AssetSvc {
	return defaultAsset
}

func (s *assetSvc) Get(ctx context.Context, id int64) (*asset_entity.Asset, error) {
	return asset_repo.Asset().Find(ctx, id)
}

func (s *assetSvc) List(ctx context.Context, assetType string, groupID int64) ([]*asset_entity.Asset, error) {
	return asset_repo.Asset().List(ctx, asset_repo.ListOptions{
		Type:    assetType,
		GroupID: groupID,
	})
}

func (s *assetSvc) Create(ctx context.Context, asset *asset_entity.Asset) error {
	if err := asset.Validate(); err != nil {
		return err
	}
	now := time.Now().Unix()
	asset.Createtime = now
	asset.Updatetime = now
	asset.Status = asset_entity.StatusActive
	// 未设置命令策略时，根据资产类型应用默认策略
	if asset.CmdPolicy == "" {
		switch asset.Type {
		case asset_entity.AssetTypeDatabase:
			if err := asset.SetQueryPolicy(asset_entity.DefaultQueryPolicy()); err != nil {
				logger.Default().Error("set default query policy", zap.Error(err))
			}
		case asset_entity.AssetTypeRedis:
			if err := asset.SetRedisPolicy(asset_entity.DefaultRedisPolicy()); err != nil {
				logger.Default().Error("set default redis policy", zap.Error(err))
			}
		case asset_entity.AssetTypeSSH:
			if err := asset.SetCommandPolicy(asset_entity.DefaultCommandPolicy()); err != nil {
				logger.Default().Error("set default command policy", zap.Error(err))
			}
		default:
			// 扩展类型：查找扩展默认权限组
			if defaultPolicy := getExtensionDefaultPolicyJSON(asset.Type); defaultPolicy != "" {
				asset.CmdPolicy = defaultPolicy
			}
		}
	}
	return asset_repo.Asset().Create(ctx, asset)
}

func (s *assetSvc) Update(ctx context.Context, asset *asset_entity.Asset) error {
	if err := asset.Validate(); err != nil {
		return err
	}
	asset.Updatetime = time.Now().Unix()
	return asset_repo.Asset().Update(ctx, asset)
}

func (s *assetSvc) Delete(ctx context.Context, id int64) error {
	return asset_repo.Asset().Delete(ctx, id)
}

// Move 移动资产排序（up/down/top）
func (s *assetSvc) Move(ctx context.Context, id int64, direction string) error {
	asset, err := asset_repo.Asset().Find(ctx, id)
	if err != nil {
		return err
	}
	siblings, err := asset_repo.Asset().List(ctx, asset_repo.ListOptions{GroupID: asset.GroupID, ExactGroupID: true})
	if err != nil {
		return err
	}
	return sortutil.MoveItem(id, direction, siblings,
		func(item *asset_entity.Asset) int64 { return item.ID },
		func(item *asset_entity.Asset) int { return item.SortOrder },
		func(itemID int64, order int) error {
			return asset_repo.Asset().UpdateSortOrder(ctx, itemID, order)
		},
	)
}
