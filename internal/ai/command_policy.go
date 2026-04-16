package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/grant_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/repository/grant_repo"
	"github.com/opskat/opskat/internal/repository/group_repo"
	"github.com/opskat/opskat/internal/service/asset_svc"
)

// Decision 权限判定结果
type Decision int

const (
	Allow       Decision = iota // 直接放行
	Deny                        // 拒绝
	NeedConfirm                 // 需要用户确认
)

// 决策来源常量
const (
	SourcePolicyAllow = "policy_allow" // 命令策略白名单放行
	SourcePolicyDeny  = "policy_deny"  // 命令策略黑名单拒绝
	SourceUserAllow   = "user_allow"   // 用户手动允许
	SourceUserDeny    = "user_deny"    // 用户手动拒绝
	SourceGrantAllow  = "grant_allow"  // Grant 预批准匹配放行
	SourceGrantDeny   = "grant_deny"   // Grant 权限申请被拒绝
)

// CheckResult 权限检查结果
type CheckResult struct {
	Decision       Decision
	Message        string   // 返回给 AI 的消息
	HintRules      []string // 拒绝时的允许规则提示
	DecisionSource string   // 决策来源（SourcePolicyAllow 等常量）
	MatchedPattern string   // 匹配的命令模式
}

// DecisionString 返回决策的字符串表示（用于审计日志存储）
func (r CheckResult) DecisionString() string {
	switch r.Decision {
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	default:
		return ""
	}
}

// --- CheckResult context（供 AuditingExecutor 读取决策信息）---

type checkResultKey struct{}

// withCheckResult 注入 CheckResult 占位指针（由 AuditingExecutor 调用）
func withCheckResult(ctx context.Context, r *CheckResult) context.Context {
	return context.WithValue(ctx, checkResultKey{}, r)
}

// setCheckResult 在工具 handler 中设置决策结果
func setCheckResult(ctx context.Context, result CheckResult) {
	if r, ok := ctx.Value(checkResultKey{}).(*CheckResult); ok && r != nil {
		*r = result
	}
}

// CommandConfirmFunc 命令确认回调，发送审批请求并阻塞等待前端响应
// ctx 携带会话 ID 等上下文（通过 GetConversationID 获取）
// kind: "single", "batch", "grant"
// items: 审批项列表
// agentRole: 子 agent 角色（可为空）
// 返回 ApprovalResponse
type CommandConfirmFunc func(ctx context.Context, kind string, items []ApprovalItem, agentRole string) ApprovalResponse

// GrantRequestFunc Grant 审批回调，创建 grant 并等待用户审批
// ctx 携带会话 ID 等上下文（通过 GetConversationID 获取）
// items 为多资产的审批条目列表，用户可能在审批弹窗中编辑
// 返回 (approved, 用户编辑后的 patterns)
type GrantRequestFunc func(ctx context.Context, items []ApprovalItem, reason string) (approved bool, finalPatterns []string)

// ApprovedPattern 会话级已批准的命令模式
type ApprovedPattern struct {
	AssetID int64  // 0 表示所有资产
	Pattern string // 命令模式，支持 * 通配，复用 MatchCommandRule 匹配
}

// Match 检查实际命令是否匹配此模式
func (p *ApprovedPattern) Match(assetID int64, command string) bool {
	if p.AssetID != 0 && p.AssetID != assetID {
		return false
	}
	return MatchCommandRule(p.Pattern, command)
}

// CommandPolicyChecker 命令权限检查器，通过 context 注入到两条执行路径
type CommandPolicyChecker struct {
	confirmFunc      CommandConfirmFunc
	grantRequestFunc GrantRequestFunc
}

// NewCommandPolicyChecker 创建权限检查器
func NewCommandPolicyChecker(confirmFunc CommandConfirmFunc) *CommandPolicyChecker {
	return &CommandPolicyChecker{
		confirmFunc: confirmFunc,
	}
}

// ConfirmFunc 返回确认回调（供 batch_command 等直接调用）
func (c *CommandPolicyChecker) ConfirmFunc() CommandConfirmFunc {
	return c.confirmFunc
}

// SetGrantRequestFunc 设置 Grant 审批回调
func (c *CommandPolicyChecker) SetGrantRequestFunc(fn GrantRequestFunc) {
	c.grantRequestFunc = fn
}

