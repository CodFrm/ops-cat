package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"ops-cat/internal/model/entity/asset_entity"
	"ops-cat/internal/repository/group_repo"
	"ops-cat/internal/service/asset_svc"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ToolHandlerFunc 统一工具处理函数
type ToolHandlerFunc func(ctx context.Context, args map[string]any) (string, error)

// ParamType 参数类型
type ParamType string

const (
	ParamString ParamType = "string"
	ParamNumber ParamType = "number"
)

// ParamDef 参数定义
type ParamDef struct {
	Name        string
	Type        ParamType
	Description string
	Required    bool
}

// ToolDef 统一工具定义
type ToolDef struct {
	Name        string
	Description string
	Params      []ParamDef
	Handler     ToolHandlerFunc
}

// AllToolDefs 返回所有工具定义
func AllToolDefs() []ToolDef {
	return []ToolDef{
		{
			Name:        "list_assets",
			Description: "List managed remote server assets. Returns an array of assets (with ID, name, type, group, etc.). This is typically the first step to discover asset IDs for other operations. Supports filtering by type and group.",
			Params: []ParamDef{
				{Name: "asset_type", Type: ParamString, Description: `Filter by asset type. Currently only "ssh" is supported. Omit to return all types.`},
				{Name: "group_id", Type: ParamNumber, Description: "Filter by group ID. Omit or set to 0 to list all groups."},
			},
			Handler: handleListAssets,
		},
		{
			Name:        "get_asset",
			Description: "Get detailed information about a specific asset, including its SSH connection configuration (host, port, username, auth method).",
			Params: []ParamDef{
				{Name: "id", Type: ParamNumber, Description: "Asset ID. Use list_assets to find available IDs.", Required: true},
			},
			Handler: handleGetAsset,
		},
		{
			Name:        "run_command",
			Description: "Execute a shell command on a remote server via SSH and return the output. Credentials are resolved automatically from the app's encrypted store — do not ask the user for passwords. IMPORTANT: The command runs on the REMOTE server, not locally.",
			Params: []ParamDef{
				{Name: "asset_id", Type: ParamNumber, Description: "Target server asset ID. Use list_assets to find available IDs.", Required: true},
				{Name: "command", Type: ParamString, Description: "Shell command to execute on the remote server.", Required: true},
			},
			Handler: handleRunCommand,
		},
		{
			Name:        "add_asset",
			Description: `Add a new asset to the inventory. Supports types: "ssh", "database", "redis". For database, specify driver ("mysql" or "postgresql").`,
			Params: []ParamDef{
				{Name: "name", Type: ParamString, Description: `Display name for the asset.`, Required: true},
				{Name: "type", Type: ParamString, Description: `Asset type: "ssh" (default), "database", or "redis".`},
				{Name: "host", Type: ParamString, Description: "Hostname or IP address.", Required: true},
				{Name: "port", Type: ParamNumber, Description: "Port number (default: 22 for SSH, 3306 for MySQL, 5432 for PostgreSQL, 6379 for Redis).", Required: true},
				{Name: "username", Type: ParamString, Description: "Login username.", Required: true},
				{Name: "auth_type", Type: ParamString, Description: `SSH auth method: "password" or "key". Only for SSH type.`},
				{Name: "driver", Type: ParamString, Description: `Database driver: "mysql" or "postgresql". Required for database type.`},
				{Name: "database", Type: ParamString, Description: "Default database name. For database type."},
				{Name: "read_only", Type: ParamString, Description: `Set to "true" to enable read-only mode. For database type.`},
				{Name: "ssh_asset_id", Type: ParamNumber, Description: "SSH asset ID for tunnel connection. For database/redis types."},
				{Name: "group_id", Type: ParamNumber, Description: "Group ID to assign this asset to."},
				{Name: "description", Type: ParamString, Description: "Optional description or notes."},
			},
			Handler: handleAddAsset,
		},
		{
			Name:        "update_asset",
			Description: "Update an existing asset's information. Only provide the fields you want to change; omitted fields remain unchanged.",
			Params: []ParamDef{
				{Name: "id", Type: ParamNumber, Description: "ID of the asset to update.", Required: true},
				{Name: "name", Type: ParamString, Description: "New display name."},
				{Name: "host", Type: ParamString, Description: "New hostname or IP."},
				{Name: "port", Type: ParamNumber, Description: "New SSH port."},
				{Name: "username", Type: ParamString, Description: "New SSH username."},
				{Name: "description", Type: ParamString, Description: "New description."},
				{Name: "group_id", Type: ParamNumber, Description: "New group ID."},
			},
			Handler: handleUpdateAsset,
		},
		{
			Name:        "list_groups",
			Description: "List all asset groups. Groups organize assets into a hierarchy via parent_id.",
			Handler:     handleListGroups,
		},
		{
			Name:        "upload_file",
			Description: "Upload a local file to a remote server via SFTP. Credentials are resolved automatically.",
			Params: []ParamDef{
				{Name: "asset_id", Type: ParamNumber, Description: "Target server asset ID.", Required: true},
				{Name: "local_path", Type: ParamString, Description: "Absolute path of the local file to upload.", Required: true},
				{Name: "remote_path", Type: ParamString, Description: "Destination path on the remote server (including filename).", Required: true},
			},
			Handler: handleUploadFile,
		},
		{
			Name:        "download_file",
			Description: "Download a file from a remote server to the local machine via SFTP. Credentials are resolved automatically.",
			Params: []ParamDef{
				{Name: "asset_id", Type: ParamNumber, Description: "Source server asset ID.", Required: true},
				{Name: "remote_path", Type: ParamString, Description: "Path of the file on the remote server.", Required: true},
				{Name: "local_path", Type: ParamString, Description: "Absolute local path to save the file (including filename).", Required: true},
			},
			Handler: handleDownloadFile,
		},
		{
			Name:        "exec_sql",
			Description: "Execute SQL on a database asset (MySQL, PostgreSQL). Returns rows as JSON for queries (SELECT/SHOW/DESCRIBE/EXPLAIN), or affected row count for statements (INSERT/UPDATE/DELETE). Credentials are resolved automatically.",
			Params: []ParamDef{
				{Name: "asset_id", Type: ParamNumber, Description: "Database asset ID. Use list_assets with asset_type='database' to find.", Required: true},
				{Name: "sql", Type: ParamString, Description: "SQL to execute.", Required: true},
				{Name: "database", Type: ParamString, Description: "Override the default database for this execution."},
			},
			Handler: handleExecSQL,
		},
		{
			Name:        "exec_redis",
			Description: "Execute a Redis command on a Redis asset. Returns the result as JSON. Credentials are resolved automatically.",
			Params: []ParamDef{
				{Name: "asset_id", Type: ParamNumber, Description: "Redis asset ID. Use list_assets with asset_type='redis' to find.", Required: true},
				{Name: "command", Type: ParamString, Description: "Redis command (e.g. 'GET mykey', 'HGETALL user:1', 'SET key value EX 3600').", Required: true},
			},
			Handler: handleExecRedis,
		},
	}
}

