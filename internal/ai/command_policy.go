package ai

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"ops-cat/internal/model/entity/asset_entity"
	"ops-cat/internal/model/entity/group_entity"
	"ops-cat/internal/repository/group_repo"
	"ops-cat/internal/service/asset_svc"

	"mvdan.cc/sh/v3/syntax"
)

// Decision 权限判定结果
type Decision int

const (
	Allow       Decision = iota // 直接放行
	Deny                        // 拒绝
	NeedConfirm                 // 需要用户确认
)

// CheckResult 权限检查结果
type CheckResult struct {
	Decision  Decision
	Message   string   // 返回给 AI 的消息
	HintRules []string // 拒绝时的允许规则提示
}

// CommandConfirmFunc 命令确认回调，阻塞等待用户响应
type CommandConfirmFunc func(assetName, command string) (allowed, alwaysAllow bool)

// CommandPolicyChecker 命令权限检查器，通过 context 注入到两条执行路径
type CommandPolicyChecker struct {
	confirmFunc    CommandConfirmFunc
	sessionAllowed []string // 会话级白名单（命令名）
	mu             sync.Mutex
}

// NewCommandPolicyChecker 创建权限检查器
func NewCommandPolicyChecker(confirmFunc CommandConfirmFunc) *CommandPolicyChecker {
	return &CommandPolicyChecker{
		confirmFunc: confirmFunc,
	}
}

// Reset 重置会话级白名单
func (c *CommandPolicyChecker) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionAllowed = nil
}

// Check 检查命令是否允许执行
func (c *CommandPolicyChecker) Check(ctx context.Context, assetID int64, command string) CheckResult {
	// 1. 提取所有子命令
	subCmds, err := ExtractSubCommands(command)
	if err != nil || len(subCmds) == 0 {
		// 解析失败，整条视为一个命令
		subCmds = []string{command}
	}

	// 2. 获取资产 + 组链
	asset, _ := asset_svc.Asset().Get(ctx, assetID)
	var groups []*group_entity.Group
	if asset != nil && asset.GroupID > 0 {
		groups = resolveGroupChain(ctx, asset.GroupID)
	}

	// 收集所有层级的策略
	allPolicies := collectPolicies(asset, groups)
	allDenyRules := collectDenyRules(allPolicies)
	allAllowRules := collectAllowRules(allPolicies)

	// 3. 检查 deny list（所有层级合并，任一匹配即拒绝）
	for _, cmd := range subCmds {
		for _, rule := range allDenyRules {
			if MatchCommandRule(rule, cmd) {
				assetName := ""
				if asset != nil {
					assetName = asset.Name
				}
				hints := findHintRules(cmd, allAllowRules)
				msg := formatDenyMessage(assetName, command, "命令被策略禁止执行", hints)
				return CheckResult{Decision: Deny, Message: msg, HintRules: hints}
			}
		}
	}

	// 4. 检查 allow list（所有子命令都匹配才放行）
	if len(allAllowRules) > 0 && allSubCommandsAllowed(subCmds, allAllowRules) {
		return CheckResult{Decision: Allow}
	}

	// 5. 检查会话级白名单
	c.mu.Lock()
	sessionOK := allSubCommandsAllowed(subCmds, c.sessionAllowed)
	c.mu.Unlock()
	if sessionOK {
		return CheckResult{Decision: Allow}
	}

	// 6. 请求用户确认
	if c.confirmFunc == nil {
		hints := findHintRules(subCmds[0], allAllowRules)
		msg := formatDenyMessage("", command, "命令未授权且无确认机制", hints)
		return CheckResult{Decision: Deny, Message: msg, HintRules: hints}
	}

	assetName := ""
	if asset != nil {
		assetName = asset.Name
	}
	allowed, alwaysAllow := c.confirmFunc(assetName, command)
	if !allowed {
		hints := findHintRules(subCmds[0], allAllowRules)
		msg := formatDenyMessage(assetName, command, "用户拒绝执行", hints)
		return CheckResult{Decision: Deny, Message: msg, HintRules: hints}
	}

	// "始终允许" → 每个子命令加入会话白名单
	if alwaysAllow {
		c.mu.Lock()
		c.sessionAllowed = append(c.sessionAllowed, subCmds...)
		c.mu.Unlock()
	}

	return CheckResult{Decision: Allow}
}

