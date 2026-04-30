package ai

import (
	"context"
	"strings"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

func CheckK8sPolicy(ctx context.Context, policy *asset_entity.K8sPolicy, command string) CheckResult {
	merged := mergeK8sPolicy(policy, asset_entity.DefaultK8sPolicy())
	return checkK8sPolicyRules(ctx, merged, command)
}

func checkK8sPolicyRules(ctx context.Context, policy *asset_entity.K8sPolicy, command string) CheckResult {
	if policy == nil {
		return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
	}

	for _, rule := range policy.DenyList {
		if MatchCommandRule(rule, command) {
			return CheckResult{
				Decision:       Deny,
				Message:        policyFmt(ctx, "kubectl command denied by policy: %s", "kubectl 命令被策略禁止: %s", command),
				DecisionSource: SourcePolicyDeny,
				MatchedPattern: rule,
			}
		}
	}

	if len(policy.AllowList) > 0 {
		for _, rule := range policy.AllowList {
			if MatchCommandRule(rule, command) {
				return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow, MatchedPattern: rule}
			}
		}
		return CheckResult{Decision: NeedConfirm}
	}

	return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
}

func mergeK8sPolicy(custom, defaults *asset_entity.K8sPolicy) *asset_entity.K8sPolicy {
	result := &asset_entity.K8sPolicy{}
	if custom != nil {
		result.AllowList = custom.AllowList
		result.DenyList = append(result.DenyList, custom.DenyList...)
	}
	if defaults != nil {
		seen := make(map[string]bool, len(result.DenyList))
		for _, rule := range result.DenyList {
			seen[strings.ToUpper(rule)] = true
		}
		for _, rule := range defaults.DenyList {
			key := strings.ToUpper(rule)
			if !seen[key] {
				result.DenyList = append(result.DenyList, rule)
				seen[key] = true
			}
		}
	}
	return result
}