// --- 格式转换 ---

// ToOpenAITools 将工具定义转换为 OpenAI function calling 格式
func ToOpenAITools(defs []ToolDef) []Tool {
	tools := make([]Tool, len(defs))
	for i, def := range defs {
		properties := make(map[string]any)
		var required []string
		for _, p := range def.Params {
			properties[p.Name] = map[string]any{
				"type":        string(p.Type),
				"description": p.Description,
			}
			if p.Required {
				required = append(required, p.Name)
			}
		}
		params := map[string]any{
			"type":       "object",
			"properties": properties,
		}
		if len(required) > 0 {
			params["required"] = required
		}
		tools[i] = Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  params,
			},
		}
	}
	return tools
}

// --- SSH 客户端缓存（内置 Agent 同一次 Chat 中复用连接）---

type sshCacheKeyType struct{}

// SSHClientCache 缓存 SSH 连接
type SSHClientCache struct {
	clients map[int64]*ssh.Client
}

// NewSSHClientCache 创建 SSH 客户端缓存
func NewSSHClientCache() *SSHClientCache {
	return &SSHClientCache{clients: make(map[int64]*ssh.Client)}
}

// Close 关闭所有缓存的 SSH 连接
func (c *SSHClientCache) Close() error {
	for id, client := range c.clients {
		_ = client.Close()
		delete(c.clients, id)
	}
	return nil
}

func (c *SSHClientCache) getOrCreate(ctx context.Context, assetID int64, cfg *asset_entity.SSHConfig) (*ssh.Client, error) {
	if client, ok := c.clients[assetID]; ok {
		return client, nil
	}
	password, key, err := resolveAssetCredentials(ctx, cfg)
	if err != nil {
		return nil, err
	}
	client, err := createSSHClient(cfg, password, key)
	if err != nil {
		return nil, err
	}
	c.clients[assetID] = client
	return client, nil
}

