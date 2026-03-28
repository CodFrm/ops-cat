package ai

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/grant_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/repository/grant_repo"
	"github.com/opskat/opskat/internal/service/asset_svc"
)

// CheckPermission 统一权限检查（策略 + DB Grant 匹配）。
// 不包含用户确认逻辑 — NeedConfirm 时由调用方处理。
// assetType: "ssh" | "database" | "redis" | "exec"（exec 等同于 ssh）
func CheckPermission(ctx context.Context, assetType string, assetID int64, command string) CheckResult {
	// opsctl 使用的类型名映射到内部类型
	switch assetType {
	case "exec":
		assetType = asset_entity.AssetTypeSSH
	case "sql":
		assetType = asset_entity.AssetTypeDatabase
	}

	switch assetType {
	case asset_entity.AssetTypeSSH:
		return checkSSHPermission(ctx, assetID, command)
	case asset_entity.AssetTypeDatabase:
		return checkDatabasePermission(ctx, assetID, command)
	case asset_entity.AssetTypeRedis:
		return checkRedisPermission(ctx, assetID, command)
	default:
		// 扩展类型：action 匹配
		return checkExtensionPermission(ctx, assetID, command)
	}
}

// --- SSH ---

func checkSSHPermission(ctx context.Context, assetID int64, command string) CheckResult {
	subCmds, err := ExtractSubCommands(command)
	if err != nil || len(subCmds) == 0 {
		subCmds = []string{command}
	}

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		logger.Default().Warn("get asset for permission check", zap.Int64("assetID", assetID), zap.Error(err))
	}
	var groups []*group_entity.Group
	if asset != nil && asset.GroupID > 0 {
		groups = resolveGroupChain(ctx, asset.GroupID)
	}

	// 策略检查
	allPolicies := collectPolicies(ctx, asset, groups)
	allDenyRules := collectDenyRules(allPolicies)
	allAllowRules := collectAllowRules(allPolicies)

	// deny list
	for _, cmd := range subCmds {
		for _, rule := range allDenyRules {
			if MatchCommandRule(rule, cmd) {
				assetName := ""
				if asset != nil {
					assetName = asset.Name
				}
				hints := findHintRules(cmd, allAllowRules)
				reason := policyMsg(ctx, "command blocked by policy", "命令被策略禁止执行")
				msg := formatDenyMessage(ctx, assetName, command, reason, hints)
				return CheckResult{Decision: Deny, Message: msg, HintRules: hints, DecisionSource: SourcePolicyDeny, MatchedPattern: rule}
			}
		}
	}

	// allow list
	if len(allAllowRules) > 0 {
		if ok, matched := allSubCommandsAllowed(subCmds, allAllowRules); ok {
			return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow, MatchedPattern: matched}
		}
	}

	// DB Grant 匹配
	if grantPattern := matchGrantPatterns(ctx, assetID, groups, subCmds); grantPattern != "" {
		return CheckResult{Decision: Allow, DecisionSource: SourceGrantAllow, MatchedPattern: grantPattern}
	}

	// 只返回与命令相似的 allow 规则作为提示
	var filteredHints []string
	seen := make(map[string]bool)
	for _, cmd := range subCmds {
		for _, h := range findHintRules(cmd, allAllowRules) {
			if !seen[h] {
				filteredHints = append(filteredHints, h)
				seen[h] = true
			}
		}
	}
	return CheckResult{Decision: NeedConfirm, HintRules: filteredHints}
}

// --- Database ---

func checkDatabasePermission(ctx context.Context, assetID int64, sqlText string) CheckResult {
	// 组通用策略
	groupResult := CheckGroupGenericPolicy(ctx, assetID, sqlText, MatchCommandRule)
	if groupResult.Decision == Deny {
		return groupResult
	}

	// SQL 分类 + 查询策略
	stmts, err := ClassifyStatements(sqlText)
	if err != nil {
		return CheckResult{Decision: Deny, Message: policyFmt(ctx, "SQL parse failed, execution denied: %v", "SQL 解析失败，拒绝执行: %v", err)}
	}

	asset, _ := resolveAssetPolicyChain(ctx, assetID)
	mergedPolicy := collectQueryPolicies(ctx, asset)
	result := CheckQueryPolicy(ctx, mergedPolicy, stmts)

	// 组通用 allow 优先于类型专用的 NeedConfirm
	if result.Decision == NeedConfirm && groupResult.Decision == Allow {
		return groupResult
	}

	if result.Decision != NeedConfirm {
		return result
	}

	// DB Grant 匹配
	if grantResult := matchGrantForAsset(ctx, assetID, sqlText); grantResult != nil {
		return *grantResult
	}

	// NeedConfirm：收集允许的 SQL 类型作为提示
	merged := mergeQueryPolicy(mergedPolicy, asset_entity.DefaultQueryPolicy())
	if len(merged.AllowTypes) > 0 {
		result.HintRules = merged.AllowTypes
	}
	return result
}

// --- Redis ---

