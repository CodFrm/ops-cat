package backup_svc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/model/entity/policy_group_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/credential_repo"
	"github.com/opskat/opskat/internal/repository/forward_repo"
	"github.com/opskat/opskat/internal/repository/group_repo"
	"github.com/opskat/opskat/internal/repository/policy_group_repo"
)

// Export 导出数据
func Export(ctx context.Context, opts *ExportOptions, crypto CredentialCrypto) (*BackupData, error) {
	allAssets, err := asset_repo.Asset().List(ctx, asset_repo.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("导出资产失败: %w", err)
	}
	allGroups, err := group_repo.Group().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("导出分组失败: %w", err)
	}

	// 构建查找映射
	assetMap := make(map[int64]*asset_entity.Asset, len(allAssets))
	for _, a := range allAssets {
		assetMap[a.ID] = a
	}
	groupMap := make(map[int64]*group_entity.Group, len(allGroups))
	for _, g := range allGroups {
		groupMap[g.ID] = g
	}

	// 确定要导出的资产
	var selectedAssets []*asset_entity.Asset
	if len(opts.AssetIDs) > 0 {
		selectedIDs := make(map[int64]bool)
		for _, id := range opts.AssetIDs {
			selectedIDs[id] = true
		}
		// 自动解析依赖
		resolveDependentAssets(selectedIDs, assetMap)
		for _, a := range allAssets {
			if selectedIDs[a.ID] {
				selectedAssets = append(selectedAssets, a)
			}
		}
	} else {
		selectedAssets = allAssets
	}

	// 收集所需的分组（含祖先链）
	neededGroupIDs := make(map[int64]bool)
	for _, a := range selectedAssets {
		if a.GroupID > 0 {
			collectAncestorGroups(a.GroupID, groupMap, neededGroupIDs)
		}
	}
	var selectedGroups []*group_entity.Group
	for _, g := range allGroups {
		if neededGroupIDs[g.ID] {
			selectedGroups = append(selectedGroups, g)
		}
	}
	// 如果导出全部资产，导出全部分组
	if len(opts.AssetIDs) == 0 {
		selectedGroups = allGroups
	}

	data := &BackupData{
		Version:    "1.0",
		ExportedAt: time.Now().Format(time.RFC3339),
		Assets:     selectedAssets,
		Groups:     selectedGroups,
	}
	if opts.Shortcuts != "" {
		data.Shortcuts = json.RawMessage(opts.Shortcuts)
	}
	if opts.CustomThemes != "" {
		data.CustomThemes = json.RawMessage(opts.CustomThemes)
	}

	// 收集选中资产 ID 集合（后续模块使用）
	selectedAssetIDs := make(map[int64]bool, len(selectedAssets))
	for _, a := range selectedAssets {
		selectedAssetIDs[a.ID] = true
	}

	// 收集策略组
	if opts.IncludePolicyGroups {
		pgIDs := collectPolicyGroupIDs(selectedAssets, selectedGroups)
		if len(pgIDs) > 0 {
			ids := make([]int64, 0, len(pgIDs))
			for id := range pgIDs {
				ids = append(ids, id)
			}
			pgs, err := policy_group_repo.PolicyGroup().ListByIDs(ctx, ids)
			if err != nil {
				return nil, fmt.Errorf("导出策略组失败: %w", err)
			}
			data.PolicyGroups = pgs
		}
	}

	// 凭据处理
	if opts.IncludeCredentials && crypto != nil {
		data.IncludesCredentials = true
		creds, err := exportCredentials(ctx, selectedAssets, crypto)
		if err != nil {
			return nil, fmt.Errorf("导出凭据失败: %w", err)
		}
		data.Credentials = creds
		// 解密资产内联密码
		if err := decryptAssetPasswords(data.Assets, crypto); err != nil {
			return nil, fmt.Errorf("解密资产密码失败: %w", err)
		}
	} else {
		// 不含凭据：清除敏感字段
		stripAssetSecrets(data.Assets)
	}

	// 端口转发
	if opts.IncludeForwards {
		forwards, err := exportForwards(ctx, selectedAssetIDs)
		if err != nil {
			return nil, fmt.Errorf("导出端口转发失败: %w", err)
		}
		data.Forwards = forwards
	}

	return data, nil
}

