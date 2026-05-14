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

	// 拆 shell 执行单元，避免组合命令绕过策略
	subCmds, err := ExtractSubCommands(command)
	if err != nil {
		return CheckResult{Decision: NeedConfirm}
	}
	if len(subCmds) == 0 {
		subCmds = []string{command}
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