// SubmitGrant 提交 grant 审批请求（request_permission 工具调用）
func (c *CommandPolicyChecker) SubmitGrant(ctx context.Context, assetID int64, patterns []string, reason string) CheckResult {
	if c.grantRequestFunc == nil {
		return CheckResult{Decision: Deny, Message: policyMsg(ctx, "no grant approval mechanism", "无 Grant 审批机制")}
	}

	assetName := ""
	if assetID > 0 {
		asset, err := asset_svc.Asset().Get(ctx, assetID)
		if err != nil {
			logger.Default().Warn("get asset for grant submission", zap.Int64("assetID", assetID), zap.Error(err))
		}
		if asset != nil {
			assetName = asset.Name
		}
	}

	items := make([]ApprovalItem, 0, len(patterns))
	for _, p := range patterns {
		items = append(items, ApprovalItem{
			Type:      "grant",
			AssetID:   assetID,
			AssetName: assetName,
			Command:   p,
			Detail:    reason,
		})
	}

	approved, finalPatterns := c.grantRequestFunc(ctx, items, reason)
	if !approved {
		return CheckResult{Decision: Deny, Message: policyMsg(ctx, "USER DENIED: The user has denied the grant approval request. Stop the current task immediately.", "用户拒绝：用户已拒绝 Grant 审批请求。请立即停止当前任务。"), DecisionSource: SourceGrantDeny, MatchedPattern: strings.Join(patterns, "; ")}
	}

	return CheckResult{Decision: Allow, Message: policyFmt(ctx, "grant approved, %d patterns", "Grant 已批准，共 %d 条模式", len(finalPatterns)), DecisionSource: SourceGrantAllow, MatchedPattern: strings.Join(finalPatterns, "; ")}
}

// GrantItem represents a single asset's patterns in a multi-asset grant request.
type GrantItem struct {
	AssetID  int64
	Patterns []string
}

// SubmitGrantMulti 提交多资产 grant 审批请求
func (c *CommandPolicyChecker) SubmitGrantMulti(ctx context.Context, items []GrantItem, reason string) CheckResult {
	if c.grantRequestFunc == nil {
		return CheckResult{Decision: Deny, Message: policyMsg(ctx, "no grant approval mechanism", "无 Grant 审批机制")}
	}

	approvalItems := make([]ApprovalItem, 0)
	var allPatterns []string
	for _, item := range items {
		assetName := ""
		if item.AssetID > 0 {
			asset, err := asset_svc.Asset().Get(ctx, item.AssetID)
			if err != nil {
				logger.Default().Warn("get asset for grant submission", zap.Int64("assetID", item.AssetID), zap.Error(err))
			}
			if asset != nil {
				assetName = asset.Name
			}
		}
		for _, p := range item.Patterns {
			approvalItems = append(approvalItems, ApprovalItem{
				Type:      "grant",
				AssetID:   item.AssetID,
				AssetName: assetName,
				Command:   p,
				Detail:    reason,
			})
			allPatterns = append(allPatterns, p)
		}
	}

	approved, finalPatterns := c.grantRequestFunc(ctx, approvalItems, reason)
	if !approved {
		return CheckResult{Decision: Deny, Message: policyMsg(ctx, "USER DENIED: The user has denied the grant approval request. Stop the current task immediately.", "用户拒绝：用户已拒绝 Grant 审批请求。请立即停止当前任务。"), DecisionSource: SourceGrantDeny, MatchedPattern: strings.Join(allPatterns, "; ")}
	}

	return CheckResult{Decision: Allow, Message: policyFmt(ctx, "grant approved, %d patterns", "Grant 已批准，共 %d 条模式", len(finalPatterns)), DecisionSource: SourceGrantAllow, MatchedPattern: strings.Join(finalPatterns, "; ")}
}

// matchGrantPatterns 从 DB 中查找已批准 grant 的 items，用通配匹配命令
// 返回首个匹配的 pattern，空字符串表示未匹配
// groups 为资产所属的组链（组 → 父组 → ... → 根）
func matchGrantPatterns(ctx context.Context, assetID int64, groups []*group_entity.Group, subCmds []string) string {
	sessionID := GetSessionID(ctx)
	if sessionID == "" {
		return ""
	}
	repo := grant_repo.Grant()
	if repo == nil {
		return ""
	}
	items, err := repo.ListApprovedItems(ctx, sessionID)
	if err != nil || len(items) == 0 {
		return ""
	}

	// 构建资产所属的组 ID 集合，用于匹配 group 级 grant item
	groupIDs := make(map[int64]bool, len(groups))
	for _, g := range groups {
		groupIDs[g.ID] = true
	}

	// 所有子命令都必须匹配某个 grant item
	var firstPattern string
	for _, cmd := range subCmds {
		matched := false
		for _, item := range items {
			if !grantItemMatchesTarget(item, assetID, groupIDs) {
				continue
			}
			if MatchCommandRule(item.Command, cmd) {
				matched = true
				if firstPattern == "" {
					firstPattern = item.Command
				}
				break
			}
		}
		if !matched {
			return ""
		}
	}
	return firstPattern
}

