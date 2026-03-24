package import_svc

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/repository/group_repo"
	"github.com/opskat/opskat/internal/service/asset_svc"
)

// sshConfigHost 解析后的 SSH Config Host 块
type sshConfigHost struct {
	alias         string
	hostName      string
	port          int
	user          string
	identityFiles []string
	proxyJump     string
}

// PreviewSSHConfig 解析 SSH Config 文件，返回预览（不写数据库）
func PreviewSSHConfig(ctx context.Context, data []byte) (*PreviewResult, error) {
	hosts := parseSSHConfig(string(data))

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
	for idx, h := range hosts {
		port := h.port
		if port == 0 {
			port = 22
		}
		user := h.user
		if user == "" {
			user = "root"
		}
		name := h.alias
		if name == "" {
			name = fmt.Sprintf("%s@%s:%d", user, h.hostName, port)
		}

		authType := asset_entity.AuthTypePassword
		if len(h.identityFiles) > 0 {
			authType = asset_entity.AuthTypeKey
		}

		exists := existingSet[fmt.Sprintf("%s:%d:%s", h.hostName, port, user)]

		items = append(items, PreviewItem{
			Index:    idx,
			Name:     name,
			Host:     h.hostName,
			Port:     port,
			Username: user,
			AuthType: authType,
			Exists:   exists,
		})
	}

	return &PreviewResult{Items: items}, nil
}

// ImportSSHConfigSelected 导入用户选中的 SSH Config 连接
func ImportSSHConfigSelected(ctx context.Context, data []byte, selectedIndexes []int, opts ImportOptions) (*ImportResult, error) {
	hosts := parseSSHConfig(string(data))

	// 构建选中索引集合
	selectedSet := make(map[int]bool, len(selectedIndexes))
	for _, i := range selectedIndexes {
		selectedSet[i] = true
	}

	var toImport []sshConfigHost
	for i, h := range hosts {
		if selectedSet[i] {
			toImport = append(toImport, h)
		}
	}

	result := &ImportResult{Total: len(toImport)}
	if len(toImport) == 0 {
		return result, nil
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

	// 第一轮：创建或更新资产，记录 alias → assetID
	aliasToID := make(map[string]int64)
	type jumpPending struct {
		assetID   int64
		proxyJump string
	}
	var pendingJumps []jumpPending

	for _, h := range toImport {
		port := h.port
		if port == 0 {
			port = 22
		}
		user := h.user
		if user == "" {
			user = "root"
		}
		name := h.alias
		if name == "" {
			name = fmt.Sprintf("%s@%s:%d", user, h.hostName, port)
		}

		dupKey := fmt.Sprintf("%s:%d:%s", h.hostName, port, user)
		existingAsset := existingMap[dupKey]

		if existingAsset != nil && !opts.Overwrite {
			result.Skipped++
			continue
		}

		authType := asset_entity.AuthTypePassword
		var privateKeys []string
		if len(h.identityFiles) > 0 {
			authType = asset_entity.AuthTypeKey
			for _, f := range h.identityFiles {
				privateKeys = append(privateKeys, expandPath(f))
			}
		}

		sshCfg := &asset_entity.SSHConfig{
			Host:        h.hostName,
			Port:        port,
			Username:    user,
			AuthType:    authType,
			PrivateKeys: privateKeys,
		}
		// 使用 SSH Config 的 Host alias 作为分组依据时不分组，直接放根目录
		groupID := int64(0)
		_ = groupCache // 预留分组逻辑

		if existingAsset != nil && opts.Overwrite {
			// 覆盖模式：保留已有密码
			if oldCfg, err := existingAsset.GetSSHConfig(); err == nil && oldCfg.Password != "" {
				sshCfg.Password = oldCfg.Password
			}
			existingAsset.Name = name
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
			aliasToID[h.alias] = existingAsset.ID
			result.Success++
		} else {
			asset := &asset_entity.Asset{
				Name:    name,
				Type:    asset_entity.AssetTypeSSH,
				GroupID: groupID,
				Icon:    "server",
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
			aliasToID[h.alias] = asset.ID
			result.Success++
		}

		if h.proxyJump != "" {
			pendingJumps = append(pendingJumps, jumpPending{assetID: aliasToID[h.alias], proxyJump: h.proxyJump})
		}
	}

	// 第二轮：回填 ProxyJump → JumpHostID
	for _, p := range pendingJumps {
		jumpAlias := p.proxyJump
		jumpAssetID, ok := aliasToID[jumpAlias]
		if !ok {
			// 尝试在已有资产中按名称查找
			jumpAssetID = findAssetIDByName(existingAssets, jumpAlias)
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
			logger.Default().Warn("update asset jump host after ssh config import", zap.Int64("assetID", asset.ID), zap.Error(err))
		}
	}

	return result, nil
}

// parseSSHConfig 解析 SSH Config 文件内容
func parseSSHConfig(content string) []sshConfigHost {
	var hosts []sshConfigHost
	var current *sshConfigHost

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析 key value（支持 = 和空格分隔）
		key, value := splitDirective(line)
		if key == "" {
			continue
		}

		switch strings.ToLower(key) {
		case "host":
			// 跳过通配符 pattern（如 * 或 *.example.com）
			if strings.Contains(value, "*") || strings.Contains(value, "?") {
				current = nil
				continue
			}
			// 多个 host alias 只取第一个
			alias := strings.Fields(value)[0]
			hosts = append(hosts, sshConfigHost{alias: alias})
			current = &hosts[len(hosts)-1]

		case "hostname":
			if current != nil {
				current.hostName = value
			}
		case "port":
			if current != nil {
				if p, err := strconv.Atoi(value); err == nil {
					current.port = p
				}
			}
		case "user":
			if current != nil {
				current.user = value
			}
		case "identityfile":
			if current != nil {
				current.identityFiles = append(current.identityFiles, value)
			}
		case "proxyjump":
			if current != nil {
				// ProxyJump 可能是 user@host:port 格式，也可能是别名
				// 取第一跳
				first := strings.Split(value, ",")[0]
				current.proxyJump = strings.TrimSpace(first)
			}
		}
	}

	// 过滤掉没有 HostName 的条目
	var result []sshConfigHost
	for _, h := range hosts {
		if h.hostName != "" {
			result = append(result, h)
		}
	}
	return result
}

// splitDirective 将 SSH Config 行拆分为 key 和 value
// 支持 "Key Value"、"Key=Value" 和 "Key = Value" 三种格式
func splitDirective(line string) (string, string) {
	// keyword 是第一个 token，以空白或 = 结束
	idx := strings.IndexAny(line, " \t=")
	if idx <= 0 {
		return "", ""
	}
	key := line[:idx]
	rest := line[idx:]
	// 跳过分隔符：可选空白 + 可选 = + 可选空白
	rest = strings.TrimLeft(rest, " \t")
	if len(rest) > 0 && rest[0] == '=' {
		rest = rest[1:]
	}
	rest = strings.TrimLeft(rest, " \t")
	if rest == "" {
		return "", ""
	}
	return key, rest
}

// expandPath 展开路径中的 ~ 为 home 目录
func expandPath(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// DetectSSHConfigPath 检测 SSH Config 默认路径
func DetectSSHConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	var path string
	switch runtime.GOOS {
	case "windows":
		path = filepath.Join(homeDir, ".ssh", "config")
	default:
		path = filepath.Join(homeDir, ".ssh", "config")
	}

	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}
