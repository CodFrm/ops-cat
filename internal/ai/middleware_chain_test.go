package ai

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/opskat/opskat/internal/repository/audit_repo"
	"github.com/stretchr/testify/assert"
)

// TestMiddlewareChain_AuditCapturesGateDeny 验证注册顺序：
// audit 在外层（先注册），gate 在内层（后注册）。当 gate AbortWithDeny 时，
// audit 的 c.Next() 返回时 c.Output 已是 deny block —— 拒绝路径仍要落审计。
func TestMiddlewareChain_AuditCapturesGateDeny(t *testing.T) {
	mockRepo := &mockAuditRepo{}
	origRepo := audit_repo.Audit()
	audit_repo.RegisterAudit(mockRepo)
	t.Cleanup(func() {
		if origRepo != nil {
			audit_repo.RegisterAudit(origRepo)
		}
	})

	gate := NewLocalToolGate(func(_ context.Context, _ LocalToolApprovalRequest) ApprovalResponse {
		return ApprovalResponse{Decision: "deny"}
	})

	td := &agent.ToolDispatcher{
		Tools: []agent.Tool{stubLocalTool{name: "bash"}},
		Middleware: []agent.ToolHookEntry[agent.ToolMiddleware]{
			{Matcher: ".*", Fn: auditMiddleware},
			{Matcher: "^bash$", Fn: gate.Middleware()},
		},
	}

	res := td.Run(WithConversationID(context.Background(), 11), agent.DispatchInput{
		ToolName:  "bash",
		ToolUseID: "tu_chain_deny",
		Input:     map[string]any{"command": "rm -rf /"},
	})

	assert.True(t, res.Output != nil && res.Output.IsError, "gate 应该 deny 出 IsError 块")
	waitForAudit(t, mockRepo, 1)
	entry := mockRepo.logs[0]
	assert.Equal(t, "bash", entry.ToolName, "审计应记录 deny 路径下的工具调用")
	assert.Equal(t, 0, entry.Success)
}

// TestMiddlewareChain_RunBatch_StoreIsolation 验证并行 RunBatch 下两个 ToolContext
// 的 *CheckResult slot 互不干扰 —— 每个 dispatch 拿到自己的 closure-local slot，
// 不会出现 A 的决策落到 B 的审计里。
func TestMiddlewareChain_RunBatch_StoreIsolation(t *testing.T) {
	mockRepo := &mockAuditRepo{}
	origRepo := audit_repo.Audit()
	audit_repo.RegisterAudit(mockRepo)
	t.Cleanup(func() {
		if origRepo != nil {
			audit_repo.RegisterAudit(origRepo)
		}
	})

	var callCount atomic.Int32
	tool := &perCallDecisionTool{count: &callCount}

	td := &agent.ToolDispatcher{
		Tools: []agent.Tool{tool},
		Middleware: []agent.ToolHookEntry[agent.ToolMiddleware]{
			{Matcher: ".*", Fn: auditMiddleware},
		},
	}

	calls := []agent.DispatchInput{
		{ToolName: "per_call", ToolUseID: "tu_a", Input: map[string]any{"id": "A"}},
		{ToolName: "per_call", ToolUseID: "tu_b", Input: map[string]any{"id": "B"}},
		{ToolName: "per_call", ToolUseID: "tu_c", Input: map[string]any{"id": "C"}},
	}
	td.RunBatch(context.Background(), calls)

	waitForAudit(t, mockRepo, 3)

	// 每条审计的 MatchedPattern 应等于自己 dispatch 的 id（A→cat-A，B→cat-B，C→cat-C）。
	mockRepo.mu.Lock()
	defer mockRepo.mu.Unlock()
	got := map[string]string{}
	for _, e := range mockRepo.logs {
		got[e.ToolName+"-"+e.MatchedPattern] = e.MatchedPattern
	}
	// 三个不同 pattern 必须都出现 —— 任意一个 slot 串扰都会让其中一个 pattern 缺失或重复。
	assert.Contains(t, got, "per_call-cat-A")
	assert.Contains(t, got, "per_call-cat-B")
	assert.Contains(t, got, "per_call-cat-C")
}

type perCallDecisionTool struct {
	count *atomic.Int32
}

func (p *perCallDecisionTool) Name() string { return "per_call" }
func (p *perCallDecisionTool) Description() string {
	return "writes per-call decision via RecordDecision"
}
func (p *perCallDecisionTool) Schema() agent.Schema {
	return agent.Schema{Type: "object"}
}
func (p *perCallDecisionTool) Call(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
	p.count.Add(1)
	id, _ := in["id"].(string)
	RecordDecision(ctx, CheckResult{
		Decision:       Allow,
		DecisionSource: SourcePolicyAllow,
		MatchedPattern: "cat-" + id,
	})
	return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: "out-" + id}}}, nil
}