func checkRedisPermission(ctx context.Context, assetID int64, command string) CheckResult {
	// 组通用策略
	groupResult := CheckGroupGenericPolicy(ctx, assetID, command, MatchRedisRule)
	if groupResult.Decision == Deny {
		return groupResult
	}

	// Redis 策略
	asset, _ := resolveAssetPolicyChain(ctx, assetID)
	mergedPolicy := collectRedisPolicies(ctx, asset)
	result := CheckRedisPolicy(ctx, mergedPolicy, command)

	// 组通用 allow 优先于类型专用的 NeedConfirm
	if result.Decision == NeedConfirm && groupResult.Decision == Allow {
		return groupResult
	}

	if result.Decision != NeedConfirm {
		return result
	}

	// DB Grant 匹配
	if grantResult := matchGrantForAsset(ctx, assetID, command); grantResult != nil {
		return *grantResult
	}

	// NeedConfirm：收集允许的 Redis 命令作为提示
	merged := mergeRedisPolicy(mergedPolicy, asset_entity.DefaultRedisPolicy())
	if len(merged.AllowList) > 0 {
		result.HintRules = merged.AllowList
	}
	return result
}

// --- Extension ---

func checkExtensionPermission(ctx context.Context, assetID int64, action string) CheckResult {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		logger.Default().Warn("get asset for extension permission check", zap.Int64("assetID", assetID), zap.Error(err))
		return CheckResult{Decision: NeedConfirm}
	}

	// 解析扩展策略
	var allowList, denyList []string
	if asset.CmdPolicy != "" {
		var raw struct {
			AllowList []string `json:"allow_list"`
			DenyList  []string `json:"deny_list"`
			Groups    []string `json:"groups"`
		}
		if json.Unmarshal([]byte(asset.CmdPolicy), &raw) == nil {
			allowList = raw.AllowList
			denyList = raw.DenyList
			// 解析引用的权限组
			if len(raw.Groups) > 0 {
				grpAllow, grpDeny := resolveExtensionGroups(ctx, raw.Groups)
				allowList = append(allowList, grpAllow...)
				denyList = append(denyList, grpDeny...)
			}
		}
	}

	assetName := ""
	if asset != nil {
		assetName = asset.Name
	}

	// deny 检查（精确 action 匹配 + 通配符）
	for _, d := range denyList {
		if d == action || d == "*" {
			reason := policyMsg(ctx, "action blocked by policy", "操作被策略禁止执行")
			msg := formatDenyMessage(ctx, assetName, action, reason, allowList)
			return CheckResult{Decision: Deny, Message: msg, HintRules: allowList, DecisionSource: SourcePolicyDeny, MatchedPattern: d}
		}
	}

	// allow 检查
	for _, a := range allowList {
		if a == action || a == "*" {
			return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow, MatchedPattern: a}
		}
	}

	// DB Grant 匹配
	var groups []*group_entity.Group
	if asset != nil && asset.GroupID > 0 {
		groups = resolveGroupChain(ctx, asset.GroupID)
	}
	if grantPattern := matchGrantPatterns(ctx, assetID, groups, []string{action}); grantPattern != "" {
		return CheckResult{Decision: Allow, DecisionSource: SourceGrantAllow, MatchedPattern: grantPattern}
	}

	return CheckResult{Decision: NeedConfirm, HintRules: allowList}
}

// --- Grant 匹配辅助 ---

// matchGrantForAsset 为 database/redis 类型做 DB Grant 匹配
func matchGrantForAsset(ctx context.Context, assetID int64, command string) *CheckResult {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil
	}
	var groups []*group_entity.Group
	if asset != nil && asset.GroupID > 0 {
		groups = resolveGroupChain(ctx, asset.GroupID)
	}
	if pattern := matchGrantPatterns(ctx, assetID, groups, []string{command}); pattern != "" {
		return &CheckResult{Decision: Allow, DecisionSource: SourceGrantAllow, MatchedPattern: pattern}
	}
	return nil
}

// --- SaveGrantPattern ---

// SaveGrantPattern 将命令模式保存为已批准的 GrantItem。
// 如果 sessionID 对应的 GrantSession 不存在，自动创建（状态: approved）。
func SaveGrantPattern(ctx context.Context, sessionID string, assetID int64, assetName string, command string) {
	if sessionID == "" || command == "" {
		return
	}
	repo := grant_repo.Grant()
	if repo == nil {
		return
	}

	// 确保 session 存在（create-if-not-exists）
	if _, err := repo.GetSession(ctx, sessionID); err != nil {
		session := &grant_entity.GrantSession{
			ID:         sessionID,
			Status:     grant_entity.GrantStatusApproved,
			Createtime: time.Now().Unix(),
		}
		if createErr := repo.CreateSession(ctx, session); createErr != nil {
			// 可能并发创建，忽略重复错误
			logger.Default().Debug("create grant session (may already exist)", zap.String("sessionID", sessionID), zap.Error(createErr))
		}
	}

	item := &grant_entity.GrantItem{
		GrantSessionID: sessionID,
		ToolName:       "exec",
		AssetID:        assetID,
		AssetName:      assetName,
		Command:        command,
		Createtime:     time.Now().Unix(),
	}
	if err := repo.CreateItems(ctx, []*grant_entity.GrantItem{item}); err != nil {
		logger.Default().Error("save grant pattern", zap.Error(err))
	}
}
