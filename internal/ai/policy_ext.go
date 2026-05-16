package ai

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// ExtensionPolicyRule represents the allow/deny action lists in an extension policy group's Policy JSON.
type ExtensionPolicyRule struct {
	AllowList []string `json:"allow_list"`
	DenyList  []string `json:"deny_list"`
}

// checkExtensionPolicy resolves ext: prefixed policy groups and checks allow/deny action lists.
// Logic:
//   - If no groupIDs → NeedConfirm
//   - Fetch policy groups via fetchPolicyGroups (handles ext: prefix)
//   - Unmarshal each group's Policy JSON into ExtensionPolicyRule
//   - Deny takes precedence: if action in any deny_list → Deny
//   - Then check allow: if action in any allow_list → Allow
//   - Otherwise → NeedConfirm
func checkExtensionPolicy(ctx context.Context, groupIDs []string, action string) CheckResult {
	if len(groupIDs) == 0 {
		return CheckResult{Decision: NeedConfirm}
	}

	groups := fetchPolicyGroups(ctx, groupIDs)
	if len(groups) == 0 {
		return CheckResult{Decision: NeedConfirm}
	}

	var allAllow []string
	var allDeny []string

	for _, pg := range groups {
		var rule ExtensionPolicyRule
		if err := json.Unmarshal([]byte(pg.Policy), &rule); err != nil {
			logger.Default().Warn("unmarshal extension policy group",
				zap.String("id", pg.BuiltinID), zap.Error(err))
			continue
		}
		allAllow = append(allAllow, rule.AllowList...)
		allDeny = append(allDeny, rule.DenyList...)
	}

	// Deny takes precedence
	if slices.Contains(allDeny, action) {
		return CheckResult{
			Decision:       Deny,
			DecisionSource: SourcePolicyDeny,
			Message:        "action denied by extension policy: " + action,
		}
	}

	// Then check allow
	if slices.Contains(allAllow, action) {
		return CheckResult{
			Decision:       Allow,
			DecisionSource: SourcePolicyAllow,
		}
	}

	return CheckResult{Decision: NeedConfirm}
}