// CheckPolicyOnly 只检查 allow/deny 列表，不触发确认回调。
// 返回 Allow（允许列表匹配）、Deny（拒绝列表匹配）或 NeedConfirm（未匹配任何列表）。
func CheckPolicyOnly(ctx context.Context, assetID int64, command string) CheckResult {
	subCmds, err := ExtractSubCommands(command)
	if err != nil || len(subCmds) == 0 {
		subCmds = []string{command}
	}

	asset, _ := asset_svc.Asset().Get(ctx, assetID)
	var groups []*group_entity.Group
	if asset != nil && asset.GroupID > 0 {
		groups = resolveGroupChain(ctx, asset.GroupID)
	}

	allPolicies := collectPolicies(asset, groups)
	allDenyRules := collectDenyRules(allPolicies)
	allAllowRules := collectAllowRules(allPolicies)

	// Check deny list
	for _, cmd := range subCmds {
		for _, rule := range allDenyRules {
			if MatchCommandRule(rule, cmd) {
				assetName := ""
				if asset != nil {
					assetName = asset.Name
				}
				hints := findHintRules(cmd, allAllowRules)
				msg := formatDenyMessage(assetName, command, "命令被策略禁止执行", hints)
				return CheckResult{Decision: Deny, Message: msg, HintRules: hints}
			}
		}
	}

	// Check allow list
	if len(allAllowRules) > 0 && allSubCommandsAllowed(subCmds, allAllowRules) {
		return CheckResult{Decision: Allow}
	}

	return CheckResult{Decision: NeedConfirm}
}

// CheckForAsset 按资产类型分发权限检查
func (c *CommandPolicyChecker) CheckForAsset(ctx context.Context, assetID int64, assetType, command string) CheckResult {
	switch assetType {
	case asset_entity.AssetTypeSSH:
		return c.Check(ctx, assetID, command)

	case asset_entity.AssetTypeDatabase:
		stmts, err := ClassifyStatements(command)
		if err != nil {
			return CheckResult{Decision: Deny, Message: fmt.Sprintf("SQL 解析失败，拒绝执行: %v", err)}
		}
		asset, _ := asset_svc.Asset().Get(ctx, assetID)
		var policy *asset_entity.QueryPolicy
		if asset != nil {
			policy, _ = asset.GetQueryPolicy()
		}
		result := CheckQueryPolicy(policy, stmts)
		if result.Decision == NeedConfirm {
			return c.handleConfirm(ctx, assetID, asset, command)
		}
		return result

	case asset_entity.AssetTypeRedis:
		asset, _ := asset_svc.Asset().Get(ctx, assetID)
		var policy *asset_entity.RedisPolicy
		if asset != nil {
			policy, _ = asset.GetRedisPolicy()
		}
		result := CheckRedisPolicy(policy, command)
		if result.Decision == NeedConfirm {
			return c.handleConfirm(ctx, assetID, asset, command)
		}
		return result
	}
	return CheckResult{Decision: NeedConfirm}
}

// handleConfirm 处理需要用户确认的情况
func (c *CommandPolicyChecker) handleConfirm(ctx context.Context, assetID int64, asset *asset_entity.Asset, command string) CheckResult {
	// 检查会话级白名单
	c.mu.Lock()
	sessionOK := containsStr(c.sessionAllowed, command)
	c.mu.Unlock()
	if sessionOK {
		return CheckResult{Decision: Allow}
	}

	if c.confirmFunc == nil {
		return CheckResult{Decision: Deny, Message: "命令未授权且无确认机制"}
	}

	assetName := ""
	if asset != nil {
		assetName = asset.Name
	}
	allowed, alwaysAllow := c.confirmFunc(assetName, command)
	if !allowed {
		return CheckResult{Decision: Deny, Message: fmt.Sprintf("用户拒绝执行: %s", command)}
	}
	if alwaysAllow {
		c.mu.Lock()
		c.sessionAllowed = append(c.sessionAllowed, command)
		c.mu.Unlock()
	}
	return CheckResult{Decision: Allow}
}

// --- context 注入 ---

type policyCheckerKeyType struct{}

// WithPolicyChecker 将 PolicyChecker 注入 context
func WithPolicyChecker(ctx context.Context, c *CommandPolicyChecker) context.Context {
	return context.WithValue(ctx, policyCheckerKeyType{}, c)
}

// GetPolicyChecker 从 context 中获取 PolicyChecker
func GetPolicyChecker(ctx context.Context) *CommandPolicyChecker {
	c, _ := ctx.Value(policyCheckerKeyType{}).(*CommandPolicyChecker)
	return c
}