func (c *SSHClientCache) remove(assetID int64) {
	if client, ok := c.clients[assetID]; ok {
		_ = client.Close()
		delete(c.clients, assetID)
	}
}

// WithSSHCache 将 SSH 缓存注入 context
func WithSSHCache(ctx context.Context, cache *SSHClientCache) context.Context {
	return context.WithValue(ctx, sshCacheKeyType{}, cache)
}

func getSSHCache(ctx context.Context) *SSHClientCache {
	if cache, ok := ctx.Value(sshCacheKeyType{}).(*SSHClientCache); ok {
		return cache
	}
	return nil
}

// --- 参数提取辅助函数 ---

func argString(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func argInt64(args map[string]any, key string) int64 {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int64(n)
		case int:
			return int64(n)
		case int64:
			return n
		case json.Number:
			i, _ := n.Int64()
			return i
		}
	}
	return 0
}

func argInt(args map[string]any, key string) int {
	return int(argInt64(args, key))
}

// --- 工具 handler 实现 ---

// safeAssetView 返回不含敏感信息的资产视图
type safeAssetView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	GroupID     int64  `json:"group_id"`
	Description string `json:"description,omitempty"`
	SortOrder   int    `json:"sort_order"`
	Createtime  int64  `json:"createtime"`
	Updatetime  int64  `json:"updatetime"`
	// 连接信息（不含密码/密钥）
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	AuthType string `json:"auth_type,omitempty"`
	// Database 专属
	Driver   string `json:"driver,omitempty"`
	Database string `json:"database,omitempty"`
	ReadOnly bool   `json:"read_only,omitempty"`
	// Redis 专属
	RedisDB int `json:"redis_db,omitempty"`
}

func toSafeView(a *asset_entity.Asset) safeAssetView {
	v := safeAssetView{
		ID:          a.ID,
		Name:        a.Name,
		Type:        a.Type,
		GroupID:     a.GroupID,
		Description: a.Description,
		SortOrder:   a.SortOrder,
		Createtime:  a.Createtime,
		Updatetime:  a.Updatetime,
	}
	switch a.Type {
	case asset_entity.AssetTypeSSH:
		if cfg, err := a.GetSSHConfig(); err == nil && cfg != nil {
			v.Host = cfg.Host
			v.Port = cfg.Port
			v.Username = cfg.Username
			v.AuthType = cfg.AuthType
		}
	case asset_entity.AssetTypeDatabase:
		if cfg, err := a.GetDatabaseConfig(); err == nil && cfg != nil {
			v.Host = cfg.Host
			v.Port = cfg.Port
			v.Username = cfg.Username
			v.Driver = string(cfg.Driver)
			v.Database = cfg.Database
			v.ReadOnly = cfg.ReadOnly
		}
	case asset_entity.AssetTypeRedis:
		if cfg, err := a.GetRedisConfig(); err == nil && cfg != nil {
			v.Host = cfg.Host
			v.Port = cfg.Port
			v.Username = cfg.Username
			v.RedisDB = cfg.Database
		}
	}
	return v
}

func handleListAssets(ctx context.Context, args map[string]any) (string, error) {
	assetType := argString(args, "asset_type")
	groupID := argInt64(args, "group_id")
	assets, err := asset_svc.Asset().List(ctx, assetType, groupID)
	if err != nil {
		return "", err
	}
	views := make([]safeAssetView, len(assets))
	for i, a := range assets {
		views[i] = toSafeView(a)
	}
	data, _ := json.Marshal(views)
	return string(data), nil
}

func handleGetAsset(ctx context.Context, args map[string]any) (string, error) {
	id := argInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("缺少参数 id")
	}
	asset, err := asset_svc.Asset().Get(ctx, id)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	data, _ := json.Marshal(toSafeView(asset))
	return string(data), nil
}

