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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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
			Description: "Add a new SSH server to the asset inventory.",
			Params: []ParamDef{
				{Name: "name", Type: ParamString, Description: `Display name for the asset, e.g. "Production Web Server".`, Required: true},
				{Name: "host", Type: ParamString, Description: "Server hostname or IP address.", Required: true},
				{Name: "port", Type: ParamNumber, Description: "SSH port number, typically 22.", Required: true},
				{Name: "username", Type: ParamString, Description: "SSH login username.", Required: true},
				{Name: "auth_type", Type: ParamString, Description: `Authentication method: "password" or "key". Defaults to "password".`},
				{Name: "group_id", Type: ParamNumber, Description: "Group ID to assign this asset to. Use list_groups to find available groups. Omit for ungrouped."},
				{Name: "description", Type: ParamString, Description: "Optional description or notes for this asset."},
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

// RegisterToMCP 将工具定义注册到 MCP Server
func RegisterToMCP(s *server.MCPServer, defs []ToolDef) {
	for _, def := range defs {
		mcpTool := toMCPTool(def)
		handler := def.Handler
		s.AddTool(mcpTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args map[string]any
			if m, ok := req.Params.Arguments.(map[string]any); ok {
				args = m
			} else {
				args = make(map[string]any)
			}
			result, err := handler(ctx, args)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(result), nil
		})
	}
}

func toMCPTool(def ToolDef) mcp.Tool {
	opts := []mcp.ToolOption{
		mcp.WithDescription(def.Description),
	}
	for _, p := range def.Params {
		switch p.Type {
		case ParamString:
			if p.Required {
				opts = append(opts, mcp.WithString(p.Name, mcp.Required(), mcp.Description(p.Description)))
			} else {
				opts = append(opts, mcp.WithString(p.Name, mcp.Description(p.Description)))
			}
		case ParamNumber:
			if p.Required {
				opts = append(opts, mcp.WithNumber(p.Name, mcp.Required(), mcp.Description(p.Description)))
			} else {
				opts = append(opts, mcp.WithNumber(p.Name, mcp.Description(p.Description)))
			}
		}
	}
	return mcp.NewTool(def.Name, opts...)
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
		client.Close()
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
		client.Close()
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

func handleListAssets(ctx context.Context, args map[string]any) (string, error) {
	assetType := argString(args, "asset_type")
	groupID := argInt64(args, "group_id")
	assets, err := asset_svc.Asset().List(ctx, assetType, groupID)
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(assets)
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
	data, _ := json.Marshal(asset)
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

	// 无缓存（MCP 模式），创建一次性连接
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

	authType := argString(args, "auth_type")
	if authType == "" {
		authType = "password"
	}
	groupID := argInt64(args, "group_id")
	description := argString(args, "description")

	asset := &asset_entity.Asset{
		Name:        name,
		Type:        "ssh",
		GroupID:     groupID,
		Description: description,
	}
	_ = asset.SetSSHConfig(&asset_entity.SSHConfig{
		Host:     host,
		Port:     port,
		Username: username,
		AuthType: authType,
	})

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

	if asset.IsSSH() {
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
		srcFile, err := os.Open(localPath)
		if err != nil {
			return fmt.Errorf("打开本地文件失败: %w", err)
		}
		defer srcFile.Close()

		dstFile, err := client.Create(remotePath)
		if err != nil {
			return fmt.Errorf("创建远程文件失败: %w", err)
		}
		defer dstFile.Close()

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
		defer srcFile.Close()

		dstFile, err := os.Create(localPath)
		if err != nil {
			return fmt.Errorf("创建本地文件失败: %w", err)
		}
		defer dstFile.Close()

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