// --- Shell AST 解析 ---

// ExtractSubCommands 从 shell 命令中提取所有子命令（处理 &&、||、;、|、$() 等）
func ExtractSubCommands(command string) ([]string, error) {
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, fmt.Errorf("shell 解析失败: %w", err)
	}

	var cmds []string
	printer := syntax.NewPrinter()

	var extractFromStmt func(stmt *syntax.Stmt)
	extractFromStmt = func(stmt *syntax.Stmt) {
		if stmt == nil || stmt.Cmd == nil {
			return
		}
		switch cmd := stmt.Cmd.(type) {
		case *syntax.BinaryCmd:
			// &&、||、| 等二元操作
			extractFromStmt(cmd.X)
			extractFromStmt(cmd.Y)
		default:
			// CallExpr、其他命令类型 — 打印为字符串
			var buf strings.Builder
			_ = printer.Print(&buf, stmt.Cmd)
			cmdStr := strings.TrimSpace(buf.String())
			if cmdStr != "" {
				cmds = append(cmds, cmdStr)
			}
		}
	}

	syntax.Walk(file, func(node syntax.Node) bool {
		stmt, ok := node.(*syntax.Stmt)
		if !ok {
			return true
		}
		extractFromStmt(stmt)
		return false
	})

	return cmds, nil
}

// --- 命令规则匹配 ---

// ParsedCommand 解析后的命令结构
type ParsedCommand struct {
	Program     string
	SubCommands []string
	Flags       map[string]string
	Wildcard    bool
}

// ParseCommandRule 将规则字符串解析为结构化表示
func ParseCommandRule(rule string) *ParsedCommand {
	tokens := tokenize(rule)
	if len(tokens) == 0 {
		return &ParsedCommand{}
	}

	result := &ParsedCommand{
		Program: tokens[0],
		Flags:   make(map[string]string),
	}

	i := 1
	for i < len(tokens) {
		t := tokens[i]
		if isFlag(t) {
			if strings.Contains(t, "=") {
				// --flag=value
				parts := strings.SplitN(t, "=", 2)
				result.Flags[parts[0]] = parts[1]
			} else if i+1 < len(tokens) && !isFlag(tokens[i+1]) {
				// -f value（* 在 flag 后面作为值，不是通配符）
				result.Flags[t] = tokens[i+1]
				i++
			} else {
				// 布尔 flag
				result.Flags[t] = ""
			}
		} else if t == "*" {
			// 只有非 flag 值位置的 * 才是通配符
			result.Wildcard = true
		} else {
			result.SubCommands = append(result.SubCommands, t)
		}
		i++
	}

	return result
}

// ParseActualCommand 解析实际命令，用规则的 flag 列表作为参照判断哪些 flag 带值
func ParseActualCommand(command string, rule *ParsedCommand) *ParsedCommand {
	tokens := tokenize(command)
	if len(tokens) == 0 {
		return &ParsedCommand{}
	}

	result := &ParsedCommand{
		Program: tokens[0],
		Flags:   make(map[string]string),
	}

	i := 1
	for i < len(tokens) {
		t := tokens[i]
		if isFlag(t) {
			if strings.Contains(t, "=") {
				parts := strings.SplitN(t, "=", 2)
				result.Flags[parts[0]] = parts[1]
			} else if i+1 < len(tokens) && !isFlag(tokens[i+1]) {
				// 用规则判断：如果规则中该 flag 带值，则实际命令中也视为带值
				if _, hasValue := rule.Flags[t]; hasValue || rule.Flags[t] != "" {
					result.Flags[t] = tokens[i+1]
					i++
				} else {
					// 规则中没有该 flag，按启发式处理：
					// 如果下一个 token 不是 flag 且不以 - 开头，视为带值
					result.Flags[t] = tokens[i+1]
					i++
				}
			} else {
				result.Flags[t] = ""
			}
		} else {
			result.SubCommands = append(result.SubCommands, t)
		}
		i++
	}

	return result
}

