package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
)

// CheckPermissionFunc 是 policyHook 调用的静态策略检查。生产用 ai.CheckPermission；
// 测试可注入 fake，避免拉起 DB / asset 仓库。
//
// 这条 hook 必须使用「只查策略，不发审批」的检查 —— 真正的审批弹卡走 gw.RequestSingle，
// 用的是 NewSystem 闭包里绑定了正确 convID 的 emitter。曾经走 ai.CommandPolicyChecker.Check
// 会触发 legacy makeCommandConfirmFunc，那条路径靠 ai.GetConversationID(ctx)
// 取 convID，但 cago-only 迁移后 ai.WithConversationID 不再注入 → 总是 0 → 走
// fallback 全局 currentConversationID → 审批事件发到错的 ai:event:N，前端永远收不到。
type CheckPermissionFunc func(ctx context.Context, assetType string, assetID int64, command string) ai.CheckResult

// approvalRequester is the slice of ApprovalGateway the policy hook calls.
type approvalRequester interface {
	RequestSingle(ctx context.Context, convID int64, kind string,
		items []ai.ApprovalItem, agentRole string) ai.ApprovalResponse
}

// makePolicyHook constructs the PreToolUse hook. It:
//  1. extracts asset_id + command from ToolInput by tool name,
//  2. calls checkPerm（静态策略：allow/deny 列表 + DB grant 匹配，不触发审批回调）,
//  3. on Allow: plants CheckResult in sidecar, returns nil (no-op),
//  4. on Deny: plants result, returns agent.Deny(reason),
//  5. on NeedConfirm: emits approval_request via gw, blocks on response,
//     converts response to Allow/Deny, persists grant pattern on allowAll.
func makePolicyHook(sc *sidecar, gw approvalRequester, checkPerm CheckPermissionFunc) agent.HookFunc {
	if checkPerm == nil {
		checkPerm = ai.CheckPermission
	}
	return func(ctx context.Context, in agent.HookInput) (*agent.HookOutput, error) {
		convID := getConvID(ctx)
		agentRole := getAgentRole(ctx)

		assetID, command, kind, ok := extractAssetAndCommand(in.ToolName, in.ToolInput)
		if !ok {
			return nil, nil
		}

		assetType := assetTypeForKind(kind)
		result := checkPerm(ctx, assetType, assetID, command)
		sc.put(in.ToolCallID, &result)

		switch result.Decision {
		case ai.Allow:
			return nil, nil
		case ai.Deny:
			return agent.Deny(result.Message), nil
		}

		// NeedConfirm: 通过 closure-bound emitter 发 approval_request；
		// gw 内部 select 的 ctx 是 hook 的 ctx —— NewSystem 已用 OnWith(Timeout=-1) 关闭
		// 框架默认 5min 超时，所以这里只跟随用户 stop / app shutdown 取消。
		assetName := lookupAssetName(ctx, assetID)
		items := []ai.ApprovalItem{{
			Type:      kind,
			AssetID:   assetID,
			AssetName: assetName,
			Command:   command,
		}}
		resp := gw.RequestSingle(ctx, convID, kind, items, agentRole)
		switch resp.Decision {
		case "allow", "allowAll":
			result.Decision = ai.Allow
			result.DecisionSource = ai.SourceUserAllow
			sc.put(in.ToolCallID, &result)
			if resp.Decision == "allowAll" {
				saveGrantPatternsFromResponse(ctx, convID, assetID, assetName, assetType, command, resp)
			}
			return nil, nil
		default:
			result.Decision = ai.Deny
			result.DecisionSource = ai.SourceUserDeny
			sc.put(in.ToolCallID, &result)
			return agent.Deny("user denied"), nil
		}
	}
}

// assetTypeForKind 把 extractAssetAndCommand 返回的 kind（"exec"/"sql"/...）映射到
// ai.CheckPermission 期望的 asset type 常量。"cp"（upload/download_file）按 SSH 策略
// 走，与 legacy CommandPolicyChecker.Check 行为一致。
func assetTypeForKind(kind string) string {
	switch kind {
	case "exec", "cp":
		return asset_entity.AssetTypeSSH
	case "sql":
		return asset_entity.AssetTypeDatabase
	case "redis":
		return asset_entity.AssetTypeRedis
	case "mongo":
		return asset_entity.AssetTypeMongoDB
	case "kafka":
		return asset_entity.AssetTypeKafka
	case "k8s":
		return asset_entity.AssetTypeK8s
	}
	return asset_entity.AssetTypeSSH
}

