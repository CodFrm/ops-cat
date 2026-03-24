package import_svc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/repository/group_repo"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/credential_svc"
)

// ImportResult 导入结果
type ImportResult struct {
	Total   int           `json:"total"`
	Success int           `json:"success"`
	Skipped int           `json:"skipped"`
	Failed  int           `json:"failed"`
	Errors  []ImportError `json:"errors"`
}

// ImportError 单条导入错误
type ImportError struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// PreviewGroup 预览分组
type PreviewGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PreviewItem 预览条目
type PreviewItem struct {
	Index       int    `json:"index"` // 在原始列表中的索引
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	AuthType    string `json:"authType"`
	GroupID     string `json:"groupId"`     // Tabby 分组 UUID
	Exists      bool   `json:"exists"`      // 是否已存在
	HasPassword bool   `json:"hasPassword"` // vault 中是否有密码
}

// PreviewResult 预览结果
type PreviewResult struct {
	Groups   []PreviewGroup `json:"groups"`
	Items    []PreviewItem  `json:"items"`
	HasVault bool           `json:"hasVault"` // 是否有加密 vault
}

// ImportOptions 导入选项
type ImportOptions struct {
	Passphrase string `json:"passphrase"` // Tabby vault 密码
	Overwrite  bool   `json:"overwrite"`  // 覆盖已存在的资产
}

// tabbyConfig Tabby 配置文件顶层结构
type tabbyConfig struct {
	Profiles []tabbyProfile    `yaml:"profiles"`
	Groups   []tabbyGroup      `yaml:"groups"`
	Vault    *tabbyStoredVault `yaml:"vault"`
}

// tabbyGroup Tabby 分组定义
type tabbyGroup struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

// tabbyProfile Tabby profile 配置
type tabbyProfile struct {
	Type    string       `yaml:"type"`
	Name    string       `yaml:"name"`
	Icon    string       `yaml:"icon"`
	Color   string       `yaml:"color"`
	Group   string       `yaml:"group"`
	ID      string       `yaml:"id"`
	Weight  int          `yaml:"weight"`
	Options tabbyOptions `yaml:"options"`
}

// tabbyOptions Tabby SSH 选项
type tabbyOptions struct {
	Host           string               `yaml:"host"`
	Port           int                  `yaml:"port"`
	User           string               `yaml:"user"`
	Auth           string               `yaml:"auth"`
	PrivateKeys    []string             `yaml:"privateKeys"`
	ForwardedPorts []tabbyForwardedPort `yaml:"forwardedPorts"`
	SocksProxyHost string               `yaml:"socksProxyHost"`
	SocksProxyPort int                  `yaml:"socksProxyPort"`
	JumpHost       string               `yaml:"jumpHost"`
}

// tabbyForwardedPort Tabby 端口转发
type tabbyForwardedPort struct {
	Type       string `yaml:"type"`
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	TargetHost string `yaml:"targetAddress"`
	TargetPort int    `yaml:"targetPort"`
}

// PreviewTabbyConfig 解析 Tabby 配置，返回预览数据（不写数据库）
func PreviewTabbyConfig(ctx context.Context, data []byte) (*PreviewResult, error) {
	var cfg tabbyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析 Tabby 配置失败: %w", err)
	}

	// 检测 vault
	hasVault := cfg.Vault != nil && cfg.Vault.Contents != ""

	// 构建 Tabby groupID → name 映射
	tabbyGroupMap := make(map[string]string, len(cfg.Groups))
	var groups []PreviewGroup
	for _, g := range cfg.Groups {
		tabbyGroupMap[g.ID] = g.Name
		groups = append(groups, PreviewGroup(g))
	}

	// 加载已有资产用于重复检测
	existingAssets, err := asset_svc.Asset().List(ctx, asset_entity.AssetTypeSSH, 0)
	if err != nil {
		return nil, fmt.Errorf("查询已有资产失败: %w", err)
	}
	existingSet := make(map[string]bool, len(existingAssets))
	for _, a := range existingAssets {
		sshCfg, err := a.GetSSHConfig()
		if err != nil {
			continue
		}
		existingSet[fmt.Sprintf("%s:%d:%s", sshCfg.Host, sshCfg.Port, sshCfg.Username)] = true
	}

	var items []PreviewItem
	idx := 0
	for _, p := range cfg.Profiles {
		if p.Type != "ssh" {
			continue
		}
		host := p.Options.Host
		port := p.Options.Port
		username := p.Options.User
		if port == 0 {
			port = 22
		}
		if username == "" {
			username = "root"
		}
		name := p.Name
		if name == "" {
			name = fmt.Sprintf("%s@%s:%d", username, host, port)
		}

		exists := false
		if host != "" {
			exists = existingSet[fmt.Sprintf("%s:%d:%s", host, port, username)]
		}

		items = append(items, PreviewItem{
			Index:       idx,
			Name:        name,
			Host:        host,
			Port:        port,
			Username:    username,
			AuthType:    mapAuthType(p.Options.Auth),
			GroupID:     p.Group,
			Exists:      exists,
			HasPassword: hasVault, // vault 存在时所有 profile 可能有密码
		})
		idx++
	}

	return &PreviewResult{Groups: groups, Items: items, HasVault: hasVault}, nil
}

