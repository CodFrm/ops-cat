package ai

import (
	"context"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/group_svc"
)

// MatchFunc 通用命令匹配函数签名
type MatchFunc func(rule, command string) bool

// PolicyTestInput 策略测试入参
type PolicyTestInput struct {
	PolicyType string // "ssh" | "database" | "redis" | "k8s"
	AssetID    int64  // 资产ID（从资产的 groupID 开始解析组链）
	GroupID    int64  // 资产组ID（从父组开始解析，当前组策略由 Current* 字段提供）

	// 当前编辑中的策略（来自前端 UI state，可能未保存）
	CurrentSSH   *asset_entity.CommandPolicy
	CurrentQuery *asset_entity.QueryPolicy
	CurrentRedis *asset_entity.RedisPolicy
	CurrentK8s   *asset_entity.K8sPolicy
}

// PolicyTestOutput 策略测试结果
type PolicyTestOutput struct {
	Decision       Decision
	MatchedPattern string
	MatchedSource  string // "" 当前策略, "default" 默认规则, 组名
	Message        string
}

// taggedRule 带来源标签的规则
type taggedRule struct {
	Rule, Source string
}

// TestPolicy 统一的策略测试入口，解析资产组链并合并策略后检查命令。
func TestPolicy(ctx context.Context, input PolicyTestInput, command string) PolicyTestOutput {
	groups := resolveGroupChainForTest(ctx, input.AssetID, input.GroupID)

	switch input.PolicyType {
	case "ssh":
		return testSSHPolicy(ctx, input.CurrentSSH, groups, command)
	case "database":
		return testQueryPolicy(ctx, input.CurrentQuery, groups, command)
	case "redis":
		return testRedisPolicy(ctx, input.CurrentRedis, groups, command)
	case "k8s":
		return testK8sPolicy(ctx, input.CurrentK8s, groups, command)
	}
	return PolicyTestOutput{Decision: NeedConfirm}
}

// --- 通用组规则收集 ---

// collectGroupGenericRules 从组链的 CmdPolicy（通用策略）中收集 deny/allow 规则。
// 组的 CmdPolicy 是通用类型，适用于所有资产类型。
func collectGroupGenericRules(ctx context.Context, groups []*group_entity.Group) (deny, allow []taggedRule) {
	for _, g := range groups {
		p, err := g.GetCommandPolicy()
		if err != nil || p == nil {
			continue
		}
		// 解析引用的权限组
		if len(p.Groups) > 0 {
			grpAllow, grpDeny := resolveCommandGroups(ctx, p.Groups)
			for _, r := range grpAllow {
				allow = append(allow, taggedRule{r, g.Name})
			}
			for _, r := range grpDeny {
				deny = append(deny, taggedRule{r, g.Name})
			}
		}
		for _, r := range p.DenyList {
			deny = append(deny, taggedRule{r, g.Name})
		}
		for _, r := range p.AllowList {
			allow = append(allow, taggedRule{r, g.Name})
		}
	}
	return
}

// checkGenericDeny 用指定的 matcher 检查 deny 规则
func checkGenericDeny(rules []taggedRule, command string, matchFn MatchFunc) *PolicyTestOutput {
	for _, tr := range rules {
		if matchFn(tr.Rule, command) {
			return &PolicyTestOutput{
				Decision:       Deny,
				MatchedPattern: tr.Rule,
				MatchedSource:  tr.Source,
			}
		}
	}
	return nil
}

// checkGenericAllow 用指定的 matcher 检查 allow 规则
func checkGenericAllow(rules []taggedRule, command string, matchFn MatchFunc) *PolicyTestOutput {
	for _, tr := range rules {
		if matchFn(tr.Rule, command) {
			return &PolicyTestOutput{
				Decision:       Allow,
				MatchedPattern: tr.Rule,
				MatchedSource:  tr.Source,
			}
		}
	}
	return nil
}

// --- SSH ---