// lookupAssetName 取审批卡上展示用的资产名。失败 / repo 未初始化（单测）时返回空串 ——
// 资产名只是审批 UI 展示用，缺失不影响策略决策。
//
// 通过 var 而不是直接调 asset_svc.Asset().Get：方便单测注入 fake，避免拉起整个 DB 栈。
var lookupAssetName = func(ctx context.Context, assetID int64) string {
	if assetID <= 0 {
		return ""
	}
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil || asset == nil {
		return ""
	}
	return asset.Name
}

// extractAssetAndCommand returns (assetID, commandSummary, kind, isPolicyGated).
// kind is the ApprovalItem.Type ("exec","sql","redis","mongo","kafka","k8s","cp").
func extractAssetAndCommand(toolName string, raw json.RawMessage) (int64, string, string, bool) {
	var args map[string]any
	_ = json.Unmarshal(raw, &args)
	get := func(k string) string { v, _ := args[k].(string); return v }
	getNum := func(k string) int64 {
		switch v := args[k].(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		}
		return 0
	}

	switch toolName {
	case "run_command":
		return getNum("asset_id"), get("command"), "exec", true
	case "exec_sql":
		return getNum("asset_id"), get("sql"), "sql", true
	case "exec_redis":
		return getNum("asset_id"), get("command"), "redis", true
	case "exec_mongo":
		return getNum("asset_id"), get("operation"), "mongo", true
	case "exec_k8s":
		return getNum("asset_id"), get("command"), "k8s", true
	case "upload_file":
		return getNum("asset_id"), "upload " + get("local_path") + " → " + get("remote_path"), "cp", true
	case "download_file":
		return getNum("asset_id"), "download " + get("remote_path") + " → " + get("local_path"), "cp", true
	case "kafka_cluster", "kafka_topic", "kafka_consumer_group", "kafka_acl",
		"kafka_schema", "kafka_connect", "kafka_message":
		return getNum("asset_id"), get("operation") + ":" + get("topic"), "kafka", true
	}
	return 0, "", "", false
}

// saveGrantPatternsFromResponse 在用户点 "remember & allow" 时把 pattern 落库。
// 与 ai.CommandPolicyChecker.handleConfirm 的 allowAll 分支保持等价行为：
//   - 优先使用用户编辑过的 EditedItems；
//   - 没编辑且是 SSH 类，则按子命令拆分（grep / awk / ... 各自落一个）；
//   - 其它类型按整条命令落。
//
// sessionID 直接构造为 conv_<convID>，跟 RequestGrant 里 grant 流程的 sessionID 命名一致。
// 历史上这条 sessionID 依赖 ai.WithSessionID(ctx) 注入，但 cago-only 迁移后注入断了
// 导致 SaveGrantPattern 静默 no-op，allowAll 看起来"成功"但下次还是要再点一次。
func saveGrantPatternsFromResponse(ctx context.Context, convID, assetID int64,
	assetName, assetType, command string, resp ai.ApprovalResponse) {
	sessionID := fmt.Sprintf("conv_%d", convID)

	var patterns []string
	if len(resp.EditedItems) > 0 {
		for _, item := range resp.EditedItems {
			cmd := strings.TrimSpace(item.Command)
			if cmd != "" {
				patterns = append(patterns, cmd)
			}
		}
	}
	if len(patterns) == 0 {
		if assetType == asset_entity.AssetTypeSSH {
			subCmds, _ := ai.ExtractSubCommands(command)
			if len(subCmds) == 0 {
				subCmds = []string{command}
			}
			patterns = subCmds
		} else {
			patterns = []string{command}
		}
	}

	for _, cmd := range patterns {
		ai.SaveGrantPattern(ctx, sessionID, assetID, assetName, cmd)
	}
	if len(patterns) > 0 {
		logger.Default().Debug("saved AI grant patterns",
			zap.Int64("convID", convID), zap.Int64("assetID", assetID),
			zap.Strings("patterns", patterns))
	}
}
