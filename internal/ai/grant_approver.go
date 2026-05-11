package ai

import "context"

// GrantApprover 处理 request_permission 工具的"申请预批准"路径：
//   - 通过 per-conv ApprovalGateway 发 approval_request 事件 (kind="grant")，等用户审批；
//   - 用户允许时把（可能被编辑过的）模式持久化到 grant_repo（conv-scoped sessionID）；
//   - 拒绝/取消则直接返回 approved=false。
//
// 由 aiagent 层在 ConvHandle.Send/Edit/Resend 之前注入到 ctx；ai 层 handler
// (handleRequestGrant) 通过 GetGrantApprover(ctx) 取出。设计上替代 v1 的
// CommandPolicyChecker.grantRequestFunc 全局回调 —— 因为审批渠道是 per-conv 的
// (emitter/resolver 都绑 convID)，全局回调没法正确路由。
//
// items 由 handler 在 ai 层预先构建（含 AssetName 解析），gateway 仅负责传输 +
// 持久化，保持 aiagent 层与 asset_svc 解耦。
type GrantApprover interface {
	// RequestGrant 阻塞等待用户审批 items 中的模式。
	//   - approved=true 时 finalPatterns 是实际持久化的模式列表（用户可能编辑过）。
	//   - approved=false 时 finalPatterns 为空。
	// ctx 取消视作 deny。
	RequestGrant(ctx context.Context, items []ApprovalItem, reason string) (approved bool, finalPatterns []string)
}

type grantApproverKey struct{}

// WithGrantApprover 把 GrantApprover 挂到 ctx 上。
func WithGrantApprover(ctx context.Context, a GrantApprover) context.Context {
	return context.WithValue(ctx, grantApproverKey{}, a)
}

// GetGrantApprover 从 ctx 取出 GrantApprover；未注入时返回 nil。
func GetGrantApprover(ctx context.Context) GrantApprover {
	a, _ := ctx.Value(grantApproverKey{}).(GrantApprover)
	return a
}
