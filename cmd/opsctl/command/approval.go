package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/approval"
	"github.com/opskat/opskat/internal/bootstrap"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
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

// requireApproval 检查命令策略 → DB Grant 匹配 → 桌面端审批。
// exec/sql/redis 类型支持离线模式：策略/Grant 匹配通过则放行，否则拒绝并提示允许的命令。
// 其他类型（cp/create/update）离线时直接报错。
func requireApproval(ctx context.Context, req approval.ApprovalRequest) (ApprovalResult, error) {
	// Stage 1: Auto-create session if none exists
	if req.SessionID == "" {
		id := uuid.New().String()
		if err := writeActiveSession(id); err != nil {
			logger.Default().Warn("write active session", zap.String("sessionID", id), zap.Error(err))
		}
		req.SessionID = id
	}

	// Stage 2: 统一权限检查（策略 + DB Grant）— 与 AI run_command 共用 CheckPermission
	var permResult ai.CheckResult
	var policyHints []string
	if req.AssetID > 0 && req.Command != "" {
		// 注入 sessionID 到 context，供 matchGrantPatterns 使用
		permCtx := ai.WithSessionID(ctx, req.SessionID)
		permResult = ai.CheckPermission(permCtx, req.Type, req.AssetID, req.Command)

		switch permResult.Decision {
		case ai.Allow:
			return ApprovalResult{
				Decision:       ai.Allow,
				DecisionSource: permResult.DecisionSource,
				MatchedPattern: permResult.MatchedPattern,
				SessionID:      req.SessionID,
			}, nil
		case ai.Deny:
			return ApprovalResult{
				Decision:       ai.Deny,
				DecisionSource: permResult.DecisionSource,
				MatchedPattern: permResult.MatchedPattern,
				SessionID:      req.SessionID,
			}, fmt.Errorf("command denied by policy: %s", permResult.Message)
		default: // NeedConfirm → fall through to desktop approval
			policyHints = permResult.HintRules
		}
	}

	// Stage 3: Connect to desktop app via Unix socket
	dataDir := bootstrap.AppDataDir()
	sockPath := approval.SocketPath(dataDir)

	authToken, err := bootstrap.ReadAuthToken(dataDir)
	if err != nil {
		logger.Default().Warn("read auth token", zap.Error(err))
	}

	resp, err := approval.RequestApprovalWithToken(sockPath, authToken, req)
	if err != nil {
		// 桌面端不在线
		switch req.Type {
		case "exec", "sql", "redis", "mongo":
			// 离线拒绝：给出允许的命令提示
			msg := formatOfflineDenyMessage(req.Type, req.Command, policyHints)
			return ApprovalResult{
				Decision:       ai.Deny,
				DecisionSource: ai.SourcePolicyDeny,
				SessionID:      req.SessionID,
			}, fmt.Errorf("%s", msg)
		default:
			// cp/create/update 等：保持原有报错
			return ApprovalResult{}, fmt.Errorf("desktop app is not running -- write operations require approval from the running desktop app\n(%v)", err)
		}
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
	if resp.ApproveGrant && req.SessionID != "" {
		if err := writeActiveSession(req.SessionID); err != nil {
			logger.Default().Warn("write active session", zap.String("sessionID", req.SessionID), zap.Error(err))
		}
	}

	return ApprovalResult{
		Decision:       ai.Allow,
		DecisionSource: ai.SourceUserAllow,
		SessionID:      req.SessionID,
	}, nil
}

// formatOfflineDenyMessage 构造离线拒绝的错误信息，包含允许的命令提示
func formatOfflineDenyMessage(reqType, command string, hints []string) string {
	var sb strings.Builder
	sb.WriteString("desktop app is not running, ")

	switch reqType {
	case "exec":
		sb.WriteString("command did not match any allowed policy")
	case "sql":
		sb.WriteString("SQL statement did not match any allowed policy")
	case "redis":
		sb.WriteString("Redis command did not match any allowed policy")
	case "mongo":
		sb.WriteString("MongoDB operation did not match any allowed policy")
	}

	if len(hints) > 0 {
		switch reqType {
		case "exec":
			sb.WriteString("\nAllowed commands for this asset:\n")
		case "sql":
			sb.WriteString("\nAllowed SQL types for this asset:\n")
		case "redis":
			sb.WriteString("\nAllowed Redis commands for this asset:\n")
		case "mongo":
			sb.WriteString("\nAllowed MongoDB operations for this asset:\n")
		}
		for _, h := range hints {
			fmt.Fprintf(&sb, "  - %s\n", h)
		}
	}

	sb.WriteString("\nPlease adjust the command or start the desktop app for approval.")
	return sb.String()
}
