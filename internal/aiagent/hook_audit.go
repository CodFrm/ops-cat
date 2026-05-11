package aiagent

import (
	"context"
	"encoding/json"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// AuditWriter 写一条工具调用审计。decision 由 policy hook 通过 toolDecisionStore
// 旁路传过来——*ai.CheckResult 包含 Decision/DecisionSource/MatchedPattern，对应
// audit_log 的同名列。decision 为 nil 时，audit 还是会写一行（只是来源字段为空）。
//
// Write 错误被吞掉（审计失败不该影响 agent 主流程）；实现里建议自己 logger.Warn。
type AuditWriter interface {
	Write(ctx context.Context, toolName, inputJSON, outputJSON string, isError bool, decision *ai.CheckResult) error
}

// newAuditHook 返回 PostToolUse hook：序列化 input + output 后调用 AuditWriter，
// 同时 Pop 出 policy hook 在 PreToolUse 阶段留下的决策记录传给 Writer。
//
// store 为 nil 时（测试场景）decision 一律按 nil 传——AuditWriter 实现里需要
// 容忍 nil decision，否则 audit 会 panic。
func newAuditHook(w AuditWriter, store *toolDecisionStore) agent.PostToolUseHook {
	return func(ctx context.Context, in *agent.PostToolUseInput) (*agent.PostToolUseOutput, error) {
		// Defensive: skip if Output is nil (shouldn't happen per cago contract,
		// but a malformed tool result would panic on .Content access).
		if in.Output == nil {
			return &agent.PostToolUseOutput{}, nil
		}
		inJSON, _ := json.Marshal(in.Input)
		outBlocks := serializeBlocks(in.Output.Content)
		outJSON, _ := json.Marshal(outBlocks)
		var decision *ai.CheckResult
		if store != nil {
			decision = store.Pop(in.ToolUseID)
		}
		_ = w.Write(ctx, in.ToolName, string(inJSON), string(outJSON), in.Output.IsError, decision)
		return &agent.PostToolUseOutput{}, nil
	}
}
