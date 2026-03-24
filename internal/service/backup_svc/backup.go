package backup_svc

import (
	"context"
	"fmt"
	"time"

	"github.com/cago-frame/cago/database/db"
	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/group_repo"
)

// BackupData 备份数据结构
type BackupData struct {
	Version    string                `json:"version"`
	ExportedAt string                `json:"exported_at"`
	Assets     []*asset_entity.Asset `json:"assets"`
	Groups     []*group_entity.Group `json:"groups"`
}

// Export 导出所有数据
func Export(ctx context.Context) (*BackupData, error) {
	assets, err := asset_repo.Asset().List(ctx, asset_repo.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("导出资产失败: %w", err)
	}
	groups, err := group_repo.Group().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("导出分组失败: %w", err)
	}
	return &BackupData{
		Version:    "1.0",
		ExportedAt: time.Now().Format(time.RFC3339),
		Assets:     assets,
		Groups:     groups,
	}, nil
}

// Import 导入备份数据（替换现有数据）
func Import(ctx context.Context, data *BackupData) error {
	return db.Ctx(ctx).Transaction(func(tx *gorm.DB) error {
		// 清除现有数据
		if err := tx.Exec("DELETE FROM assets").Error; err != nil {
			return fmt.Errorf("清除资产失败: %w", err)
		}
		if err := tx.Exec("DELETE FROM groups").Error; err != nil {
			return fmt.Errorf("清除分组失败: %w", err)
		}

		// 拓扑排序分组，确保父分组先创建
		sortedGroups := sortGroups(data.Groups)

		// 创建分组，建立 ID 映射
		groupIDMap := make(map[int64]int64)
		for _, g := range sortedGroups {
			oldID := g.ID
			g.ID = 0
			if g.ParentID > 0 {
				if newID, ok := groupIDMap[g.ParentID]; ok {
					g.ParentID = newID
				}
			}
			if err := tx.Create(g).Error; err != nil {
				return fmt.Errorf("创建分组 %s 失败: %w", g.Name, err)
			}
			groupIDMap[oldID] = g.ID
		}

		// 创建资产，记录跳板机引用
		assetIDMap := make(map[int64]int64)
		type jumpHostRef struct {
			newAssetID    int64
			oldJumpHostID int64
		}
		var jumpHostRefs []jumpHostRef

		for _, a := range data.Assets {
			oldID := a.ID
			a.ID = 0
			if a.GroupID > 0 {
				if newID, ok := groupIDMap[a.GroupID]; ok {
					a.GroupID = newID
				}
			}

			// 临时清除跳板机引用，后续回填
			var oldJumpHostID int64
			if a.IsSSH() && a.Config != "" {
				cfg, err := a.GetSSHConfig()
				if err == nil && cfg.JumpHostID > 0 {
					oldJumpHostID = cfg.JumpHostID
					cfg.JumpHostID = 0
					if err := a.SetSSHConfig(cfg); err != nil {
						logger.Default().Warn("clear jump host in backup restore", zap.Error(err))
					}
				}
			}

			if err := tx.Create(a).Error; err != nil {
				return fmt.Errorf("创建资产 %s 失败: %w", a.Name, err)
			}
			assetIDMap[oldID] = a.ID

			if oldJumpHostID > 0 {
				jumpHostRefs = append(jumpHostRefs, jumpHostRef{
					newAssetID:    a.ID,
					oldJumpHostID: oldJumpHostID,
				})
			}
		}

		// 回填跳板机引用
		for _, ref := range jumpHostRefs {
			newJumpHostID, ok := assetIDMap[ref.oldJumpHostID]
			if !ok {
				continue
			}
			var asset asset_entity.Asset
			if err := tx.Where("id = ?", ref.newAssetID).First(&asset).Error; err != nil {
				continue
			}
			cfg, err := asset.GetSSHConfig()
			if err != nil {
				continue
			}
			cfg.JumpHostID = newJumpHostID
			if err := asset.SetSSHConfig(cfg); err != nil {
				continue
			}
			if err := tx.Save(&asset).Error; err != nil {
				return fmt.Errorf("更新跳板机引用失败: %w", err)
			}
		}

		return nil
	})
}

// sortGroups 拓扑排序分组，确保父分组在子分组之前
func sortGroups(groups []*group_entity.Group) []*group_entity.Group {
	sorted := make([]*group_entity.Group, 0, len(groups))
	added := make(map[int64]bool)

	for len(sorted) < len(groups) {
		progress := false
		for _, g := range groups {
			if added[g.ID] {
				continue
			}
			if g.ParentID == 0 || added[g.ParentID] {
				sorted = append(sorted, g)
				added[g.ID] = true
				progress = true
			}
		}
		if !progress {
			for _, g := range groups {
				if !added[g.ID] {
					sorted = append(sorted, g)
				}
			}
			break
		}
	}
	return sorted
}
