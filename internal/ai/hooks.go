package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// checkResultKey 是 audit middleware 在 ctx 上挂的 *CheckResult 槽的 key。
// tool handler 通过 RecordDecision(ctx, r) 写决策；audit middleware 在 c.Next()
// 返回后读取该槽落审计。声明为带名空结构体（按 Go 推荐做法）以避免 string key 冲突。
type checkResultKey struct{}

// auditMiddleware 是 cago tool dispatch 的"around"中间件，负责审计落库。
//
// 工作流：
//  1. 建一个 *CheckResult slot，挂到 c.ctx（通过 c.WithContext）。
//  2. c.Next() 推链：后续 mw（如 LocalToolGate）+ 终端 tool.Call 全跑；tool
//     内部用 RecordDecision(ctx, r) 把决策写进 slot。
//  3. c.Next() 返回后，从 c.Output 抽出文本 / 错误，组合成 ToolCallInfo，
//     起 goroutine 异步写入审计仓库。
//
// 每次调用的状态都保存在当前 ctx 和闭包内，不通过全局索引跨调用共享。
func auditMiddleware(c *agent.ToolContext) {
	slot := &CheckResult{}
	c.WithContext(context.WithValue(c.Context(), checkResultKey{}, slot))

	c.Next()

	argsJSON, err := json.Marshal(c.Input)
	if err != nil {
		logger.Default().Warn("audit middleware marshal input", zap.Error(err))
	}

	result, errVal := extractAuditResult(c.Output)

	info := ToolCallInfo{
		ToolName: c.ToolName,
		ArgsJSON: string(argsJSON),
		Result:   result,
		Error:    errVal,
		Decision: slot,
	}
	auditCtx := detachAuditContext(c.Context())
	go auditWriter.WriteToolCall(auditCtx, info)
}

// extractAuditResult 把 cago 的 *ToolResultBlock 拆成审计需要的 (result, error)。
// IsError=true 的块当成 error 路径，文本作为 error message。
func extractAuditResult(block *agent.ToolResultBlock) (string, error) {
	if block == nil {
		return "", nil
	}
	var b strings.Builder
	for _, c := range block.Content {
		switch v := c.(type) {
		case agent.TextBlock:
			b.WriteString(v.Text)
		case *agent.TextBlock:
			if v != nil {
				b.WriteString(v.Text)
			}
		}
	}
	text := b.String()
	if block.IsError {
		if text == "" {
			text = "tool call failed"
		}
		return "", errors.New(text)
	}
	return text, nil
}

// detachAuditContext 把审计需要的 ctx value 复制到一个全新的 context.Background()
// 之上，避免 runner ctx 被 Cancel 后导致审计写入异步中断。
func detachAuditContext(ctx context.Context) context.Context {
	out := context.Background()
	if v := GetAuditSource(ctx); v != "" {
		out = WithAuditSource(out, v)
	}
	if v := GetConversationID(ctx); v != 0 {
		out = WithConversationID(out, v)
	}
	if v := GetGrantSessionID(ctx); v != "" {
		out = WithGrantSessionID(out, v)
	}
	if v := GetSessionID(ctx); v != "" {
		out = WithSessionID(out, v)
	}
	return out
}