// ImportTabbySelected 导入用户选中的 Tabby 连接
func ImportTabbySelected(ctx context.Context, data []byte, selectedIndexes []int, opts ImportOptions) (*ImportResult, error) {
	var cfg tabbyConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析 Tabby 配置失败: %w", err)
	}

	// 解密 vault 获取密码映射
	var vaultPasswords map[string]string
	if opts.Passphrase != "" && cfg.Vault != nil && cfg.Vault.Contents != "" {
		vault, err := decryptTabbyVault(cfg.Vault, opts.Passphrase)
		if err != nil {
			return nil, fmt.Errorf("解密 Tabby vault 失败: %w", err)
		}
		vaultPasswords = buildVaultPasswordMap(vault)
	}

	// 筛选 SSH profiles
	var sshProfiles []tabbyProfile
	for _, p := range cfg.Profiles {
		if p.Type == "ssh" {
			sshProfiles = append(sshProfiles, p)
		}
	}

	// 构建选中索引集合
	selectedSet := make(map[int]bool, len(selectedIndexes))
	for _, i := range selectedIndexes {
		selectedSet[i] = true
	}

	// 筛选选中的 profiles
	var toImport []tabbyProfile
	for i, p := range sshProfiles {
		if selectedSet[i] {
			toImport = append(toImport, p)
		}
	}

	result := &ImportResult{Total: len(toImport)}
	if len(toImport) == 0 {
		return result, nil
	}

	// 构建 Tabby groupID → name 映射
	tabbyGroupMap := make(map[string]string, len(cfg.Groups))
	for _, g := range cfg.Groups {
		tabbyGroupMap[g.ID] = g.Name
	}

	// 加载已有资产用于重复检测和覆盖
	existingAssets, err := asset_svc.Asset().List(ctx, asset_entity.AssetTypeSSH, 0)
	if err != nil {
		return nil, fmt.Errorf("查询已有资产失败: %w", err)
	}
	existingMap := make(map[string]*asset_entity.Asset, len(existingAssets))
	for _, a := range existingAssets {
		sshCfg, err := a.GetSSHConfig()
		if err != nil {
			continue
		}
		existingMap[fmt.Sprintf("%s:%d:%s", sshCfg.Host, sshCfg.Port, sshCfg.Username)] = a
	}

	existingGroups, err := group_repo.Group().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询已有分组失败: %w", err)
	}
	groupCache := buildGroupCache(existingGroups)

	tabbyNameToID := make(map[string]int64, len(toImport))
	type jumpHostPending struct {
		assetID      int64
		jumpHostName string
	}
	var pendingJumpHosts []jumpHostPending

	for _, profile := range toImport {
		name := profile.Name
		host := profile.Options.Host
		port := profile.Options.Port
		username := profile.Options.User

		if port == 0 {
			port = 22
		}
		if username == "" {
			username = "root"
		}
		if name == "" {
			name = fmt.Sprintf("%s@%s:%d", username, host, port)
		}
		if host == "" {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{Name: name, Reason: "host 为空"})
			continue
		}

		dupKey := fmt.Sprintf("%s:%d:%s", host, port, username)
		existingAsset := existingMap[dupKey]

		if existingAsset != nil && !opts.Overwrite {
			result.Skipped++
			continue
		}

		groupID := int64(0)
		if profile.Group != "" {
			groupName := tabbyGroupMap[profile.Group]
			if groupName != "" {
				var err error
				groupID, err = ensureGroupByName(ctx, groupName, groupCache)
				if err != nil {
					result.Failed++
					result.Errors = append(result.Errors, ImportError{Name: name, Reason: fmt.Sprintf("创建分组失败: %v", err)})
					continue
				}
			}
		}

		authType := mapAuthType(profile.Options.Auth)
		var privateKeys []string
		for _, pk := range profile.Options.PrivateKeys {
			pk = strings.TrimPrefix(pk, "file://")
			if pk != "" {
				privateKeys = append(privateKeys, pk)
			}
		}

		var proxyCfg *asset_entity.ProxyConfig
		if profile.Options.SocksProxyHost != "" {
			proxyPort := profile.Options.SocksProxyPort
			if proxyPort == 0 {
				proxyPort = 1080
			}
			proxyCfg = &asset_entity.ProxyConfig{Type: "socks5", Host: profile.Options.SocksProxyHost, Port: proxyPort}
		}

		sshCfg := &asset_entity.SSHConfig{
			Host: host, Port: port, Username: username, AuthType: authType,
			PrivateKeys: privateKeys, Proxy: proxyCfg,
		}
		// 从 vault 中提取密码并加密存储
		if password, ok := vaultPasswords[profile.ID]; ok && password != "" {
			encrypted, err := encryptPassword(password)
			if err == nil {
				sshCfg.Password = encrypted
			}
		}

		if existingAsset != nil && opts.Overwrite {
			// 覆盖模式：保留已有密码（如果新数据没有密码）
			if sshCfg.Password == "" {
				if oldCfg, err := existingAsset.GetSSHConfig(); err == nil && oldCfg.Password != "" {
					sshCfg.Password = oldCfg.Password
				}
			}
			existingAsset.Name = name
			if groupID != 0 {
				existingAsset.GroupID = groupID
			}
			if err := existingAsset.SetSSHConfig(sshCfg); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, ImportError{Name: name, Reason: fmt.Sprintf("序列化配置失败: %v", err)})
				continue
			}
			if err := asset_svc.Asset().Update(ctx, existingAsset); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, ImportError{Name: name, Reason: fmt.Sprintf("更新资产失败: %v", err)})
				continue
			}
			tabbyNameToID[profile.Name] = existingAsset.ID
			result.Success++
		} else {
			// 新建资产
			asset := &asset_entity.Asset{
				Name: name, Type: asset_entity.AssetTypeSSH, GroupID: groupID,
				Icon: "server",
			}
			if err := asset.SetSSHConfig(sshCfg); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, ImportError{Name: name, Reason: fmt.Sprintf("序列化配置失败: %v", err)})
				continue
			}

			if err := asset_svc.Asset().Create(ctx, asset); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, ImportError{Name: name, Reason: fmt.Sprintf("创建资产失败: %v", err)})
				continue
			}

			existingMap[dupKey] = asset
			tabbyNameToID[profile.Name] = asset.ID
			result.Success++
		}

		if profile.Options.JumpHost != "" {
			pendingJumpHosts = append(pendingJumpHosts, jumpHostPending{assetID: tabbyNameToID[profile.Name], jumpHostName: profile.Options.JumpHost})
		}
	}

	// 回填 JumpHostID
	for _, p := range pendingJumpHosts {
		jumpAssetID, ok := tabbyNameToID[p.jumpHostName]
		if !ok {
			jumpAssetID = findAssetIDByName(existingAssets, p.jumpHostName)
		}
		if jumpAssetID == 0 {
			continue
		}
		asset, err := asset_svc.Asset().Get(ctx, p.assetID)
		if err != nil {
			continue
		}
		sshCfg, err := asset.GetSSHConfig()
		if err != nil {
			continue
		}
		sshCfg.JumpHostID = jumpAssetID
		if err := asset.SetSSHConfig(sshCfg); err != nil {
			continue
		}
		if err := asset_svc.Asset().Update(ctx, asset); err != nil {
			logger.Default().Warn("update asset jump host after tabby import", zap.Int64("assetID", asset.ID), zap.Error(err))
		}
	}

	return result, nil
}

