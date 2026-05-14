package ai

import (
	"context"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

func CheckK8sPolicy(ctx context.Context, policy *asset_entity.K8sPolicy, command string) CheckResult {
	merged := effectiveK8sPolicy(ctx, policy)
	return checkK8sPolicyRules(ctx, merged, command)
}

func checkK8sPolicyRules(ctx context.Context, policy *asset_entity.K8sPolicy, command string) CheckResult {
	if policy == nil {
		return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
	}

	// 拆 shell 执行单元，避免组合命令绕过策略。
	// 解析失败或提取不到执行单元（仅注释/空白等）时都退到 NeedConfirm，
	// 禁止退回到整串匹配，否则 allow `*` 会误放行无法枚举为子命令的输入。
	subCmds, err := ExtractSubCommands(command)
	if err != nil || len(subCmds) == 0 {
		return CheckResult{Decision: NeedConfirm}
	}

	// deny：任一子命令命中即拒绝
	for _, sub := range subCmds {
		for _, rule := range policy.DenyList {
			if MatchCommandRule(rule, sub) {
				return CheckResult{
					Decision:       Deny,
					Message:        policyFmt(ctx, "kubectl command denied by policy: %s", "kubectl 命令被策略禁止: %s", sub),
					DecisionSource: SourcePolicyDeny,
					MatchedPattern: rule,
				}
			}
		}
	}

	// allow：所有子命令都需命中
	if len(policy.AllowList) > 0 {
		if ok, matched := allSubCommandsAllowed(subCmds, policy.AllowList); ok {
			return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow, MatchedPattern: matched}
		}
		return CheckResult{Decision: NeedConfirm}
	}

	return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
}
