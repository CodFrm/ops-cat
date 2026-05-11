package ai

import (
	"context"
	"encoding/json"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"golang.org/x/crypto/ssh"
)

// tool_registry.go 的旧自研工具抽象（ParamDef / ToOpenAITools / DefaultToolExecutor）
// 已在 M6 cutover 删除——OpsKat 桌面端走 cago 原生 tool.Tool（见 cago_tools*.go）。
//
// 但 opsctl CLI 仍然直接以 (ctx, args)→(string, error) 的形式调用 handler，
// 因此保留下面三个最小抽象：
//   - ToolHandlerFunc：handler 通用签名；
//   - CommandExtractorFunc：审计模块从 args 抽命令摘要的签名（audit.go 用）；
//   - ToolDef + AllToolDefs：opsctl 的 name→handler 派发表（cmd/opsctl/command/handler.go 用）。
//
// 此外保留：
//   - SSH 客户端缓存（同一次 Send 内复用 ssh.Client）；
//   - 参数取值辅助（argString / argInt64 / argInt）。

// ToolHandlerFunc 工具处理函数：从 args map 执行操作并返回纯文本结果。
type ToolHandlerFunc func(ctx context.Context, args map[string]any) (string, error)

// CommandExtractorFunc 从工具参数中提取命令摘要（用于审计日志）。
type CommandExtractorFunc func(args map[string]any) string

// ToolDef opsctl 派发表条目。
// 不含 Description/Params/CommandExtractor 字段——那些只用于 OpenAI function calling schema，
// 已由 cago_tools*.go 内嵌的 makeSchema 处理。opsctl 只需要 (name, handler) 对。
type ToolDef struct {
	Name    string
	Handler ToolHandlerFunc
}

// AllToolDefs 返回 opsctl CLI 派发用的工具列表（与 cago_tools.go 的 CagoTools 一一对应）。
// spawn_agent / batch_command 已不再注册（opsctl 也没有调用它们）。
func AllToolDefs() []ToolDef {
	return []ToolDef{
		{"list_assets", handleListAssets},
		{"get_asset", handleGetAsset},
		{"add_asset", handleAddAsset},
		{"update_asset", handleUpdateAsset},
		{"list_groups", handleListGroups},
		{"get_group", handleGetGroup},
		{"add_group", handleAddGroup},
		{"update_group", handleUpdateGroup},
		{"run_command", handleRunCommand},
		{"upload_file", handleUploadFile},
		{"download_file", handleDownloadFile},
		{"exec_sql", handleExecSQL},
		{"exec_redis", handleExecRedis},
		{"exec_mongo", handleExecMongo},
		{"exec_k8s", handleExecK8s},
		{"kafka_cluster", handleKafkaCluster},
		{"kafka_topic", handleKafkaTopic},
		{"kafka_consumer_group", handleKafkaConsumerGroup},
		{"kafka_acl", handleKafkaACL},
		{"kafka_schema", handleKafkaSchema},
		{"kafka_connect", handleKafkaConnect},
		{"kafka_message", handleKafkaMessage},
		{"request_permission", handleRequestGrant},
		{"exec_tool", handleExecTool},
	}
}

// --- SSH 客户端缓存（cago 工具 handler 在同一次 Send 中复用连接）---

type sshCacheKeyType struct{}

// SSHClientCache 在同一次 AI Send 中复用 SSH 连接。
type SSHClientCache = ConnCache[*ssh.Client]

// NewSSHClientCache 创建 SSH 客户端缓存。
func NewSSHClientCache() *SSHClientCache {
	return NewConnCache[*ssh.Client]("SSH")
}

// WithSSHCache 将 SSH 缓存注入 context。
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
