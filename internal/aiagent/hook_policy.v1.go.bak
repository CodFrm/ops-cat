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

// kind 常量。前 7 个 = OpsKat 资产维度（走 ai.CheckPermission）；带 local_ 前缀的
// 是 cago built-in 工具（不走资产策略，靠 LocalGrantStore 做"始终放行"开关）。
const (
	kindLocalBash  = "local_bash"
	kindLocalWrite = "local_write"
	kindLocalEdit  = "local_edit"
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

// LocalGrantStore 抽象 cago built-in 工具的会话级"始终放行"开关。生产实现走
// local_tool_grant_repo（落库），测试可注入纯内存 fake。
//
// Save 用于 write / edit 在 allowAll 时持久化；Has 在 hook 入口判断是否短路放行。
// bash 永远不调 Save——bash 必须每次确认。
type LocalGrantStore interface {
	Has(ctx context.Context, sessionID, toolName string) bool
	Save(ctx context.Context, sessionID, toolName string)
}

// makePolicyHook constructs the PreToolUse hook. It:
//  1. extracts asset_id + command from ToolInput by tool name,
//  2. for OpsKat 资产工具：calls checkPerm（静态策略：allow/deny 列表 + DB grant 匹配，
//     不触发审批回调），
//  3. for cago built-in 工具 (bash/write/edit)：跳过 checkPerm，先看 grants 直接放行，
//     否则进入审批；bash 不开 allowAll，write/edit 开。
//  4. on Allow: plants CheckResult in sidecar, returns nil (no-op),
//  5. on Deny: plants result, returns agent.Deny(reason),
//  6. on NeedConfirm: emits approval_request via gw, blocks on response,
//     converts response to Allow/Deny, persists grant pattern on allowAll.
func makePolicyHook(sc *sidecar, gw approvalRequester, checkPerm CheckPermissionFunc, grants LocalGrantStore) agent.HookFunc {
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

		if isLocalKind(kind) {
			return handleLocalToolHook(ctx, sc, gw, grants, in, kind, command, agentRole, convID)
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

// handleLocalToolHook 处理 cago built-in 工具（bash / write / edit）的审批流。
// 不走资产策略——只看 LocalGrantStore + 弹卡。
//
// bash：永远 RequestSingle，allowAll 退化成 allow（不写 grants）。
// write / edit：先查 grants 命中即放行；否则 RequestSingle，allowAll 落 grants。
func handleLocalToolHook(ctx context.Context, sc *sidecar, gw approvalRequester,
	grants LocalGrantStore, in agent.HookInput, kind, command, agentRole string,
	convID int64) (*agent.HookOutput, error) {
	toolName := localKindToToolName(kind)
	sessionID := fmt.Sprintf("conv_%d", convID)

	// write / edit 命中已落库的会话级放行 → 直接 Allow，不弹卡。
	if grants != nil && kind != kindLocalBash {
		if grants.Has(ctx, sessionID, toolName) {
			result := ai.CheckResult{
				Decision:       ai.Allow,
				DecisionSource: ai.SourceUserAllow,
			}
			sc.put(in.ToolCallID, &result)
			return nil, nil
		}
	}

	items := []ai.ApprovalItem{{
		Type:    kind,
		Command: command,
	}}
	resp := gw.RequestSingle(ctx, convID, kind, items, agentRole)

	result := ai.CheckResult{}
	switch resp.Decision {
	case "allow", "allowAll":
		result.Decision = ai.Allow
		result.DecisionSource = ai.SourceUserAllow
		sc.put(in.ToolCallID, &result)
		// 只有 write / edit 才允许"本次会话起永久放行"。bash 不写 grants。
		if resp.Decision == "allowAll" && kind != kindLocalBash && grants != nil {
			grants.Save(ctx, sessionID, toolName)
			logger.Default().Debug("saved local-tool grant",
				zap.Int64("convID", convID), zap.String("tool", toolName))
		}
		return nil, nil
	default:
		result.Decision = ai.Deny
		result.DecisionSource = ai.SourceUserDeny
		sc.put(in.ToolCallID, &result)
		return agent.Deny("user denied"), nil
	}
}

// isLocalKind 判断 kind 是否是 cago built-in 工具的 kind。
func isLocalKind(kind string) bool {
	switch kind {
	case kindLocalBash, kindLocalWrite, kindLocalEdit:
		return true
	}
	return false
}

// localKindToToolName 把 ApprovalItem.Type 反推成 cago 工具名（bash / write / edit）。
// LocalGrantStore 的 toolName 一律存 cago 原生名，不带 local_ 前缀，方便日后直接 grep
// 出 "write" / "edit" 而不必关心 OpsKat 这一层封装。
func localKindToToolName(kind string) string {
	switch kind {
	case kindLocalBash:
		return "bash"
	case kindLocalWrite:
		return "write"
	case kindLocalEdit:
		return "edit"
	}
	return ""
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
// kind is the ApprovalItem.Type:
//   - 资产维度："exec","sql","redis","mongo","kafka","k8s","cp"（assetID > 0）
//   - 本地工具："local_bash","local_write","local_edit"（assetID == 0）
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

	// cago built-in 本地工具——只摘要、不走资产策略。
	// bash 的 command 可能很长（比如完整 shell 脚本），保持原样发给前端，让 UI 折叠展示。
	// write / edit 用 path 当 summary，让用户一眼看出动哪个文件；具体内容由前端读 ToolInput
	// 自行展示 diff/preview。
	case "bash":
		return 0, get("command"), kindLocalBash, true
	case "write":
		return 0, get("path"), kindLocalWrite, true
	case "edit":
		return 0, get("path"), kindLocalEdit, true
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