func testSSHPolicy(ctx context.Context, current *asset_entity.CommandPolicy, groups []*group_entity.Group, command string) PolicyTestOutput {
	subCmds, err := ExtractSubCommands(command)
	if err != nil || len(subCmds) == 0 {
		subCmds = []string{command}
	}

	var denyRules, allowRules []taggedRule

	// 当前编辑的策略（资产自身）
	if current != nil {
		// 解析引用的权限组
		if len(current.Groups) > 0 {
			grpAllow, grpDeny := resolveCommandGroups(ctx, current.Groups)
			for _, r := range grpAllow {
				allowRules = append(allowRules, taggedRule{r, ""})
			}
			for _, r := range grpDeny {
				denyRules = append(denyRules, taggedRule{r, ""})
			}
		}
		for _, r := range current.DenyList {
			denyRules = append(denyRules, taggedRule{r, ""})
		}
		for _, r := range current.AllowList {
			allowRules = append(allowRules, taggedRule{r, ""})
		}
	}
	// 组链通用策略（SSH 直接用 MatchCommandRule）
	groupDeny, groupAllow := collectGroupGenericRules(ctx, groups)
	denyRules = append(denyRules, groupDeny...)
	allowRules = append(allowRules, groupAllow...)

	// flat allow rules
	allowFlat := make([]string, 0, len(allowRules))
	for _, r := range allowRules {
		allowFlat = append(allowFlat, r.Rule)
	}

	// deny 检查
	for _, cmd := range subCmds {
		for _, tr := range denyRules {
			if MatchCommandRule(tr.Rule, cmd) {
				hints := findHintRules(cmd, allowFlat)
				return PolicyTestOutput{
					Decision:       Deny,
					MatchedPattern: tr.Rule,
					MatchedSource:  tr.Source,
					Message:        formatDenyMessage(ctx, "", command, policyMsg(ctx, "command blocked by policy", "命令被策略禁止执行"), hints),
				}
			}
		}
	}

	// allow 检查
	if ok, _ := allSubCommandsAllowed(subCmds, allowFlat); len(allowFlat) > 0 && ok {
		source := ""
		for _, cmd := range subCmds {
			for _, tr := range allowRules {
				if MatchCommandRule(tr.Rule, cmd) {
					source = tr.Source
					break
				}
			}
			if source != "" {
				break
			}
		}
		return PolicyTestOutput{Decision: Allow, MatchedSource: source}
	}

	return PolicyTestOutput{Decision: NeedConfirm}
}

// --- Database ---

func testQueryPolicy(ctx context.Context, current *asset_entity.QueryPolicy, groups []*group_entity.Group, command string) PolicyTestOutput {
	// 先检查组通用规则（用 MatchCommandRule，SQL 以动词开头可匹配）
	groupDeny, groupAllow := collectGroupGenericRules(ctx, groups)
	if out := checkGenericDeny(groupDeny, command, MatchCommandRule); out != nil {
		out.Message = policyFmt(ctx, "SQL statement denied by group policy: %s", "SQL 语句被组策略禁止: %s", command)
		return *out
	}

	// 类型专用策略：当前（含引用的权限组）
	if current != nil && len(current.Groups) > 0 {
		grpAllowTypes, grpDenyTypes, grpDenyFlags := resolveQueryGroups(ctx, current.Groups)
		current.AllowTypes = append(current.AllowTypes, grpAllowTypes...)
		current.DenyTypes = append(current.DenyTypes, grpDenyTypes...)
		current.DenyFlags = append(current.DenyFlags, grpDenyFlags...)
	}

	// 解析 SQL
	stmts, err := ClassifyStatements(command)
	if err != nil {
		return PolicyTestOutput{
			Decision: Deny,
			Message:  policyFmt(ctx, "SQL parse failed, execution denied: %v", "SQL 解析失败，拒绝执行: %v", err),
		}
	}

	// 复用核心策略检查逻辑（不合并默认策略，仅检查用户配置的规则）
	result := checkQueryPolicyRules(ctx, current, stmts)
	if result.Decision == Deny {
		return PolicyTestOutput{
			Decision:       Deny,
			MatchedPattern: result.MatchedPattern,
			MatchedSource:  "", // 当前资产策略
			Message:        result.Message,
		}
	}
	if result.Decision == NeedConfirm {
		return PolicyTestOutput{Decision: NeedConfirm}
	}

	// 检查组通用 allow 规则
	if out := checkGenericAllow(groupAllow, command, MatchCommandRule); out != nil {
		return *out
	}

	return PolicyTestOutput{Decision: Allow}
}

// --- Redis ---

func testRedisPolicy(ctx context.Context, current *asset_entity.RedisPolicy, groups []*group_entity.Group, command string) PolicyTestOutput {
	// 先检查组通用规则（用 MatchRedisRule）
	groupDeny, groupAllow := collectGroupGenericRules(ctx, groups)
	if out := checkGenericDeny(groupDeny, command, MatchRedisRule); out != nil {
		out.Message = policyFmt(ctx, "Redis command denied by group policy: %s", "Redis 命令被组策略禁止: %s", command)
		return *out
	}

	// 类型专用策略：当前资产（含引用的权限组）
	if current != nil && len(current.Groups) > 0 {
		grpAllow, grpDeny := resolveRedisGroups(ctx, current.Groups)
		current.AllowList = append(current.AllowList, grpAllow...)
		current.DenyList = append(current.DenyList, grpDeny...)
	}

	// 复用核心策略检查逻辑（不合并默认策略，仅检查用户配置的规则）
	result := checkRedisPolicyRules(ctx, current, command)

	// deny 结果映射
	if result.Decision == Deny {
		return PolicyTestOutput{
			Decision:       Deny,
			MatchedPattern: result.MatchedPattern,
			MatchedSource:  "", // 当前资产策略
			Message:        result.Message,
		}
	}

	// 检查组通用 allow 规则（优先于资产 allow 结论）
	if out := checkGenericAllow(groupAllow, command, MatchRedisRule); out != nil {
		return *out
	}

	// 映射资产策略结果（Allow 或 NeedConfirm）
	if result.Decision == NeedConfirm {
		return PolicyTestOutput{Decision: NeedConfirm}
	}
	return PolicyTestOutput{Decision: Allow}
}

