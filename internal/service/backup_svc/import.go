package backup_svc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cago-frame/cago/database/db"
	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
)

// Import 导入备份数据
func Import(ctx context.Context, data *BackupData, opts *ImportOptions, crypto CredentialCrypto) (*ImportResult, error) {
	result := &ImportResult{}
	isReplace := opts.Mode != "merge"

	err := db.Ctx(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 策略组
		pgIDMap := make(map[int64]int64)
		if opts.ImportPolicyGroups && len(data.PolicyGroups) > 0 {
			if isReplace {
				if err := tx.Exec("DELETE FROM policy_groups").Error; err != nil {
					return fmt.Errorf("清除策略组失败: %w", err)
				}
			}
			for _, pg := range data.PolicyGroups {
				oldID := pg.ID
				pg.ID = 0
				if err := tx.Create(pg).Error; err != nil {
					return fmt.Errorf("创建策略组 %s 失败: %w", pg.Name, err)
				}
				pgIDMap[oldID] = pg.ID
				result.PolicyGroupsImported++
			}
		}

		// 2. 凭据
		credIDMap := make(map[int64]int64)
		if opts.ImportCredentials && len(data.Credentials) > 0 && crypto != nil {
			if isReplace {
				if err := tx.Exec("DELETE FROM credentials").Error; err != nil {
					return fmt.Errorf("清除凭据失败: %w", err)
				}
			}
			for _, bc := range data.Credentials {
				oldID := bc.ID
				cred := bc.Credential
				cred.ID = 0
				// 重新加密
				if bc.PlainPassword != "" {
					encrypted, err := crypto.Encrypt(bc.PlainPassword)
					if err != nil {
						return fmt.Errorf("加密密码失败: %w", err)
					}
					cred.Password = encrypted
				}
				if bc.PlainPrivateKey != "" {
					encrypted, err := crypto.Encrypt(bc.PlainPrivateKey)
					if err != nil {
						return fmt.Errorf("加密私钥失败: %w", err)
					}
					cred.PrivateKey = encrypted
				}
				if err := tx.Create(&cred).Error; err != nil {
					return fmt.Errorf("创建凭据 %s 失败: %w", cred.Name, err)
				}
				credIDMap[oldID] = cred.ID
				result.CredentialsImported++
			}
		}

		// 3. 分组
		groupIDMap := make(map[int64]int64)
		if opts.ImportAssets && len(data.Groups) > 0 {
			if isReplace {
				if err := tx.Exec("DELETE FROM groups").Error; err != nil {
					return fmt.Errorf("清除分组失败: %w", err)
				}
			}
			sortedGroups := sortGroups(data.Groups)
			for _, g := range sortedGroups {
				oldID := g.ID
				g.ID = 0
				if g.ParentID > 0 {
					if newID, ok := groupIDMap[g.ParentID]; ok {
						g.ParentID = newID
					}
				}
				// 回填策略组引用
				remapGroupPolicyGroupIDs(g, pgIDMap)
				if err := tx.Create(g).Error; err != nil {
					return fmt.Errorf("创建分组 %s 失败: %w", g.Name, err)
				}
				groupIDMap[oldID] = g.ID
				result.GroupsImported++
			}
		}

		// 4. 资产
		assetIDMap := make(map[int64]int64)
		if opts.ImportAssets && len(data.Assets) > 0 {
			if isReplace {
				if err := tx.Exec("DELETE FROM assets").Error; err != nil {
					return fmt.Errorf("清除资产失败: %w", err)
				}
			}

			type deferredRef struct {
				newAssetID int64
				oldRefID   int64
				refType    string // "jump_host" | "ssh_tunnel"
			}
			var deferredRefs []deferredRef

			for _, a := range data.Assets {
				oldID := a.ID
				a.ID = 0
				if a.GroupID > 0 {
					if newID, ok := groupIDMap[a.GroupID]; ok {
						a.GroupID = newID
					}
				}
				// 回填策略组引用
				remapAssetPolicyGroupIDs(a, pgIDMap)
				// 处理 Config 中的引用
				var oldJumpHostID, oldSSHAssetID int64
				switch {
				case a.IsSSH() && a.Config != "":
					cfg, err := a.GetSSHConfig()
					if err == nil {
						if cfg.JumpHostID > 0 {
							oldJumpHostID = cfg.JumpHostID
							cfg.JumpHostID = 0
						}
						// 回填 CredentialID
						if cfg.CredentialID > 0 {
							if newID, ok := credIDMap[cfg.CredentialID]; ok {
								cfg.CredentialID = newID
							} else if !opts.ImportCredentials {
								cfg.CredentialID = 0
							}
						}
						// 重新加密内联密码
						if data.IncludesCredentials && cfg.Password != "" && crypto != nil {
							encrypted, encErr := crypto.Encrypt(cfg.Password)
							if encErr != nil {
								logger.Default().Warn("re-encrypt ssh password", zap.Error(encErr))
							} else {
								cfg.Password = encrypted
							}
						}
						// 代理密码
						if data.IncludesCredentials && cfg.Proxy != nil && cfg.Proxy.Password != "" && crypto != nil {
							encrypted, encErr := crypto.Encrypt(cfg.Proxy.Password)
							if encErr != nil {
								logger.Default().Warn("re-encrypt proxy password", zap.Error(encErr))
							} else {
								cfg.Proxy.Password = encrypted
							}
						}
						if err := a.SetSSHConfig(cfg); err != nil {
							logger.Default().Warn("set ssh config in import", zap.Error(err))
						}
					}
				case a.IsDatabase() && a.Config != "":
					cfg, err := a.GetDatabaseConfig()
					if err == nil {
						if cfg.SSHAssetID > 0 {
							oldSSHAssetID = cfg.SSHAssetID
							cfg.SSHAssetID = 0
						}
						if data.IncludesCredentials && cfg.Password != "" && crypto != nil {
							encrypted, encErr := crypto.Encrypt(cfg.Password)
							if encErr != nil {
								logger.Default().Warn("re-encrypt db password", zap.Error(encErr))
							} else {
								cfg.Password = encrypted
							}
						}
						if err := a.SetDatabaseConfig(cfg); err != nil {
							logger.Default().Warn("set db config in import", zap.Error(err))
						}
					}
				case a.IsRedis() && a.Config != "":
					cfg, err := a.GetRedisConfig()
					if err == nil {
						if cfg.SSHAssetID > 0 {
							oldSSHAssetID = cfg.SSHAssetID
							cfg.SSHAssetID = 0
						}
						if data.IncludesCredentials && cfg.Password != "" && crypto != nil {
							encrypted, encErr := crypto.Encrypt(cfg.Password)
							if encErr != nil {
								logger.Default().Warn("re-encrypt redis password", zap.Error(encErr))
							} else {
								cfg.Password = encrypted
							}
						}
						if err := a.SetRedisConfig(cfg); err != nil {
							logger.Default().Warn("set redis config in import", zap.Error(err))
						}
					}
				}

				if err := tx.Create(a).Error; err != nil {
					return fmt.Errorf("创建资产 %s 失败: %w", a.Name, err)
				}
				assetIDMap[oldID] = a.ID
				result.AssetsImported++

				if oldJumpHostID > 0 {
					deferredRefs = append(deferredRefs, deferredRef{a.ID, oldJumpHostID, "jump_host"})
				}
				if oldSSHAssetID > 0 {
					deferredRefs = append(deferredRefs, deferredRef{a.ID, oldSSHAssetID, "ssh_tunnel"})
				}
			}

			// 回填跳板机和 SSH 隧道引用
			for _, ref := range deferredRefs {
				newRefID, ok := assetIDMap[ref.oldRefID]
				if !ok {
					continue
				}
				var asset asset_entity.Asset
				if err := tx.Where("id = ?", ref.newAssetID).First(&asset).Error; err != nil {
					continue
				}
				switch ref.refType {
				case "jump_host":
					cfg, err := asset.GetSSHConfig()
					if err != nil {
						continue
					}
					cfg.JumpHostID = newRefID
					if err := asset.SetSSHConfig(cfg); err != nil {
						continue
					}
				case "ssh_tunnel":
					if asset.IsDatabase() {
						cfg, err := asset.GetDatabaseConfig()
						if err != nil {
							continue
						}
						cfg.SSHAssetID = newRefID
						if err := asset.SetDatabaseConfig(cfg); err != nil {
							continue
						}
					} else if asset.IsRedis() {
						cfg, err := asset.GetRedisConfig()
						if err != nil {
							continue
						}
						cfg.SSHAssetID = newRefID
						if err := asset.SetRedisConfig(cfg); err != nil {
							continue
						}
					}
				}
				if err := tx.Save(&asset).Error; err != nil {
					return fmt.Errorf("更新资产引用失败: %w", err)
				}
			}
		}

		// 5. 端口转发
		if opts.ImportForwards && len(data.Forwards) > 0 {
			if isReplace {
				if err := tx.Exec("DELETE FROM forward_rules").Error; err != nil {
					return fmt.Errorf("清除转发规则失败: %w", err)
				}
				if err := tx.Exec("DELETE FROM forward_configs").Error; err != nil {
					return fmt.Errorf("清除转发配置失败: %w", err)
				}
			}
			for _, bf := range data.Forwards {
				newAssetID, ok := assetIDMap[bf.AssetID]
				if !ok {
					// 合并模式下资产可能已存在，尝试按名字匹配
					continue
				}
				config := bf.ForwardConfig
				config.ID = 0
				config.AssetID = newAssetID
				if err := tx.Create(&config).Error; err != nil {
					return fmt.Errorf("创建转发配置 %s 失败: %w", config.Name, err)
				}
				for _, rule := range bf.Rules {
					rule.ID = 0
					rule.ConfigID = config.ID
					if err := tx.Create(rule).Error; err != nil {
						return fmt.Errorf("创建转发规则失败: %w", err)
					}
				}
				result.ForwardsImported++
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// 6. 客户端设置透传
	if opts.ImportShortcuts && len(data.Shortcuts) > 0 {
		result.Shortcuts = string(data.Shortcuts)
	}
	if opts.ImportThemes && len(data.CustomThemes) > 0 {
		result.CustomThemes = string(data.CustomThemes)
	}

	return result, nil
}

// --- 导入辅助函数 ---

// remapGroupPolicyGroupIDs 回填分组中策略的 Groups 引用
func remapGroupPolicyGroupIDs(g *group_entity.Group, pgIDMap map[int64]int64) {
	g.CmdPolicy = remapPolicyGroupRefs(g.CmdPolicy, pgIDMap)
	g.QryPolicy = remapPolicyGroupRefs(g.QryPolicy, pgIDMap)
	g.RdsPolicy = remapPolicyGroupRefs(g.RdsPolicy, pgIDMap)
}

// remapAssetPolicyGroupIDs 回填资产中策略的 Groups 引用
func remapAssetPolicyGroupIDs(a *asset_entity.Asset, pgIDMap map[int64]int64) {
	a.CmdPolicy = remapPolicyGroupRefs(a.CmdPolicy, pgIDMap)
}

// remapPolicyGroupRefs 替换策略 JSON 中的 groups ID 引用（字符串格式）
func remapPolicyGroupRefs(policyJSON string, pgIDMap map[int64]int64) string {
	if policyJSON == "" || len(pgIDMap) == 0 {
		return policyJSON
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(policyJSON), &raw); err != nil {
		return policyJSON
	}
	groupsRaw, ok := raw["groups"]
	if !ok {
		return policyJSON
	}
	var groups []string
	if err := json.Unmarshal(groupsRaw, &groups); err != nil {
		return policyJSON
	}
	changed := false
	for i, id := range groups {
		// 跳过内置组和扩展组
		if policy.IsBuiltinID(id) || policy.IsExtensionID(id) {
			continue
		}
		// 用户自定义组：尝试 remap
		if dbID, parseOK := policy.ParseUserID(id); parseOK {
			if newID, mapOK := pgIDMap[dbID]; mapOK {
				groups[i] = policy.FormatUserID(newID)
				changed = true
			}
		}
	}
	if !changed {
		return policyJSON
	}
	newGroupsRaw, err := json.Marshal(groups)
	if err != nil {
		return policyJSON
	}
	raw["groups"] = newGroupsRaw
	result, err := json.Marshal(raw)
	if err != nil {
		return policyJSON
	}
	return string(result)
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