// --- 导出辅助函数 ---

// resolveDependentAssets 递归补全跳板机和 SSH 隧道资产
func resolveDependentAssets(selectedIDs map[int64]bool, assetMap map[int64]*asset_entity.Asset) {
	changed := true
	for changed {
		changed = false
		for id := range selectedIDs {
			a, ok := assetMap[id]
			if !ok {
				continue
			}
			switch {
			case a.IsSSH() && a.Config != "":
				cfg, err := a.GetSSHConfig()
				if err == nil && cfg.JumpHostID > 0 && !selectedIDs[cfg.JumpHostID] {
					selectedIDs[cfg.JumpHostID] = true
					changed = true
				}
			case a.IsDatabase() && a.Config != "":
				cfg, err := a.GetDatabaseConfig()
				if err == nil && cfg.SSHAssetID > 0 && !selectedIDs[cfg.SSHAssetID] {
					selectedIDs[cfg.SSHAssetID] = true
					changed = true
				}
			case a.IsRedis() && a.Config != "":
				cfg, err := a.GetRedisConfig()
				if err == nil && cfg.SSHAssetID > 0 && !selectedIDs[cfg.SSHAssetID] {
					selectedIDs[cfg.SSHAssetID] = true
					changed = true
				}
			}
		}
	}
}

// collectAncestorGroups 收集分组及其所有祖先
func collectAncestorGroups(groupID int64, groupMap map[int64]*group_entity.Group, result map[int64]bool) {
	for groupID > 0 && !result[groupID] {
		result[groupID] = true
		g, ok := groupMap[groupID]
		if !ok {
			break
		}
		groupID = g.ParentID
	}
}

// collectPolicyGroupIDs 从资产和分组中收集用户自定义策略组 ID（ID>0）
func collectPolicyGroupIDs(assets []*asset_entity.Asset, groups []*group_entity.Group) map[int64]bool {
	ids := make(map[int64]bool)
	for _, a := range assets {
		collectPolicyIDs(a.CmdPolicy, ids)
	}
	for _, g := range groups {
		collectPolicyIDs(g.CmdPolicy, ids)
		collectPolicyIDs(g.QryPolicy, ids)
		collectPolicyIDs(g.RdsPolicy, ids)
	}
	return ids
}

// collectPolicyIDs 从策略 JSON 中提取 Groups 字段的用户自定义组 ID
func collectPolicyIDs(policyJSON string, ids map[int64]bool) {
	if policyJSON == "" {
		return
	}
	var p struct {
		Groups []string `json:"groups"`
	}
	if err := json.Unmarshal([]byte(policyJSON), &p); err != nil {
		return
	}
	for _, id := range p.Groups {
		if !policy_group_entity.IsBuiltinID(id) {
			if dbID, err := strconv.ParseInt(id, 10, 64); err == nil {
				ids[dbID] = true
			}
		}
	}
}

// exportCredentials 导出关联凭据（解密为明文）
func exportCredentials(ctx context.Context, assets []*asset_entity.Asset, crypto CredentialCrypto) ([]*BackupCredential, error) {
	credIDs := make(map[int64]bool)
	for _, a := range assets {
		if a.IsSSH() && a.Config != "" {
			cfg, err := a.GetSSHConfig()
			if err == nil && cfg.CredentialID > 0 {
				credIDs[cfg.CredentialID] = true
			}
		}
	}
	if len(credIDs) == 0 {
		return nil, nil
	}

	var result []*BackupCredential
	for credID := range credIDs {
		cred, err := credential_repo.Credential().Find(ctx, credID)
		if err != nil {
			logger.Ctx(ctx).Warn("credential not found during export", zap.Int64("id", credID), zap.Error(err))
			continue
		}
		bc := &BackupCredential{Credential: *cred}
		if cred.Password != "" {
			plain, err := crypto.Decrypt(cred.Password)
			if err != nil {
				return nil, fmt.Errorf("解密凭据 %s 密码失败: %w", cred.Name, err)
			}
			bc.PlainPassword = plain
			bc.Password = "" // 清除密文
		}
		if cred.PrivateKey != "" {
			plain, err := crypto.Decrypt(cred.PrivateKey)
			if err != nil {
				return nil, fmt.Errorf("解密凭据 %s 私钥失败: %w", cred.Name, err)
			}
			bc.PlainPrivateKey = plain
			bc.PrivateKey = "" // 清除密文
		}
		result = append(result, bc)
	}
	return result, nil
}