// encryptPassword 使用 credential_svc 加密密码
func encryptPassword(password string) (string, error) {
	return credential_svc.Default().Encrypt(password)
}

func mapAuthType(tabbyAuth string) string {
	switch strings.ToLower(tabbyAuth) {
	case "publickey":
		return asset_entity.AuthTypeKey
	case "password":
		return asset_entity.AuthTypePassword
	default:
		return asset_entity.AuthTypePassword
	}
}

func groupCacheKey(parentID int64, name string) string {
	return fmt.Sprintf("%d/%s", parentID, name)
}

func buildGroupCache(groups []*group_entity.Group) map[string]int64 {
	cache := make(map[string]int64, len(groups))
	for _, g := range groups {
		cache[groupCacheKey(g.ParentID, g.Name)] = g.ID
	}
	return cache
}

func ensureGroupByName(ctx context.Context, name string, cache map[string]int64) (int64, error) {
	key := groupCacheKey(0, name)
	if id, ok := cache[key]; ok {
		return id, nil
	}
	now := time.Now().Unix()
	group := &group_entity.Group{Name: name, ParentID: 0, Icon: "folder", Createtime: now, Updatetime: now}
	if err := group_repo.Group().Create(ctx, group); err != nil {
		return 0, err
	}
	cache[key] = group.ID
	return group.ID, nil
}

func findAssetIDByName(assets []*asset_entity.Asset, name string) int64 {
	for _, a := range assets {
		if a.Name == name {
			return a.ID
		}
	}
	return 0
}
