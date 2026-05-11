package aiagent

import (
	"context"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// PolicyDecision 是 PolicyChecker 的三态返回。Allow → 直接放行；Deny → 立即拒绝；
// Confirm → 需要弹用户审批卡（policy hook 负责走 gateway 完成 round trip）。
type PolicyDecision int

const (
	PolicyAllow PolicyDecision = iota
	PolicyDeny
	PolicyConfirm
)

// PolicyOutcome 是 PolicyChecker.Check 的丰富返回：除了三态决策本身，还携带
// audit 需要的来源字段（DecisionSource / MatchedPattern），让 hook 链路下游
// 不必重复检查就能写出正确的审计日志。
//
// kind 与 ai.ApprovalItem.Type 对齐：资产维度 "exec" / "sql" / "redis" / "mongo"
// / "kafka" / "k8s" / "cp"；本地工具 "local_bash" / "local_write" / "local_edit"；
// 空串 = 不属于策略门控范围（list_assets 等 read-only 工具）。
type PolicyOutcome struct {
	Decision       PolicyDecision
	Item           ai.ApprovalItem // Confirm 分支用于弹卡；非 Confirm 可为零值
	Reason         string          // Deny 分支返回给 AI 的拒绝原因
	DecisionSource string          // ai.SourcePolicyAllow / ai.SourcePolicyDeny / ...
	MatchedPattern string          // 命中的 grant 模式（若有），透传到 audit_log
}

// PolicyChecker 抽象命令策略。Check 失败（err != nil）会被 cago 当 HookError
// 处理 → EventError + StopHook，慎用；正常的"不放行"应当走 PolicyDeny。
type PolicyChecker interface {
	Check(ctx context.Context, toolName string, input map[string]any) (PolicyOutcome, error)
}

// newPolicyHook 构造 PreToolUse hook：
//   - PolicyAllow → DecisionPass，store 里写一条 Allow 决策给 audit
//   - PolicyDeny → DecisionDeny + reason，store 里写一条 Deny 决策给 audit
//   - PolicyConfirm → gw.RequestSingle 弹卡 + 等响应，store 决策按用户选择更新
//
// store 为 nil 时跳过 stash（测试场景兼容）；gw 为 nil 时 Confirm 视为 Deny ——
// 把"应当弹卡却没渠道"的 bug 暴露成显式拒绝，比无声放行安全。
func newPolicyHook(c PolicyChecker, gw *ApprovalGateway, store *toolDecisionStore) agent.PreToolUseHook {
	return func(ctx context.Context, in *agent.PreToolUseInput) (*agent.PreToolUseOutput, error) {
		outcome, err := c.Check(ctx, in.ToolName, in.Input)
		if err != nil {
			return nil, err
		}
		stash := func(dec ai.Decision, source string) {
			if store == nil {
				return
			}
			store.Stash(in.ToolUseID, &ai.CheckResult{
				Decision:       dec,
				DecisionSource: source,
				MatchedPattern: outcome.MatchedPattern,
			})
		}
		switch outcome.Decision {
		case PolicyAllow:
			stash(ai.Allow, outcome.DecisionSource)
			return &agent.PreToolUseOutput{Decision: agent.DecisionPass}, nil
		case PolicyDeny:
			stash(ai.Deny, outcome.DecisionSource)
			return &agent.PreToolUseOutput{Decision: agent.DecisionDeny, DenyReason: outcome.Reason}, nil
		case PolicyConfirm:
			if gw == nil {
				stash(ai.Deny, ai.SourcePolicyDeny)
				return &agent.PreToolUseOutput{Decision: agent.DecisionDeny, DenyReason: "no approval gateway"}, nil
			}
			resp := gw.RequestSingle(ctx, outcome.Item.Type, []ai.ApprovalItem{outcome.Item}, "")
			switch resp.Decision {
			case "allow", "allowAll":
				stash(ai.Allow, ai.SourceUserAllow)
				return &agent.PreToolUseOutput{Decision: agent.DecisionPass}, nil
			default:
				stash(ai.Deny, ai.SourceUserDeny)
				return &agent.PreToolUseOutput{Decision: agent.DecisionDeny, DenyReason: "user denied"}, nil
			}
		}
		return &agent.PreToolUseOutput{Decision: agent.DecisionPass}, nil
	}
}