// decryptAssetPasswords 解密资产 Config 中的内联密码为明文
func decryptAssetPasswords(assets []*asset_entity.Asset, crypto CredentialCrypto) error {
	for _, a := range assets {
		switch {
		case a.IsSSH() && a.Config != "":
			cfg, err := a.GetSSHConfig()
			if err != nil {
				continue
			}
			changed := false
			if cfg.Password != "" {
				plain, err := crypto.Decrypt(cfg.Password)
				if err != nil {
					return fmt.Errorf("解密资产 %s SSH 密码失败: %w", a.Name, err)
				}
				cfg.Password = plain
				changed = true
			}
			if cfg.Proxy != nil && cfg.Proxy.Password != "" {
				plain, err := crypto.Decrypt(cfg.Proxy.Password)
				if err != nil {
					return fmt.Errorf("解密资产 %s 代理密码失败: %w", a.Name, err)
				}
				cfg.Proxy.Password = plain
				changed = true
			}
			if changed {
				if err := a.SetSSHConfig(cfg); err != nil {
					return err
				}
			}
		case a.IsDatabase() && a.Config != "":
			cfg, err := a.GetDatabaseConfig()
			if err != nil {
				continue
			}
			if cfg.Password != "" {
				plain, err := crypto.Decrypt(cfg.Password)
				if err != nil {
					return fmt.Errorf("解密资产 %s 数据库密码失败: %w", a.Name, err)
				}
				cfg.Password = plain
				if err := a.SetDatabaseConfig(cfg); err != nil {
					return err
				}
			}
		case a.IsRedis() && a.Config != "":
			cfg, err := a.GetRedisConfig()
			if err != nil {
				continue
			}
			if cfg.Password != "" {
				plain, err := crypto.Decrypt(cfg.Password)
				if err != nil {
					return fmt.Errorf("解密资产 %s Redis 密码失败: %w", a.Name, err)
				}
				cfg.Password = plain
				if err := a.SetRedisConfig(cfg); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// stripAssetSecrets 清除资产配置中的敏感字段
func stripAssetSecrets(assets []*asset_entity.Asset) {
	for _, a := range assets {
		switch {
		case a.IsSSH() && a.Config != "":
			cfg, err := a.GetSSHConfig()
			if err != nil {
				continue
			}
			cfg.Password = ""
			cfg.CredentialID = 0
			cfg.PrivateKeys = nil
			if cfg.Proxy != nil {
				cfg.Proxy.Password = ""
			}
			if err := a.SetSSHConfig(cfg); err != nil {
				logger.Default().Warn("strip ssh secrets", zap.Error(err))
			}
		case a.IsDatabase() && a.Config != "":
			cfg, err := a.GetDatabaseConfig()
			if err != nil {
				continue
			}
			cfg.Password = ""
			if err := a.SetDatabaseConfig(cfg); err != nil {
				logger.Default().Warn("strip db secrets", zap.Error(err))
			}
		case a.IsRedis() && a.Config != "":
			cfg, err := a.GetRedisConfig()
			if err != nil {
				continue
			}
			cfg.Password = ""
			if err := a.SetRedisConfig(cfg); err != nil {
				logger.Default().Warn("strip redis secrets", zap.Error(err))
			}
		}
	}
}

// exportForwards 导出关联的端口转发配置
func exportForwards(ctx context.Context, assetIDs map[int64]bool) ([]*BackupForward, error) {
	configs, err := forward_repo.Forward().ListConfigs(ctx)
	if err != nil {
		return nil, err
	}
	var result []*BackupForward
	for _, config := range configs {
		if !assetIDs[config.AssetID] {
			continue
		}
		rules, err := forward_repo.Forward().ListRulesByConfigID(ctx, config.ID)
		if err != nil {
			return nil, fmt.Errorf("导出转发规则失败: %w", err)
		}
		result = append(result, &BackupForward{
			ForwardConfig: *config,
			Rules:         rules,
		})
	}
	return result, nil
}
