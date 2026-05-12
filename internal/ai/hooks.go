package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// decisionMap 是 PreToolUseHook、工具 handler、PostToolUseHook 三方共享的
// per-call 状态表，key 是 agent.ToolUseID。
//
// 用法：
//  1. attachCheckResultHook (PreHook) 在每次工具调用前 Store 一个 *CheckResult 占位指针；
//  2. handler 内部调 setCheckResult，通过 ToolUseIDFromContext 拿 ID 后写入指针；
//  3. auditPostHook (PostHook) 取出 *CheckResult 并 LoadAndDelete 清理，写入审计日志。
//
// cago dispatcher 对每个 ToolUseID 只调一次 Run（见 agent/tooldispatch.go），不会
// 重复 Store；并发安全靠 sync.Map 本身。
var decisionMap sync.Map

// attachCheckResultHook 注册为 PreToolUseHook（matcher ".*"），为每次工具调用准备
// 一个空的 *CheckResult 槽，供 handler 填决策、PostHook 读决策。
func attachCheckResultHook(_ context.Context, in *agent.PreToolUseInput) (*agent.PreToolUseOutput, error) {
	if in.ToolUseID == "" {
		return nil, nil
	}
	decisionMap.Store(in.ToolUseID, &CheckResult{})
	return nil, nil
}

// auditPostHook 注册为 PostToolUseHook（matcher ".*"），从 ToolResultBlock 抽出
// 文本和错误，组合 decisionMap 里的决策，异步写入审计日志。
func auditPostHook(ctx context.Context, in *agent.PostToolUseInput) (*agent.PostToolUseOutput, error) {
	var decision *CheckResult
	if in.ToolUseID != "" {
		if v, ok := decisionMap.LoadAndDelete(in.ToolUseID); ok {
			decision = v.(*CheckResult)
		}
	}

	argsJSON, err := json.Marshal(in.Input)
	if err != nil {
		logger.Default().Warn("audit hook marshal input", zap.Error(err))
	}

	result, errVal := extractAuditResult(in.Output)

	info := ToolCallInfo{
		ToolName: in.ToolName,
		ArgsJSON: string(argsJSON),
		Result:   result,
		Error:    errVal,
		Decision: decision,
	}
	// 复制 ctx 里的值到独立 context，避免 dispatcher ctx 被取消后审计写入失败
	auditCtx := detachAuditContext(ctx)
	go auditWriter.WriteToolCall(auditCtx, info)
	return nil, nil
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