func handleRunCommand(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	command := argString(args, "command")
	if assetID == 0 {
		return "", fmt.Errorf("缺少参数 asset_id")
	}
	if command == "" {
		return "", fmt.Errorf("缺少参数 command")
	}

	// 权限检查（两条路径共用）
	if checker := GetPolicyChecker(ctx); checker != nil {
		result := checker.Check(ctx, assetID, command)
		if result.Decision != Allow {
			return result.Message, nil // 返回提示消息给 AI（非 error）
		}
	}

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsSSH() {
		return "", fmt.Errorf("资产不是SSH类型")
	}
	sshCfg, err := asset.GetSSHConfig()
	if err != nil {
		return "", fmt.Errorf("获取SSH配置失败: %w", err)
	}

	// 如果有 SSH 缓存（内置 Agent 模式），使用缓存连接
	if cache := getSSHCache(ctx); cache != nil {
		return runCommandWithCache(ctx, cache, assetID, sshCfg, command)
	}

	// 无缓存，创建一次性连接
	password, key, err := resolveAssetCredentials(ctx, sshCfg)
	if err != nil {
		return "", fmt.Errorf("解析凭据失败: %w", err)
	}
	return executeSSHCommand(sshCfg, password, key, command)
}

func runCommandWithCache(ctx context.Context, cache *SSHClientCache, assetID int64, cfg *asset_entity.SSHConfig, command string) (string, error) {
	client, err := cache.getOrCreate(ctx, assetID, cfg)
	if err != nil {
		return "", err
	}
	output, err := runSSHCommand(client, command)
	if err != nil {
		// 连接可能已断开，移除缓存后重试一次
		cache.remove(assetID)
		client, err = cache.getOrCreate(ctx, assetID, cfg)
		if err != nil {
			return "", err
		}
		output, err = runSSHCommand(client, command)
		if err != nil {
			cache.remove(assetID)
			return "", err
		}
	}
	return output, nil
}

func handleAddAsset(ctx context.Context, args map[string]any) (string, error) {
	name := argString(args, "name")
	host := argString(args, "host")
	port := argInt(args, "port")
	username := argString(args, "username")
	if name == "" || host == "" || port == 0 || username == "" {
		return "", fmt.Errorf("缺少必要参数 (name, host, port, username)")
	}

	assetType := argString(args, "type")
	if assetType == "" {
		assetType = asset_entity.AssetTypeSSH
	}
	groupID := argInt64(args, "group_id")
	description := argString(args, "description")

	asset := &asset_entity.Asset{
		Name:        name,
		Type:        assetType,
		GroupID:     groupID,
		Description: description,
	}

	switch assetType {
	case asset_entity.AssetTypeSSH:
		authType := argString(args, "auth_type")
		if authType == "" {
			authType = "password"
		}
		_ = asset.SetSSHConfig(&asset_entity.SSHConfig{
			Host:     host,
			Port:     port,
			Username: username,
			AuthType: authType,
		})
	case asset_entity.AssetTypeDatabase:
		driver := asset_entity.DatabaseDriver(argString(args, "driver"))
		if driver == "" {
			return "", fmt.Errorf("数据库类型必须指定 driver (mysql 或 postgresql)")
		}
		dbCfg := &asset_entity.DatabaseConfig{
			Driver:     driver,
			Host:       host,
			Port:       port,
			Username:   username,
			Database:   argString(args, "database"),
			ReadOnly:   argString(args, "read_only") == "true",
			SSHAssetID: argInt64(args, "ssh_asset_id"),
		}
		_ = asset.SetDatabaseConfig(dbCfg)
	case asset_entity.AssetTypeRedis:
		redisCfg := &asset_entity.RedisConfig{
			Host:       host,
			Port:       port,
			Username:   username,
			SSHAssetID: argInt64(args, "ssh_asset_id"),
		}
		_ = asset.SetRedisConfig(redisCfg)
	default:
		return "", fmt.Errorf("不支持的资产类型: %s", assetType)
	}

	if err := asset_svc.Asset().Create(ctx, asset); err != nil {
		return "", fmt.Errorf("创建资产失败: %w", err)
	}
	return fmt.Sprintf(`{"id":%d,"message":"资产创建成功"}`, asset.ID), nil
}