// grantItemMatchesTarget 检查 grant item 是否匹配目标资产
// AssetID=0 且 GroupID=0 → 匹配所有资产
// AssetID>0 → 精确匹配资产
// GroupID>0 → 匹配组内资产（检查资产所属组链）
func grantItemMatchesTarget(item *grant_entity.GrantItem, assetID int64, groupIDs map[int64]bool) bool {
	if item.AssetID != 0 {
		return item.AssetID == assetID
	}
	if item.GroupID != 0 {
		return groupIDs[item.GroupID]
	}
	// AssetID=0 且 GroupID=0，匹配所有资产
	return true
}

// Reset 重置会话级白名单（已迁移到 DB Grant，无需内存清理）
func (c *CommandPolicyChecker) Reset() {
}

// Check 检查命令是否允许执行
func (c *CommandPolicyChecker) Check(ctx context.Context, assetID int64, command string) CheckResult {
	result := CheckPermission(ctx, asset_entity.AssetTypeSSH, assetID, command)
	if result.Decision != NeedConfirm {
		return result
	}
	return c.handleConfirm(ctx, assetID, asset_entity.AssetTypeSSH, command)
}

// CheckPolicyOnly 只检查 allow/deny 列表 + DB Grant 匹配，不触发确认回调。
// 向后兼容包装器，内部委托 CheckPermission。
func CheckPolicyOnly(ctx context.Context, assetID int64, command string) CheckResult {
	return CheckPermission(ctx, asset_entity.AssetTypeSSH, assetID, command)
}

// CheckSQLPolicyForOpsctl 检查 SQL 策略，向后兼容包装器，内部委托 CheckPermission。
func CheckSQLPolicyForOpsctl(ctx context.Context, assetID int64, sqlText string) CheckResult {
	return CheckPermission(ctx, asset_entity.AssetTypeDatabase, assetID, sqlText)
}

// CheckRedisPolicyForOpsctl 检查 Redis 策略，向后兼容包装器，内部委托 CheckPermission。
func CheckRedisPolicyForOpsctl(ctx context.Context, assetID int64, command string) CheckResult {
	return CheckPermission(ctx, asset_entity.AssetTypeRedis, assetID, command)
}

// CheckForAsset 按资产类型分发权限检查
func (c *CommandPolicyChecker) CheckForAsset(ctx context.Context, assetID int64, assetType, command string) CheckResult {
	result := CheckPermission(ctx, assetType, assetID, command)
	if result.Decision != NeedConfirm {
		return result
	}
	return c.handleConfirm(ctx, assetID, assetType, command)
}

// handleConfirm 处理需要用户确认的情况
func (c *CommandPolicyChecker) handleConfirm(ctx context.Context, assetID int64, assetType, command string) CheckResult {
	if c.confirmFunc == nil {
		return CheckResult{Decision: Deny, Message: policyMsg(ctx, "command not authorized and no confirmation mechanism", "命令未授权且无确认机制"), DecisionSource: SourcePolicyDeny}
	}

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		logger.Default().Warn("get asset for confirm", zap.Int64("assetID", assetID), zap.Error(err))
	}
	assetName := ""
	if asset != nil {
		assetName = asset.Name
	}

	// 映射资产类型到审批项类型
	approvalType := "exec"
	switch assetType {
	case asset_entity.AssetTypeDatabase:
		approvalType = "sql"
	case asset_entity.AssetTypeRedis:
		approvalType = "redis"
	case asset_entity.AssetTypeMongoDB:
		approvalType = "mongo"
	}

	items := []ApprovalItem{{
		Type:      approvalType,
		AssetID:   assetID,
		AssetName: assetName,
		Command:   command,
	}}
	agentRole := GetAgentRole(ctx)

	resp := c.confirmFunc(ctx, "single", items, agentRole)

	if resp.Decision == "deny" {
		return CheckResult{Decision: Deny, Message: policyFmt(ctx, "USER DENIED: The user has denied execution of command: %s. Stop the current task immediately.", "用户拒绝：用户已拒绝执行命令: %s。请立即停止当前任务。", command), DecisionSource: SourceUserDeny}
	}
	if resp.Decision == "allowAll" {
		sessionID := GetSessionID(ctx)
		var patterns []string
		// 优先使用用户编辑后的模式（支持 * 通配符）
		if len(resp.EditedItems) > 0 {
			for _, item := range resp.EditedItems {
				cmd := strings.TrimSpace(item.Command)
				if cmd != "" {
					patterns = append(patterns, cmd)
				}
			}
		}
		// 无编辑项时回退：SSH 解析子命令，其他类型直接使用原始命令
		if len(patterns) == 0 {
			if assetType == asset_entity.AssetTypeSSH {
				subCmds, _ := ExtractSubCommands(command)
				if len(subCmds) == 0 {
					subCmds = []string{command}
				}
				patterns = subCmds
			} else {
				patterns = []string{command}
			}
		}
		for _, cmd := range patterns {
			SaveGrantPattern(ctx, sessionID, assetID, assetName, cmd)
		}
		writeGrantSubmitAudit(ctx, assetID, assetName, patterns)
	}
	return CheckResult{Decision: Allow, DecisionSource: SourceUserAllow}
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

