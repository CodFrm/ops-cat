package extruntime

import "encoding/json"

// ExtDecision 扩展策略判定结果
type ExtDecision int

const (
	ExtAllow       ExtDecision = iota // 直接放行
	ExtDeny                           // 拒绝
	ExtNeedConfirm                    // 需要用户确认
)

// ExtPolicyResult 扩展策略检查结果
type ExtPolicyResult struct {
	Decision       ExtDecision
	Message        string
	DecisionSource string
}

// ExtensionPolicy 扩展策略配置
type ExtensionPolicy struct {
	AllowList []string `json:"allow_list"`
	DenyList  []string `json:"deny_list"`
}

// CheckExtensionPolicy 检查 action 是否被扩展策略允许
// deny 优先于 allow，"*" 匹配所有 action
func CheckExtensionPolicy(policyJSON string, action, resource string) ExtPolicyResult {
	if policyJSON == "" {
		return ExtPolicyResult{Decision: ExtNeedConfirm}
	}
	var policy ExtensionPolicy
	if err := json.Unmarshal([]byte(policyJSON), &policy); err != nil {
		return ExtPolicyResult{Decision: ExtNeedConfirm}
	}
	for _, d := range policy.DenyList {
		if d == action || d == "*" {
			return ExtPolicyResult{
				Decision:       ExtDeny,
				Message:        "action " + action + " is denied by policy",
				DecisionSource: "policy_deny",
			}
		}
	}
	for _, a := range policy.AllowList {
		if a == action || a == "*" {
			return ExtPolicyResult{
				Decision:       ExtAllow,
				DecisionSource: "policy_allow",
			}
		}
	}
	return ExtPolicyResult{Decision: ExtNeedConfirm}
}