// MatchCommandRule 检查实际命令是否匹配规则字符串
func MatchCommandRule(rule, command string) bool {
	parsedRule := ParseCommandRule(rule)
	if parsedRule.Program == "" {
		return false
	}

	parsedCmd := ParseActualCommand(command, parsedRule)
	if parsedCmd.Program == "" {
		return false
	}

	// 1. 程序名必须相同
	if parsedRule.Program != parsedCmd.Program {
		return false
	}

	// 2. 规则中所有子命令必须出现（顺序无关）
	for _, sub := range parsedRule.SubCommands {
		if !matchSubCommand(sub, parsedCmd.SubCommands) {
			return false
		}
	}

	// 3. 规则中所有 flag 必须匹配
	for flag, ruleVal := range parsedRule.Flags {
		actualVal, ok := parsedCmd.Flags[flag]
		if !ok {
			return false
		}
		if ruleVal != "" && ruleVal != "*" && !matchGlobPattern(ruleVal, actualVal) {
			return false
		}
	}

	// 4. 无通配符时，不允许多余子命令和多余 flag
	if !parsedRule.Wildcard {
		if len(parsedCmd.SubCommands) > len(parsedRule.SubCommands) {
			return false
		}
		// 检查是否有规则中未定义的 flag
		for flag := range parsedCmd.Flags {
			if _, ok := parsedRule.Flags[flag]; !ok {
				return false
			}
		}
	}

	return true
}

// --- 辅助函数 ---

func tokenize(s string) []string {
	return strings.Fields(s)
}

func isFlag(s string) bool {
	return strings.HasPrefix(s, "-")
}

func matchSubCommand(pattern string, subs []string) bool {
	for _, sub := range subs {
		if matchGlobPattern(pattern, sub) {
			return true
		}
	}
	return false
}

// matchGlobPattern 使用 filepath.Match 做 glob 匹配
func matchGlobPattern(pattern, value string) bool {
	matched, err := filepath.Match(pattern, value)
	if err != nil {
		return pattern == value
	}
	return matched
}

// allSubCommandsAllowed 检查所有子命令是否都匹配 allow 规则
func allSubCommandsAllowed(subCmds []string, allowRules []string) bool {
	if len(allowRules) == 0 {
		return false
	}
	for _, cmd := range subCmds {
		matched := false
		for _, rule := range allowRules {
			if MatchCommandRule(rule, cmd) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// findHintRules 从 allow 规则中找同程序名的规则作为提示
func findHintRules(command string, allowRules []string) []string {
	tokens := tokenize(command)
	if len(tokens) == 0 {
		return nil
	}
	program := tokens[0]

	var hints []string
	for _, rule := range allowRules {
		ruleTokens := tokenize(rule)
		if len(ruleTokens) > 0 && ruleTokens[0] == program {
			hints = append(hints, rule)
		}
	}
	return hints
}

func formatDenyMessage(assetName, command, reason string, hints []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "命令执行被拒绝（%s）。\n命令: %s", reason, command)
	if len(hints) > 0 {
		sb.WriteString("\n\n该资产允许的相关命令格式：\n")
		for _, h := range hints {
			fmt.Fprintf(&sb, "- %s\n", h)
		}
		sb.WriteString("\n请按照上述格式调整命令后重试。")
	}
	return sb.String()
}

// --- 策略收集 ---

func collectPolicies(asset *asset_entity.Asset, groups []*group_entity.Group) []*asset_entity.CommandPolicy {
	var policies []*asset_entity.CommandPolicy
	if asset != nil {
		if p, err := asset.GetCommandPolicy(); err == nil && p != nil {
			policies = append(policies, p)
		}
	}
	for _, g := range groups {
		if p, err := g.GetCommandPolicy(); err == nil && p != nil {
			policies = append(policies, p)
		}
	}
	return policies
}

func collectDenyRules(policies []*asset_entity.CommandPolicy) []string {
	rules := make([]string, 0, len(policies))
	for _, p := range policies {
		rules = append(rules, p.DenyList...)
	}
	return rules
}

func collectAllowRules(policies []*asset_entity.CommandPolicy) []string {
	rules := make([]string, 0, len(policies))
	for _, p := range policies {
		rules = append(rules, p.AllowList...)
	}
	return rules
}

// resolveGroupChain 递归获取组链（组 → 父组 → ... → 根），最大深度 5
func resolveGroupChain(ctx context.Context, groupID int64) []*group_entity.Group {
	var chain []*group_entity.Group
	currentID := groupID
	for i := 0; i < 5 && currentID > 0; i++ {
		g, err := group_repo.Group().Find(ctx, currentID)
		if err != nil {
			break
		}
		chain = append(chain, g)
		currentID = g.ParentID
	}
	return chain
}
