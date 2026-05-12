package ai

import (
	"context"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/tool"
)

// dataTools 数据类资产执行工具：SQL / Redis / Mongo / K8s。
// 全部 Serial：写入/查询都可能跨网络且会触发审批，串行执行保证日志顺序可读。
func dataTools() []tool.Tool {
	return []tool.Tool{
		&tool.RawTool{
			NameStr: "exec_sql",
			DescStr: "Execute SQL on a database asset (MySQL, PostgreSQL). Returns rows as JSON for queries (SELECT/SHOW/DESCRIBE/EXPLAIN), or affected row count for statements (INSERT/UPDATE/DELETE). Credentials are resolved automatically.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id": {Type: "number", Description: "Database asset ID. Use list_assets with asset_type='database' to find."},
					"sql":      {Type: "string", Description: "SQL to execute."},
					"database": {Type: "string", Description: "Override the default database for this execution."},
				},
				Required: []string{"asset_id", "sql"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleExecSQL(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "exec_redis",
			DescStr: "Execute a Redis command on a Redis asset. Returns the result as JSON. Credentials are resolved automatically. IMPORTANT: Do NOT use the SELECT command to switch databases — it has no effect due to connection pooling. Use the 'db' parameter instead.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id": {Type: "number", Description: "Redis asset ID. Use list_assets with asset_type='redis' to find."},
					"command":  {Type: "string", Description: "Redis command (e.g. 'GET mykey', 'HGETALL user:1', 'SET key value EX 3600'). Do NOT use SELECT command here, use the 'db' parameter to switch databases."},
					"db":       {Type: "number", Description: "Override the default Redis database number (0-15). Use this instead of the SELECT command."},
				},
				Required: []string{"asset_id", "command"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleExecRedis(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "exec_mongo",
			DescStr: "Execute a MongoDB operation on a MongoDB asset. Returns query results as JSON for read operations (find/findOne/aggregate/countDocuments) or an acknowledgement summary (matched/modified/deleted count, inserted IDs) for write operations. Credentials are resolved automatically.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id":   {Type: "number", Description: "MongoDB asset ID. Use list_assets with asset_type='mongodb' to find."},
					"operation":  {Type: "string", Description: "Operation: find, findOne, insertOne, insertMany, updateOne, updateMany, deleteOne, deleteMany, aggregate, countDocuments, listDatabases, listCollections."},
					"database":   {Type: "string", Description: "Database name. Required except operation=listDatabases."},
					"collection": {Type: "string", Description: "Collection name. Required except operation=listDatabases / listCollections."},
					"query":      {Type: "string", Description: "JSON payload whose shape depends on operation: filter for find/findOne/count/update*/delete*, document(s) for insert*, pipeline array for aggregate."},
				},
				Required: []string{"asset_id", "operation"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleExecMongo(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "exec_k8s",
			DescStr: "Execute a kubectl command against a k8s asset. The tool uses the asset's stored kubeconfig, automatically applies the asset's default context/namespace when not explicitly provided, and preserves policy checks, approval, grant matching, and audit logging. If the k8s asset has ssh_tunnel_id, the command runs on that SSH jump host; otherwise kubectl runs locally. Pass either a full kubectl command or just the kubectl subcommand. Do not pass --kubeconfig.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_id": {Type: "number", Description: "K8s asset ID. Use list_assets with asset_type='k8s' to find it."},
					"command":  {Type: "string", Description: "kubectl command or subcommand, for example 'get pods -A' or 'kubectl describe pod api-0'."},
				},
				Required: []string{"asset_id", "command"},
			},
			IsSerial: true,
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleExecK8s(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
	}
}
