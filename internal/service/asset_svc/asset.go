package asset_svc

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/pkg/dbutil"
	"github.com/opskat/opskat/internal/pkg/sortutil"
	"github.com/opskat/opskat/internal/repository/asset_repo"
)

// AssetSvc 资产业务接口
type AssetSvc interface {
	Get(ctx context.Context, id int64) (*asset_entity.Asset, error)
	List(ctx context.Context, assetType string, groupID int64) ([]*asset_entity.Asset, error)
	Create(ctx context.Context, asset *asset_entity.Asset) error
	Update(ctx context.Context, asset *asset_entity.Asset) error
	Delete(ctx context.Context, id int64) error
	Move(ctx context.Context, id int64, direction string) error
	Reorder(ctx context.Context, id, targetGroupID, beforeID int64) error
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
	// 未设置命令策略时，根据资产类型应用默认拒绝列表
	if asset.CmdPolicy == "" {
		if p, ok := policy.GetDefaultPolicyOf(asset.Type); ok {
			data, err := json.Marshal(p)
			if err != nil {
				logger.Default().Error("marshal default policy", zap.Error(err))
			} else {
				asset.CmdPolicy = string(data)
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

// Reorder 把 id 移动到 targetGroupID 内 beforeID 之前。beforeID == 0 表示插到末尾。
// 跨分组时同步 GroupID。重排目标容器内所有兄弟的 sort_order，等间距 (10/20/30...)。
func (s *assetSvc) Reorder(ctx context.Context, id, targetGroupID, beforeID int64) error {
	return dbutil.WithTransaction(ctx, func(txCtx context.Context) error {
		asset, err := asset_repo.Asset().Find(txCtx, id)
		if err != nil {
			return err
		}

		// 跨分组迁移
		if asset.GroupID != targetGroupID {
			if err := asset_repo.Asset().UpdateGroupID(txCtx, id, targetGroupID); err != nil {
				return err
			}
			asset.GroupID = targetGroupID
		}

		siblings, err := asset_repo.Asset().List(txCtx, asset_repo.ListOptions{GroupID: targetGroupID, ExactGroupID: true})
		if err != nil {
			return err
		}

		reordered := sortutil.ReorderSiblings(siblings, id, beforeID,
			func(item *asset_entity.Asset) int64 { return item.ID },
		)

		for i, item := range reordered {
			newOrder := (i + 1) * 10
			if item.SortOrder == newOrder && item.ID != id {
				continue
			}
			if err := asset_repo.Asset().UpdateSortOrder(txCtx, item.ID, newOrder); err != nil {
				return err
			}
		}
		return nil
	})
}
