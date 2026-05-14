package ai

import (
	"context"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/tool"
)

// assetTools 资产 + 分组的 8 个工具。
// Description/Schema 是给模型看的契约，改字段时同步更新前端/文档。
func assetTools() []tool.Tool {
	return []tool.Tool{
		&tool.RawTool{
			NameStr: "list_assets",
			DescStr: "List managed remote server assets. Returns an array of assets (with ID, name, type, group, etc.). This is typically the first step to discover asset IDs for other operations. Supports filtering by type and group. Use get_asset to view asset description and connection details.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"asset_type": {Type: "string", Description: `Filter by asset type. Supported: "ssh", "database", "redis", "mongodb", "kafka", "k8s". Omit to return all types.`},
					"group_id":   {Type: "number", Description: "Filter by group ID. Omit or set to 0 to list all groups."},
				},
			},
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleListAssets(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "get_asset",
			DescStr: "Get detailed information about a specific asset, including SSH connection fields and asset-type-specific metadata. For k8s assets, inspect namespace, context, and ssh_tunnel_id to decide whether kubectl should run through an SSH jump host.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"id": {Type: "number", Description: "Asset ID. Use list_assets to find available IDs."},
				},
				Required: []string{"id"},
			},
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleGetAsset(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "add_asset",
			DescStr: `Add a new asset to the inventory. Supports types: "ssh", "database", "redis", "mongodb", "kafka", "k8s". For database, specify driver ("mysql" or "postgresql"). For k8s, specify kubeconfig. For kafka, specify brokers (comma-separated) plus optional sasl_mechanism / tls. Credentials (password / private_key) are stored encrypted; never echo them back to the user.`,
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"name":           {Type: "string", Description: "Display name for the asset."},
					"type":           {Type: "string", Description: `Asset type: "ssh" (default), "database", "redis", "mongodb", "kafka", or "k8s".`},
					"host":           {Type: "string", Description: "Hostname or IP address. Required except for k8s and kafka (kafka uses brokers, but host+port falls back to a single broker)."},
					"port":           {Type: "number", Description: "Port number (default: 22 for SSH, 3306 for MySQL, 5432 for PostgreSQL, 6379 for Redis, 27017 for MongoDB, 9092 for Kafka). Required except for k8s; for kafka, used only when brokers is omitted."},
					"username":       {Type: "string", Description: "Login username. Required for ssh/database/redis/mongodb; optional for kafka (only when SASL is configured)."},
					"password":       {Type: "string", Description: "Plaintext password. Stored encrypted by the app. For SSH password auth, database/redis/mongodb, or kafka SASL auth."},
					"auth_type":      {Type: "string", Description: `SSH auth method: "password" (default if password supplied) or "key" (default if private_key supplied). Only for SSH type.`},
					"private_key":    {Type: "string", Description: "SSH private key in PEM format. Imported into the credential store and linked to the asset. SSH only."},
					"passphrase":     {Type: "string", Description: "Passphrase for the SSH private key, if encrypted. SSH only."},
					"driver":         {Type: "string", Description: `Database driver: "mysql" or "postgresql". Required for database type.`},
					"database":       {Type: "string", Description: "Default database name. For database / mongodb type."},
					"read_only":      {Type: "string", Description: `Set to "true" to enable read-only mode. For database type.`},
					"redis_db":       {Type: "number", Description: "Default Redis DB index (0-15). Redis only."},
					"kubeconfig":     {Type: "string", Description: "Kubeconfig YAML content. Required for k8s type."},
					"namespace":      {Type: "string", Description: "Default Kubernetes namespace. K8s only."},
					"context":        {Type: "string", Description: "Kubeconfig context name to use. K8s only."},
					"brokers":        {Type: "string", Description: `Kafka broker list as comma/semicolon/newline separated "host:port" pairs (e.g. "kafka-0:9092,kafka-1:9092"). Kafka only; falls back to host+port when omitted.`},
					"client_id":      {Type: "string", Description: "Kafka client ID label. Kafka only; defaults to opskat."},
					"sasl_mechanism": {Type: "string", Description: `Kafka SASL mechanism: "none" (default), "plain", "scram-sha-256", "scram-sha-512". Kafka only.`},
					"tls":            {Type: "string", Description: `Set to "true" to enable TLS. Kafka only.`},
					"tls_insecure":   {Type: "string", Description: `Set to "true" to skip TLS certificate verification. Kafka only.`},
					"ssh_asset_id":   {Type: "number", Description: "SSH asset ID for tunnel connection. For database / redis / mongodb / kafka / k8s types."},
					"group_id":       {Type: "number", Description: "Group ID to assign this asset to."},
					"description":    {Type: "string", Description: "Optional description or notes."},
				},
				Required: []string{"name"},
			},
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleAddAsset(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "update_asset",
			DescStr: "Update an existing asset. Only provide the fields you want to change; omitted fields remain unchanged. Pass an empty string to clear description / database / icon. Use list_assets + get_asset first to confirm the current state.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"id":             {Type: "number", Description: "ID of the asset to update."},
					"name":           {Type: "string", Description: "New display name."},
					"host":           {Type: "string", Description: "New hostname or IP."},
					"port":           {Type: "number", Description: "New port."},
					"username":       {Type: "string", Description: "New username."},
					"password":       {Type: "string", Description: "New password (plaintext). Stored encrypted; switches the asset to inline password auth."},
					"auth_type":      {Type: "string", Description: `SSH auth method: "password" or "key". SSH only.`},
					"private_key":    {Type: "string", Description: "Replace SSH private key (PEM). Re-imports into credential store. SSH only."},
					"passphrase":     {Type: "string", Description: "Passphrase for new SSH private key, if encrypted. SSH only."},
					"driver":         {Type: "string", Description: `Database driver: "mysql" or "postgresql". Database only.`},
					"database":       {Type: "string", Description: "New default database. Database / mongodb only. Pass empty string to clear."},
					"read_only":      {Type: "string", Description: `Set to "true"/"false" to toggle read-only. Database only.`},
					"redis_db":       {Type: "number", Description: "New default Redis DB index. Redis only."},
					"brokers":        {Type: "string", Description: `New Kafka broker list (comma/semicolon/newline separated "host:port"). Kafka only.`},
					"client_id":      {Type: "string", Description: "New Kafka client ID. Kafka only."},
					"sasl_mechanism": {Type: "string", Description: `New Kafka SASL mechanism: "none", "plain", "scram-sha-256", "scram-sha-512". Kafka only.`},
					"tls":            {Type: "string", Description: `Set to "true"/"false" to toggle Kafka TLS. Kafka only.`},
					"tls_insecure":   {Type: "string", Description: `Set to "true"/"false" to toggle Kafka TLS skip-verify. Kafka only.`},
					"ssh_asset_id":   {Type: "number", Description: "New SSH tunnel asset ID. Pass 0 to detach. Database / redis / mongodb / kafka only."},
					"description":    {Type: "string", Description: "New description. Pass empty string to clear."},
					"group_id":       {Type: "number", Description: "New group ID (must be a positive integer from list_groups). Omit to keep current group; values <= 0 are ignored. To remove an asset from its group, ask the user to do it in the UI."},
					"icon":           {Type: "string", Description: "New icon name."},
				},
				Required: []string{"id"},
			},
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleUpdateAsset(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "list_groups",
			DescStr: "List all asset groups. Groups organize assets into a hierarchy via parent_id. Use get_group to view group description.",
			SchemaVal: agent.Schema{
				Type: "object",
			},
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleListGroups(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "get_group",
			DescStr: "Get detailed information about a specific asset group, including its description.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"id": {Type: "number", Description: "Group ID. Use list_groups to find available IDs."},
				},
				Required: []string{"id"},
			},
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleGetGroup(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "add_group",
			DescStr: "Create a new asset group. Groups can be nested via parent_id to form a hierarchy.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"name":        {Type: "string", Description: "Display name for the group."},
					"parent_id":   {Type: "number", Description: "Parent group ID for nesting. Omit or set to 0 for a top-level group."},
					"icon":        {Type: "string", Description: "Optional icon name."},
					"description": {Type: "string", Description: "Optional description."},
					"sort_order":  {Type: "number", Description: "Sort order within the parent group; lower comes first."},
				},
				Required: []string{"name"},
			},
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleAddGroup(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
		&tool.RawTool{
			NameStr: "update_group",
			DescStr: "Update an existing asset group. Only provide the fields you want to change; omitted fields remain unchanged. Pass empty string to clear icon / description.",
			SchemaVal: agent.Schema{
				Type: "object",
				Properties: map[string]*agent.Property{
					"id":          {Type: "number", Description: "ID of the group to update."},
					"name":        {Type: "string", Description: "New display name."},
					"parent_id":   {Type: "number", Description: "New parent group ID (must be a positive integer from list_groups). Omit to keep current parent; values <= 0 are ignored. To make a group top-level, ask the user to do it in the UI."},
					"icon":        {Type: "string", Description: "New icon name. Empty string clears."},
					"description": {Type: "string", Description: "New description. Empty string clears."},
					"sort_order":  {Type: "number", Description: "New sort order."},
				},
				Required: []string{"id"},
			},
			Handler: func(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
				out, err := handleUpdateGroup(ctx, in)
				if err != nil {
					return nil, err
				}
				return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: out}}}, nil
			},
		},
	}
}