func formatDenyMessage(_ context.Context, assetName, command, reason string, hints []string) string {
	var sb strings.Builder
	if assetName != "" {
		fmt.Fprintf(&sb, "Command denied (%s).\nAsset: %s\nCommand: %s", reason, assetName, command)
	} else {
		fmt.Fprintf(&sb, "Command denied (%s).\nCommand: %s", reason, command)
	}
	if len(hints) > 0 {
		sb.WriteString("\n\nAllowed command patterns for this asset:\n")
		for _, h := range hints {
			fmt.Fprintf(&sb, "- %s\n", h)
		}
		sb.WriteString("\nPlease adjust the command accordingly and retry.")
	}
	return sb.String()
}

// --- 策略收集 ---

// resolveAssetPolicyChain 解析资产及其组链，返回按优先级排序的策略持有者列表（资产优先）
func resolveAssetPolicyChain(ctx context.Context, assetID int64) (*asset_entity.Asset, []policy.Holder) {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		logger.Default().Warn("get asset for policy check", zap.Int64("assetID", assetID), zap.Error(err))
		return nil, nil
	}
	var holders []policy.Holder
	holders = append(holders, asset)
	if asset.GroupID > 0 {
		for _, g := range resolveGroupChain(ctx, asset.GroupID) {
			holders = append(holders, g)
		}
	}
	return asset, holders
}

// collectPoliciesFromChain 从策略链中收集指定类型的策略
func collectPoliciesFromChain[T any](holders []policy.Holder, getter func(policy.Holder) (*T, error)) []*T {
	var policies []*T
	for _, h := range holders {
		if p, err := getter(h); err == nil && p != nil {
			policies = append(policies, p)
		}
	}
	return policies
}

func collectPolicies(ctx context.Context, asset *asset_entity.Asset, groups []*group_entity.Group) []*asset_entity.CommandPolicy {
	var holders []policy.Holder
	if asset != nil {
		holders = append(holders, asset)
	}
	for _, g := range groups {
		holders = append(holders, g)
	}
	policies := collectPoliciesFromChain(holders, func(h policy.Holder) (*asset_entity.CommandPolicy, error) {
		return h.GetCommandPolicy()
	})
	// 解析每个策略引用的权限组，将组的规则合并进来
	for _, p := range policies {
		if len(p.Groups) > 0 {
			grpAllow, grpDeny := resolveCommandGroups(ctx, p.Groups)
			p.AllowList = append(p.AllowList, grpAllow...)
			p.DenyList = append(p.DenyList, grpDeny...)
		}
	}
	return policies
}