func handleUpdateAsset(ctx context.Context, args map[string]any) (string, error) {
	id := argInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("缺少参数 id")
	}

	asset, err := asset_svc.Asset().Get(ctx, id)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}

	if name := argString(args, "name"); name != "" {
		asset.Name = name
	}
	if desc := argString(args, "description"); desc != "" {
		asset.Description = desc
	}
	if _, ok := args["group_id"]; ok {
		asset.GroupID = argInt64(args, "group_id")
	}

	switch asset.Type {
	case asset_entity.AssetTypeSSH:
		sshCfg, _ := asset.GetSSHConfig()
		if sshCfg != nil {
			if host := argString(args, "host"); host != "" {
				sshCfg.Host = host
			}
			if port := argInt(args, "port"); port > 0 {
				sshCfg.Port = port
			}
			if username := argString(args, "username"); username != "" {
				sshCfg.Username = username
			}
			_ = asset.SetSSHConfig(sshCfg)
		}
	case asset_entity.AssetTypeDatabase:
		dbCfg, _ := asset.GetDatabaseConfig()
		if dbCfg != nil {
			if host := argString(args, "host"); host != "" {
				dbCfg.Host = host
			}
			if port := argInt(args, "port"); port > 0 {
				dbCfg.Port = port
			}
			if username := argString(args, "username"); username != "" {
				dbCfg.Username = username
			}
			if db := argString(args, "database"); db != "" {
				dbCfg.Database = db
			}
			_ = asset.SetDatabaseConfig(dbCfg)
		}
	case asset_entity.AssetTypeRedis:
		redisCfg, _ := asset.GetRedisConfig()
		if redisCfg != nil {
			if host := argString(args, "host"); host != "" {
				redisCfg.Host = host
			}
			if port := argInt(args, "port"); port > 0 {
				redisCfg.Port = port
			}
			if username := argString(args, "username"); username != "" {
				redisCfg.Username = username
			}
			_ = asset.SetRedisConfig(redisCfg)
		}
	}

	if err := asset_svc.Asset().Update(ctx, asset); err != nil {
		return "", fmt.Errorf("更新资产失败: %w", err)
	}
	return `{"message":"资产更新成功"}`, nil
}

func handleListGroups(ctx context.Context, _ map[string]any) (string, error) {
	groups, err := group_repo.Group().List(ctx)
	if err != nil {
		return "", fmt.Errorf("获取分组失败: %w", err)
	}
	data, _ := json.Marshal(groups)
	return string(data), nil
}

func handleUploadFile(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	localPath := argString(args, "local_path")
	remotePath := argString(args, "remote_path")
	if assetID == 0 || localPath == "" || remotePath == "" {
		return "", fmt.Errorf("缺少必要参数 (asset_id, local_path, remote_path)")
	}

	_, sshCfg, password, key, err := resolveAssetSSH(ctx, assetID)
	if err != nil {
		return "", err
	}

	err = executeWithSFTP(sshCfg, password, key, func(client *sftp.Client) error {
		srcFile, err := os.Open(localPath) //nolint:gosec
		if err != nil {
			return fmt.Errorf("打开本地文件失败: %w", err)
		}
		defer func() { _ = srcFile.Close() }()

		dstFile, err := client.Create(remotePath)
		if err != nil {
			return fmt.Errorf("创建远程文件失败: %w", err)
		}
		defer func() { _ = dstFile.Close() }()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"message":"文件上传成功","remote_path":"%s"}`, remotePath), nil
}

func handleDownloadFile(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	remotePath := argString(args, "remote_path")
	localPath := argString(args, "local_path")
	if assetID == 0 || remotePath == "" || localPath == "" {
		return "", fmt.Errorf("缺少必要参数 (asset_id, remote_path, local_path)")
	}

	_, sshCfg, password, key, err := resolveAssetSSH(ctx, assetID)
	if err != nil {
		return "", err
	}

	err = executeWithSFTP(sshCfg, password, key, func(client *sftp.Client) error {
		srcFile, err := client.Open(remotePath)
		if err != nil {
			return fmt.Errorf("打开远程文件失败: %w", err)
		}
		defer func() { _ = srcFile.Close() }()

		dstFile, err := os.Create(localPath) //nolint:gosec
		if err != nil {
			return fmt.Errorf("创建本地文件失败: %w", err)
		}
		defer func() { _ = dstFile.Close() }()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"message":"文件下载成功","local_path":"%s"}`, localPath), nil
}

// resolveAssetSSH 获取资产及其 SSH 凭据（共用辅助函数）
func resolveAssetSSH(ctx context.Context, assetID int64) (*asset_entity.Asset, *asset_entity.SSHConfig, string, string, error) {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsSSH() {
		return nil, nil, "", "", fmt.Errorf("资产不是SSH类型")
	}
	sshCfg, err := asset.GetSSHConfig()
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("获取SSH配置失败: %w", err)
	}
	password, key, err := resolveAssetCredentials(ctx, sshCfg)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("解析凭据失败: %w", err)
	}
	return asset, sshCfg, password, key, nil
}
