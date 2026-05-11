package ai

import "github.com/cago-frame/agents/tool"

// dataCagoTools 数据类资产执行工具：SQL / Redis / Mongo / K8s。
// 全部 Serial：写入/查询都可能跨网络且会触发审批，串行执行保证日志顺序可读。
func dataCagoTools() []tool.Tool {
	return []tool.Tool{
		rawTool(
			"exec_sql",
			"Execute SQL on a database asset (MySQL, PostgreSQL). Returns rows as JSON for queries (SELECT/SHOW/DESCRIBE/EXPLAIN), or affected row count for statements (INSERT/UPDATE/DELETE). Credentials are resolved automatically.",
			makeSchema(
				paramSpec{name: "asset_id", typ: "number", required: true, desc: "Database asset ID. Use list_assets with asset_type='database' to find."},
				paramSpec{name: "sql", typ: "string", required: true, desc: "SQL to execute."},
				paramSpec{name: "database", typ: "string", desc: "Override the default database for this execution."},
			),
			true,
			handleExecSQL,
		),
		rawTool(
			"exec_redis",
			"Execute a Redis command on a Redis asset. Returns the result as JSON. Credentials are resolved automatically. IMPORTANT: Do NOT use the SELECT command to switch databases — it has no effect due to connection pooling. Use the 'db' parameter instead.",
			makeSchema(
				paramSpec{name: "asset_id", typ: "number", required: true, desc: "Redis asset ID. Use list_assets with asset_type='redis' to find."},
				paramSpec{name: "command", typ: "string", required: true, desc: "Redis command (e.g. 'GET mykey', 'HGETALL user:1', 'SET key value EX 3600'). Do NOT use SELECT command here, use the 'db' parameter to switch databases."},
				paramSpec{name: "db", typ: "number", desc: "Override the default Redis database number (0-15). Use this instead of the SELECT command."},
			),
			true,
			handleExecRedis,
		),
		rawTool(
			"exec_mongo",
			"Execute MongoDB operations on a MongoDB asset. Credentials are resolved automatically.",
			makeSchema(
				paramSpec{name: "asset_id", typ: "number", required: true, desc: "MongoDB asset ID. Use list_assets with asset_type='mongodb' to find."},
				paramSpec{name: "operation", typ: "string", required: true, desc: "Operation: find, findOne, insertOne, insertMany, updateOne, updateMany, deleteOne, deleteMany, aggregate, countDocuments"},
				paramSpec{name: "database", typ: "string", required: true, desc: "Database name"},
				paramSpec{name: "collection", typ: "string", required: true, desc: "Collection name"},
				paramSpec{name: "query", typ: "string", desc: "JSON for filter/document/pipeline, depends on operation"},
			),
			true,
			handleExecMongo,
		),
		rawTool(
			"exec_k8s",
			"Execute a kubectl command against a k8s asset. The tool uses the asset's stored kubeconfig, automatically applies the asset's default context/namespace when not explicitly provided, and preserves policy checks, approval, grant matching, and audit logging. If the k8s asset has ssh_tunnel_id, the command runs on that SSH jump host; otherwise kubectl runs locally. Pass either a full kubectl command or just the kubectl subcommand. Do not pass --kubeconfig.",
			makeSchema(
				paramSpec{name: "asset_id", typ: "number", required: true, desc: "K8s asset ID. Use list_assets with asset_type='k8s' to find it."},
				paramSpec{name: "command", typ: "string", required: true, desc: "kubectl command or subcommand, for example 'get pods -A' or 'kubectl describe pod api-0'."},
			),
			true,
			handleExecK8s,
		),
	}
}
