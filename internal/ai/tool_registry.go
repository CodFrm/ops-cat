package ai

import (
	"context"
	"encoding/json"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

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

// CommandExtractorFunc 从工具参数中提取命令摘要（用于审计日志）
type CommandExtractorFunc func(args map[string]any) string

// ToolDef 统一工具定义
type ToolDef struct {
	Name             string
	Description      string
	Params           []ParamDef
	Handler          ToolHandlerFunc
	CommandExtractor CommandExtractorFunc // 可选，提取审计日志中的命令摘要
}

// AllToolDefs 返回所有工具定义
func AllToolDefs() []ToolDef {
	return []ToolDef{
		{
			Name:        "list_assets",
			Description: "List managed remote server assets. Returns an array of assets (with ID, name, type, group, etc.). This is typically the first step to discover asset IDs for other operations. Supports filtering by type and group. Use get_asset to view asset description and connection details.",
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
			Handler:          handleRunCommand,
			CommandExtractor: func(args map[string]any) string { return argString(args, "command") },
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
			Description: "List all asset groups. Groups organize assets into a hierarchy via parent_id. Use get_group to view group description.",
			Handler:     handleListGroups,
		},
		{
			Name:        "get_group",
			Description: "Get detailed information about a specific asset group, including its description.",
			Params: []ParamDef{
				{Name: "id", Type: ParamNumber, Description: "Group ID. Use list_groups to find available IDs.", Required: true},
			},
			Handler: handleGetGroup,
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
			CommandExtractor: func(args map[string]any) string {
				return "upload " + argString(args, "local_path") + " → " + argString(args, "remote_path")
			},
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
			CommandExtractor: func(args map[string]any) string {
				return "download " + argString(args, "remote_path") + " → " + argString(args, "local_path")
			},
		},
		{
			Name:        "exec_sql",
			Description: "Execute SQL on a database asset (MySQL, PostgreSQL). Returns rows as JSON for queries (SELECT/SHOW/DESCRIBE/EXPLAIN), or affected row count for statements (INSERT/UPDATE/DELETE). Credentials are resolved automatically.",
			Params: []ParamDef{
				{Name: "asset_id", Type: ParamNumber, Description: "Database asset ID. Use list_assets with asset_type='database' to find.", Required: true},
				{Name: "sql", Type: ParamString, Description: "SQL to execute.", Required: true},
				{Name: "database", Type: ParamString, Description: "Override the default database for this execution."},
			},
			Handler:          handleExecSQL,
			CommandExtractor: func(args map[string]any) string { return argString(args, "sql") },
		},
		{
			Name:        "exec_redis",
			Description: "Execute a Redis command on a Redis asset. Returns the result as JSON. Credentials are resolved automatically. IMPORTANT: Do NOT use the SELECT command to switch databases — it has no effect due to connection pooling. Use the 'db' parameter instead.",
			Params: []ParamDef{
				{Name: "asset_id", Type: ParamNumber, Description: "Redis asset ID. Use list_assets with asset_type='redis' to find.", Required: true},
				{Name: "command", Type: ParamString, Description: "Redis command (e.g. 'GET mykey', 'HGETALL user:1', 'SET key value EX 3600'). Do NOT use SELECT command here, use the 'db' parameter to switch databases.", Required: true},
				{Name: "db", Type: ParamNumber, Description: "Override the default Redis database number (0-15). Use this instead of the SELECT command."},
			},
			Handler:          handleExecRedis,
			CommandExtractor: func(args map[string]any) string { return argString(args, "command") },
		},
		{
			Name:        "request_permission",
			Description: "Request approval for a grant of command patterns BEFORE executing them. Submit command patterns (one per line, supports '*' wildcard) for a target asset. The user will review and may edit the patterns before approving. Once approved, subsequent run_command calls matching any approved pattern will be auto-approved. Call this proactively when you intend to run multiple commands on the same asset.",
			Params: []ParamDef{
				{Name: "asset_id", Type: ParamNumber, Description: "Target server asset ID.", Required: true},
				{Name: "command_patterns", Type: ParamString, Description: "Command patterns, one per line. Supports '*' wildcard (e.g. 'cat /var/log/*\\nsystemctl * nginx').", Required: true},
				{Name: "reason", Type: ParamString, Description: "Brief explanation of why these permissions are needed.", Required: true},
			},
			Handler: handleRequestGrant,
			CommandExtractor: func(args map[string]any) string {
				v := argString(args, "command_patterns")
				if reason := argString(args, "reason"); reason != "" {
					return "grant: " + v + " reason: " + reason
				}
				return "grant: " + v
			},
		},
		{
			Name:        "spawn_agent",
			Description: "Spawn a sub-agent to perform a complex task independently. The sub-agent has its own conversation context and session. Use this for: multi-step exploration, parallel investigation across assets, or tasks that require many tool calls. The sub-agent will request its own permissions as needed.",
			Params: []ParamDef{
				{Name: "role", Type: ParamString, Description: "Role description for the sub-agent (e.g., 'Server environment explorer')", Required: true},
				{Name: "task", Type: ParamString, Description: "Detailed task description for the sub-agent", Required: true},
				{Name: "tools", Type: ParamString, Description: "JSON array of tool names the sub-agent can use. Omit for all tools."},
			},
			Handler: handleSpawnAgent,
		},
		{
			Name:        "batch_command",
			Description: "Execute commands on multiple assets in parallel. Supports exec (SSH), sql (database), and redis command types. Commands are policy-checked and require user confirmation if needed. Results are returned per-asset.",
			Params: []ParamDef{
				{Name: "commands", Type: ParamString, Description: `JSON array of commands. Each item: {"asset": "name-or-id", "type": "exec|sql|redis", "command": "..."}. Type defaults to "exec".`, Required: true},
			},
			Handler: handleBatchCommand,
			CommandExtractor: func(args map[string]any) string {
				if cmds, ok := args["commands"]; ok {
					b, _ := json.Marshal(cmds)
					return string(b)
				}
				return ""
			},
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

// SSHClientCache 在同一次 AI Chat 中复用 SSH 连接
type SSHClientCache = ConnCache[*ssh.Client]

// NewSSHClientCache 创建 SSH 客户端缓存
func NewSSHClientCache() *SSHClientCache {
	return NewConnCache[*ssh.Client]("SSH")
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
			i, err := n.Int64()
			if err != nil {
				logger.Default().Warn("convert json.Number to int64", zap.String("value", n.String()), zap.Error(err))
			}
			return i
		}
	}
	return 0
}

func argInt(args map[string]any, key string) int {
	return int(argInt64(args, key))
}