// --- K8S ---

func testK8sPolicy(ctx context.Context, current *asset_entity.K8sPolicy, groups []*group_entity.Group, command string) PolicyTestOutput {
	var policies []*asset_entity.K8sPolicy
	if current != nil {
		policies = append(policies, current)
	}
	for _, g := range groups {
		p, err := g.GetK8sPolicy()
		if err != nil || p == nil {
			continue
		}
		policies = append(policies, p)
	}

	if len(policies) == 0 {
		return PolicyTestOutput{Decision: NeedConfirm}
	}

	for _, p := range policies {
		if len(p.Groups) > 0 {
			grpAllow, grpDeny := resolveCommandGroups(ctx, p.Groups)
			p.AllowList = append(p.AllowList, grpAllow...)
			p.DenyList = append(p.DenyList, grpDeny...)
		}
	}

	merged := &asset_entity.K8sPolicy{}
	for _, p := range policies {
		if len(merged.AllowList) == 0 && len(p.AllowList) > 0 {
			merged.AllowList = p.AllowList
		}
		merged.DenyList = appendUnique(merged.DenyList, p.DenyList...)
	}

	result := checkK8sPolicyRules(ctx, merged, command)
	if result.Decision == Deny {
		return PolicyTestOutput{
			Decision:       Deny,
			MatchedPattern: result.MatchedPattern,
			MatchedSource:  "",
			Message:        result.Message,
		}
	}
	if result.Decision == NeedConfirm {
		return PolicyTestOutput{Decision: NeedConfirm}
	}
	return PolicyTestOutput{Decision: Allow}
}

// --- 通用组链解析 ---

// resolveGroupChainForTest 根据 assetID 或 groupID 解析组链。
func resolveGroupChainForTest(ctx context.Context, assetID, groupID int64) []*group_entity.Group {
	var startGroupID int64

	if assetID > 0 {
		asset, err := asset_svc.Asset().Get(ctx, assetID)
		if err == nil && asset != nil {
			startGroupID = asset.GroupID
		}
	} else if groupID > 0 {
		// GroupDetail 编辑：当前组的策略已由调用方提供，从父组开始
		g, err := group_svc.Group().Get(ctx, groupID)
		if err == nil && g != nil {
			startGroupID = g.ParentID
		}
	}

	if startGroupID == 0 {
		return nil
	}

	var chain []*group_entity.Group
	currentID := startGroupID
	for i := 0; i < 5 && currentID > 0; i++ {
		g, err := group_svc.Group().Get(ctx, currentID)
		if err != nil || g == nil {
			break
		}
		chain = append(chain, g)
		currentID = g.ParentID
	}
	return chain
}

// CheckGroupGenericPolicy 在真实执行路径中检查组的通用策略（CmdPolicy）。
// 对 Redis/Database 资产，在类型专用策略检查之外额外检查组通用规则。
func CheckGroupGenericPolicy(ctx context.Context, assetID int64, command string, matchFn MatchFunc) CheckResult {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil || asset == nil || asset.GroupID == 0 {
		return CheckResult{Decision: NeedConfirm}
	}

	groups := resolveGroupChain(ctx, asset.GroupID)
	for _, g := range groups {
		p, err := g.GetCommandPolicy()
		if err != nil || p == nil {
			continue
		}
		// 解析引用的权限组
		var allDeny []string
		if len(p.Groups) > 0 {
			_, grpDeny := resolveCommandGroups(ctx, p.Groups)
			allDeny = append(allDeny, grpDeny...)
		}
		allDeny = append(allDeny, p.DenyList...)

		// deny 检查
		for _, rule := range allDeny {
			if matchFn(rule, command) {
				return CheckResult{
					Decision:       Deny,
					Message:        policyFmt(ctx, "command denied by group [%s] policy: %s", "命令被组 [%s] 策略禁止: %s", g.Name, command),
					DecisionSource: SourcePolicyDeny,
					MatchedPattern: rule,
				}
			}
		}
	}

	// allow 检查
	for _, g := range groups {
		p, err := g.GetCommandPolicy()
		if err != nil || p == nil {
			continue
		}
		var allAllow []string
		if len(p.Groups) > 0 {
			grpAllow, _ := resolveCommandGroups(ctx, p.Groups)
			allAllow = append(allAllow, grpAllow...)
		}
		allAllow = append(allAllow, p.AllowList...)
		for _, rule := range allAllow {
			if matchFn(rule, command) {
				return CheckResult{
					Decision:       Allow,
					DecisionSource: SourcePolicyAllow,
					MatchedPattern: rule,
				}
			}
		}
	}

	return CheckResult{Decision: NeedConfirm}
}
