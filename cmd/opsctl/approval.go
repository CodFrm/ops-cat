package main

import (
	"context"
	"fmt"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/repository/plan_repo"

	"github.com/google/uuid"
)

// ApprovalResult 审批结果，包含决策来源信息（用于审计）
type ApprovalResult struct {
	Decision       ai.Decision // Allow | Deny
	DecisionSource string      // ai.Source* 常量
	MatchedPattern string      // 匹配的规则或模式
	SessionID      string      // 会话 ID
}

// ToCheckResult 转换为 CheckResult（供 AuditWriter 使用）
func (ar ApprovalResult) ToCheckResult() *ai.CheckResult {
	return &ai.CheckResult{
		Decision:       ar.Decision,
		DecisionSource: ar.DecisionSource,
		MatchedPattern: ar.MatchedPattern,
	}
}

// requireApproval checks command policy first, then plan session, then session approval,
// then requests desktop app approval.
// Returns (result, nil) if decision made, (_, error) if communication failure.
func requireApproval(ctx context.Context, req approval.ApprovalRequest) (ApprovalResult, error) {
	// For exec commands, check allow/deny policy first
	if req.Type == "exec" && req.AssetID > 0 && req.Command != "" {
		result := ai.CheckPolicyOnly(ctx, req.AssetID, req.Command)
		switch result.Decision {
		case ai.Allow:
			return ApprovalResult{
				Decision:       ai.Allow,
				DecisionSource: result.DecisionSource,
				MatchedPattern: result.MatchedPattern,
				SessionID:      req.SessionID,
			}, nil
		case ai.Deny:
			return ApprovalResult{
				Decision:       ai.Deny,
				DecisionSource: result.DecisionSource,
				MatchedPattern: result.MatchedPattern,
				SessionID:      req.SessionID,
			}, fmt.Errorf("command denied by policy: %s", result.Message)
			// NeedConfirm -> fall through
		}
	}

	// Auto-create session if none exists
	if req.SessionID == "" {
		id := uuid.New().String()
		_ = writeActiveSession(id)
		req.SessionID = id
	}

	// Check plan items with pattern matching
	if req.SessionID != "" && req.Command != "" {
		items, err := plan_repo.Plan().ListApprovedItems(ctx, req.SessionID)
		if err == nil && len(items) > 0 {
			for _, item := range items {
				if item.AssetID != 0 && item.AssetID != req.AssetID {
					continue
				}
				if ai.MatchCommandRule(item.Command, req.Command) {
					return ApprovalResult{
						Decision:       ai.Allow,
						DecisionSource: ai.SourcePlanAllow,
						MatchedPattern: item.Command,
						SessionID:      req.SessionID,
					}, nil
				}
			}
		}
	}

	// Connect to desktop app via Unix socket
	dataDir := bootstrap.AppDataDir()
	sockPath := approval.SocketPath(dataDir)

	resp, err := approval.RequestApproval(sockPath, req)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("desktop app is not running -- write operations require approval from the running desktop app\n(%v)", err)
	}
	if !resp.Approved {
		reason := resp.Reason
		if reason == "" {
			reason = "denied"
		}
		return ApprovalResult{
			Decision:       ai.Deny,
			DecisionSource: ai.SourceUserDeny,
			SessionID:      req.SessionID,
		}, fmt.Errorf("operation denied: %s", reason)
	}

	// If the desktop app approved the entire session, persist it locally
	if resp.ApproveSession && req.SessionID != "" {
		_ = writeActiveSession(req.SessionID)
	}

	// 区分是 session 规则自动放行还是用户手动允许
	source := ai.SourceUserAllow
	if resp.Reason == "session_match" {
		source = ai.SourceSessionAllow
	}
	return ApprovalResult{
		Decision:       ai.Allow,
		DecisionSource: source,
		SessionID:      req.SessionID,
	}, nil
}
