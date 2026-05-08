package aiagent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// PolicyChecker is the minimal interface the hook needs from
// *ai.CommandPolicyChecker. Allows tests to inject fakes.
type PolicyChecker interface {
	Check(ctx context.Context, assetID int64, command string) ai.CheckResult
}

// approvalRequester is the slice of ApprovalGateway the policy hook calls.
type approvalRequester interface {
	RequestSingle(ctx context.Context, convID int64, kind string,
		items []ai.ApprovalItem, agentRole string) ai.ApprovalResponse
}

// makePolicyHook constructs the PreToolUse hook. It:
//  1. extracts asset_id + command from ToolInput by tool name,
//  2. calls checker.Check,
//  3. on Allow: plants CheckResult in sidecar, returns nil (no-op),
//  4. on Deny: plants result, returns agent.Deny(reason),
//  5. on NeedConfirm: emits approval_request via gw, blocks on response,
//     converts response to Allow/Deny, persists grant pattern on allowAll.
func makePolicyHook(deps *Deps, sc *sidecar, gw approvalRequester, checker PolicyChecker) agent.HookFunc {
	return func(ctx context.Context, in agent.HookInput) (*agent.HookOutput, error) {
		convID := getConvID(ctx)
		agentRole := getAgentRole(ctx)

		assetID, command, kind, ok := extractAssetAndCommand(in.ToolName, in.ToolInput)
		if !ok {
			return nil, nil
		}

		result := checker.Check(ctx, assetID, command)
		sc.put(in.ToolCallID, &result)

		switch result.Decision {
		case ai.Allow:
			return nil, nil
		case ai.Deny:
			return agent.Deny(result.Message), nil
		}

		// NeedConfirm
		items := []ai.ApprovalItem{{
			Type: kind, AssetID: assetID, Command: command,
		}}
		resp := gw.RequestSingle(ctx, convID, kind, items, agentRole)
		switch resp.Decision {
		case "allow", "allowAll":
			result.Decision = ai.Allow
			result.DecisionSource = ai.SourceUserAllow
			sc.put(in.ToolCallID, &result)
			if resp.Decision == "allowAll" {
				saveGrantPatternFromResponse(ctx, convID, assetID, items[0], resp)
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

func saveGrantPatternFromResponse(ctx context.Context, convID, assetID int64,
	item ai.ApprovalItem, resp ai.ApprovalResponse) {
	pattern := item.Command
	if len(resp.EditedItems) > 0 {
		pattern = resp.EditedItems[0].Command
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return
	}
	// Task 1.13 will wire the real sessionID from context.
	// For now this is a no-op stub — SaveGrantPattern needs a sessionID
	// retrieved from ctx which is not yet plumbed.
	_ = pattern
	_ = convID
	_ = assetID
}
