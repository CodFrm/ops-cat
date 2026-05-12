package ai

import (
	"context"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/opskat/opskat/internal/repository/audit_repo"
	. "github.com/smartystreets/goconvey/convey"
)

// waitForAudit 阻塞等到 mockAuditRepo 收到至少 want 条记录，或超时。
// auditMiddleware 内是 fire-and-forget goroutine，必须显式等。
func waitForAudit(t *testing.T, m *mockAuditRepo, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		got := len(m.logs)
		m.mu.Unlock()
		if got >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("audit log 未在 2s 内写入 (期望 %d 条)", want)
}

// runAuditChain 端到端跑一遍 auditMiddleware：构造一个真 ToolDispatcher，
// 注册 [auditMiddleware, recordingTool]。recordingTool 在 Call 中按需调
// RecordDecision 写决策、按需返回 IsError 块或正常文本。
//
// 这样审计的入参（c.Input、c.Output、CheckResult slot）全部走生产路径填充，
// 不需要直接戳 ToolContext 私有字段。
func runAuditChain(t *testing.T, ctx context.Context, toolName, toolUseID string, input map[string]any, fillDecision *CheckResult, makeOutput func() (*agent.ToolResultBlock, error)) {
	t.Helper()
	tool := &recordingTool{
		name: toolName,
		fill: fillDecision,
		out:  makeOutput,
	}
	td := &agent.ToolDispatcher{
		Tools: []agent.Tool{tool},
		Middleware: []agent.ToolHookEntry[agent.ToolMiddleware]{
			{Matcher: ".*", Fn: auditMiddleware},
		},
	}
	td.Run(ctx, agent.DispatchInput{
		ToolName:  toolName,
		ToolUseID: toolUseID,
		Input:     input,
	})
}

// recordingTool 模拟一个工具：可选地写决策（模拟 handler 行为）+ 返回固定输出。
type recordingTool struct {
	name string
	fill *CheckResult
	out  func() (*agent.ToolResultBlock, error)
}

func (r *recordingTool) Name() string         { return r.name }
func (r *recordingTool) Description() string  { return "audit-test stub" }
func (r *recordingTool) Schema() agent.Schema { return agent.Schema{Type: "object"} }
func (r *recordingTool) Call(ctx context.Context, _ map[string]any) (*agent.ToolResultBlock, error) {
	if r.fill != nil {
		RecordDecision(ctx, *r.fill)
	}
	if r.out == nil {
		return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: "ok"}}}, nil
	}
	return r.out()
}

func TestAuditMiddleware_WritesAuditOnSuccess(t *testing.T) {
	Convey("成功路径：auditMiddleware 写出审计记录", t, func() {
		mockRepo := &mockAuditRepo{}
		origRepo := audit_repo.Audit()
		audit_repo.RegisterAudit(mockRepo)
		t.Cleanup(func() {
			if origRepo != nil {
				audit_repo.RegisterAudit(origRepo)
			}
		})

		ctx := WithAuditSource(context.Background(), "ai")
		ctx = WithConversationID(ctx, 99)
		runAuditChain(t, ctx, "run_command", "tu_ok_1",
			map[string]any{"asset_id": float64(7), "command": "uptime"},
			nil,
			func() (*agent.ToolResultBlock, error) {
				return &agent.ToolResultBlock{
					Content: []agent.ContentBlock{agent.TextBlock{Text: "ok-output"}},
				}, nil
			},
		)

		waitForAudit(t, mockRepo, 1)
		entry := mockRepo.logs[0]
		So(entry.ToolName, ShouldEqual, "run_command")
		So(entry.Source, ShouldEqual, "ai")
		So(entry.ConversationID, ShouldEqual, int64(99))
		So(entry.Command, ShouldEqual, "uptime")
		So(entry.Success, ShouldEqual, 1)
		So(entry.Error, ShouldEqual, "")
		So(entry.AssetID, ShouldEqual, int64(7))
		So(entry.Result, ShouldEqual, "ok-output")
	})
}

func TestAuditMiddleware_WritesAuditOnError(t *testing.T) {
	Convey("error 路径：IsError=true 的 ToolResultBlock 仍写出审计", t, func() {
		mockRepo := &mockAuditRepo{}
		origRepo := audit_repo.Audit()
		audit_repo.RegisterAudit(mockRepo)
		t.Cleanup(func() {
			if origRepo != nil {
				audit_repo.RegisterAudit(origRepo)
			}
		})

		runAuditChain(t, context.Background(), "exec_sql", "tu_err_1",
			map[string]any{"asset_id": float64(1), "sql": "SELECT 1"},
			nil,
			func() (*agent.ToolResultBlock, error) {
				return &agent.ToolResultBlock{
					IsError: true,
					Content: []agent.ContentBlock{agent.TextBlock{Text: "connection refused"}},
				}, nil
			},
		)

		waitForAudit(t, mockRepo, 1)
		entry := mockRepo.logs[0]
		So(entry.ToolName, ShouldEqual, "exec_sql")
		So(entry.Command, ShouldEqual, "SELECT 1")
		So(entry.Success, ShouldEqual, 0)
		So(entry.Error, ShouldEqual, "connection refused")
	})
}

func TestAuditMiddleware_CapturesRecordedDecision(t *testing.T) {
	Convey("handler 通过 RecordDecision 设置的决策能被 auditMiddleware 写到审计里", t, func() {
		mockRepo := &mockAuditRepo{}
		origRepo := audit_repo.Audit()
		audit_repo.RegisterAudit(mockRepo)
		t.Cleanup(func() {
			if origRepo != nil {
				audit_repo.RegisterAudit(origRepo)
			}
		})

		decision := &CheckResult{
			Decision:       Allow,
			DecisionSource: SourceGrantAllow,
			MatchedPattern: "uptime",
		}
		runAuditChain(t, context.Background(), "run_command", "tu_dec_1",
			map[string]any{"asset_id": float64(1), "command": "uptime"},
			decision,
			func() (*agent.ToolResultBlock, error) {
				return &agent.ToolResultBlock{
					Content: []agent.ContentBlock{agent.TextBlock{Text: "ok"}},
				}, nil
			},
		)

		waitForAudit(t, mockRepo, 1)
		entry := mockRepo.logs[0]
		So(entry.Decision, ShouldEqual, "allow")
		So(entry.DecisionSource, ShouldEqual, SourceGrantAllow)
		So(entry.MatchedPattern, ShouldEqual, "uptime")
	})
}
