package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/bootstrap"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

func buildHandlerMap() map[string]ai.ToolHandlerFunc {
	m := make(map[string]ai.ToolHandlerFunc)
	for _, def := range ai.AllToolDefs() {
		m[def.Name] = def.Handler
	}
	return m
}

func callHandler(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, toolName string, params map[string]any, decision ...*ai.CheckResult) int {
	handler, ok := handlers[toolName]
	if !ok {
		fmt.Fprintf(os.Stderr, "Internal error: unknown tool %s\n", toolName)
		return 1
	}

	if params == nil {
		params = map[string]any{}
	}

	ctx = ai.WithAuditSource(ctx, "opsctl")
	result, err := handler(ctx, params)

	// 写审计日志
	argsJSON, marshalErr := json.Marshal(params)
	if marshalErr != nil {
		logger.Ctx(ctx).Warn("marshal audit params", zap.Error(marshalErr))
	}
	var dec *ai.CheckResult
	if len(decision) > 0 {
		dec = decision[0]
	}
	writeOpsctlAudit(ctx, toolName, string(argsJSON), result, err, dec)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// 写操作成功后通知桌面端刷新 UI
	if toolName == "add_asset" || toolName == "update_asset" {
		dataDir := bootstrap.AppDataDir()
		token, tokenErr := bootstrap.ReadAuthToken(dataDir)
		if tokenErr != nil {
			logger.Ctx(ctx).Warn("read auth token", zap.Error(tokenErr))
		}
		approval.SendNotification(
			approval.SocketPath(dataDir),
			token,
			"asset",
		)
	}

	// Pretty-print JSON output
	var obj any
	if json.Unmarshal([]byte(result), &obj) == nil {
		pretty, err := json.MarshalIndent(obj, "", "  ")
		if err == nil {
			fmt.Println(string(pretty))
			return 0
		}
	}
	fmt.Println(result)
	return 0
}

// opsctlAuditWriter 全局审计写入器
var opsctlAuditWriter ai.AuditWriter = ai.NewDefaultAuditWriter()

// writeOpsctlAudit 统一的审计日志写入函数
func writeOpsctlAudit(ctx context.Context, toolName, argsJSON, result string, execErr error, decision *ai.CheckResult) {
	opsctlAuditWriter.WriteToolCall(ctx, ai.ToolCallInfo{
		ToolName: toolName,
		ArgsJSON: argsJSON,
		Result:   result,
		Error:    execErr,
		Decision: decision,
	})
}