// collectQueryPolicies 收集资产 + 组链的 SQL 权限策略并合并
func collectQueryPolicies(ctx context.Context, asset *asset_entity.Asset) *asset_entity.QueryPolicy {
	var holders []policy.Holder
	if asset != nil {
		holders = append(holders, asset)
		if asset.GroupID > 0 {
			for _, g := range resolveGroupChain(ctx, asset.GroupID) {
				holders = append(holders, g)
			}
		}
	}
	policies := collectPoliciesFromChain(holders, func(h policy.Holder) (*asset_entity.QueryPolicy, error) {
		return h.GetQueryPolicy()
	})
	if len(policies) == 0 {
		return nil
	}
	// 解析引用的权限组
	for _, p := range policies {
		if len(p.Groups) > 0 {
			grpAllowTypes, grpDenyTypes, grpDenyFlags := resolveQueryGroups(ctx, p.Groups)
			p.AllowTypes = append(p.AllowTypes, grpAllowTypes...)
			p.DenyTypes = append(p.DenyTypes, grpDenyTypes...)
			p.DenyFlags = append(p.DenyFlags, grpDenyFlags...)
		}
	}
	// 合并：allow_types 取第一个非空（资产优先），deny_types/deny_flags 全部合并
	merged := &asset_entity.QueryPolicy{}
	for _, p := range policies {
		if len(merged.AllowTypes) == 0 && len(p.AllowTypes) > 0 {
			merged.AllowTypes = p.AllowTypes
		}
		merged.DenyTypes = appendUnique(merged.DenyTypes, p.DenyTypes...)
		merged.DenyFlags = appendUnique(merged.DenyFlags, p.DenyFlags...)
	}
	return merged
}

// collectRedisPolicies 收集资产 + 组链的 Redis 权限策略并合并
func collectRedisPolicies(ctx context.Context, asset *asset_entity.Asset) *asset_entity.RedisPolicy {
	var holders []policy.Holder
	if asset != nil {
		holders = append(holders, asset)
		if asset.GroupID > 0 {
			for _, g := range resolveGroupChain(ctx, asset.GroupID) {
				holders = append(holders, g)
			}
		}
	}
	policies := collectPoliciesFromChain(holders, func(h policy.Holder) (*asset_entity.RedisPolicy, error) {
		return h.GetRedisPolicy()
	})
	if len(policies) == 0 {
		return nil
	}
	// 解析引用的权限组
	for _, p := range policies {
		if len(p.Groups) > 0 {
			grpAllow, grpDeny := resolveRedisGroups(ctx, p.Groups)
			p.AllowList = append(p.AllowList, grpAllow...)
			p.DenyList = append(p.DenyList, grpDeny...)
		}
	}
	// 合并：allow_list 取第一个非空（资产优先），deny_list 全部合并
	merged := &asset_entity.RedisPolicy{}
	for _, p := range policies {
		if len(merged.AllowList) == 0 && len(p.AllowList) > 0 {
			merged.AllowList = p.AllowList
		}
		merged.DenyList = appendUnique(merged.DenyList, p.DenyList...)
	}
	return merged
}

// collectMongoDBPolicies 收集资产 + 组链的 MongoDB 权限策略并合并
func collectMongoDBPolicies(ctx context.Context, asset *asset_entity.Asset) *asset_entity.MongoPolicy {
	var holders []policy.Holder
	if asset != nil {
		holders = append(holders, asset)
		if asset.GroupID > 0 {
			for _, g := range resolveGroupChain(ctx, asset.GroupID) {
				holders = append(holders, g)
			}
		}
	}
	policies := collectPoliciesFromChain(holders, func(h policy.Holder) (*asset_entity.MongoPolicy, error) {
		return h.GetMongoPolicy()
	})
	if len(policies) == 0 {
		return nil
	}
	// 解析引用的权限组
	for _, p := range policies {
		if len(p.Groups) > 0 {
			grpAllowTypes, grpDenyTypes := resolveMongoGroups(ctx, p.Groups)
			p.AllowTypes = append(p.AllowTypes, grpAllowTypes...)
			p.DenyTypes = append(p.DenyTypes, grpDenyTypes...)
		}
	}
	// 合并：allow_types 取第一个非空（资产优先），deny_types 全部合并
	merged := &asset_entity.MongoPolicy{}
	for _, p := range policies {
		if len(merged.AllowTypes) == 0 && len(p.AllowTypes) > 0 {
			merged.AllowTypes = p.AllowTypes
		}
		merged.DenyTypes = appendUnique(merged.DenyTypes, p.DenyTypes...)
	}
	return merged
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

// --- agentRole context ---

type agentRoleKeyType struct{}

// WithAgentRole 注入 agent role（Sub Agent 用）
func WithAgentRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, agentRoleKeyType{}, role)
}

// GetAgentRole 获取 agent role
func GetAgentRole(ctx context.Context) string {
	if v, ok := ctx.Value(agentRoleKeyType{}).(string); ok {
		return v
	}
	return ""
}
