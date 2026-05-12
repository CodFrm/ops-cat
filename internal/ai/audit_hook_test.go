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
// auditPostHook 内是 fire-and-forget goroutine，必须显式等。
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

// runPreHookAndDecision 模拟 cago dispatcher 调 PreToolUseHook + handler 内部 setCheckResult。
// 调用方接着调 auditPostHook 完成审计写入闭环。
func runPreHookAndDecision(t *testing.T, toolName, toolUseID string, input map[string]any, fillDecision *CheckResult) {
	t.Helper()
	_, err := attachCheckResultHook(context.Background(), &agent.PreToolUseInput{
		ToolName: toolName, ToolUseID: toolUseID, Input: input,
	})
	if err != nil {
		t.Fatalf("attachCheckResultHook: %v", err)
	}
	if fillDecision != nil {
		ctx := agent.WithToolUseID(context.Background(), toolUseID)
		setCheckResult(ctx, *fillDecision)
	}
}

func TestAuditPostHook_WritesAuditOnSuccess(t *testing.T) {
	Convey("成功路径：auditPostHook 写出审计记录", t, func() {
		mockRepo := &mockAuditRepo{}
		origRepo := audit_repo.Audit()
		audit_repo.RegisterAudit(mockRepo)
		t.Cleanup(func() {
			if origRepo != nil {
				audit_repo.RegisterAudit(origRepo)
			}
		})

		toolUseID := "tu_ok_1"
		input := map[string]any{"asset_id": float64(7), "command": "uptime"}
		output := &agent.ToolResultBlock{
			Content: []agent.ContentBlock{agent.TextBlock{Text: "ok-output"}},
		}
		runPreHookAndDecision(t, "run_command", toolUseID, input, nil)

		ctx := WithAuditSource(context.Background(), "ai")
		ctx = WithConversationID(ctx, 99)
		_, err := auditPostHook(ctx, &agent.PostToolUseInput{
			ToolName: "run_command", ToolUseID: toolUseID, Input: input, Output: output,
		})
		So(err, ShouldBeNil)

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

func TestAuditPostHook_WritesAuditOnError(t *testing.T) {
	Convey("error 路径：IsError=true 的 ToolResultBlock 仍写出审计", t, func() {
		mockRepo := &mockAuditRepo{}
		origRepo := audit_repo.Audit()
		audit_repo.RegisterAudit(mockRepo)
		t.Cleanup(func() {
			if origRepo != nil {
				audit_repo.RegisterAudit(origRepo)
			}
		})

		toolUseID := "tu_err_1"
		input := map[string]any{"asset_id": float64(1), "sql": "SELECT 1"}
		output := &agent.ToolResultBlock{
			IsError: true,
			Content: []agent.ContentBlock{agent.TextBlock{Text: "connection refused"}},
		}
		runPreHookAndDecision(t, "exec_sql", toolUseID, input, nil)

		_, err := auditPostHook(context.Background(), &agent.PostToolUseInput{
			ToolName: "exec_sql", ToolUseID: toolUseID, Input: input, Output: output,
		})
		So(err, ShouldBeNil)

		waitForAudit(t, mockRepo, 1)
		entry := mockRepo.logs[0]
		So(entry.ToolName, ShouldEqual, "exec_sql")
		So(entry.Command, ShouldEqual, "SELECT 1")
		So(entry.Success, ShouldEqual, 0)
		So(entry.Error, ShouldEqual, "connection refused")
	})
}

func TestAuditPostHook_CapturesCheckResultDecision(t *testing.T) {
	Convey("handler 通过 setCheckResult 设置的决策能被 auditPostHook 写到审计里", t, func() {
		mockRepo := &mockAuditRepo{}
		origRepo := audit_repo.Audit()
		audit_repo.RegisterAudit(mockRepo)
		t.Cleanup(func() {
			if origRepo != nil {
				audit_repo.RegisterAudit(origRepo)
			}
		})

		toolUseID := "tu_dec_1"
		input := map[string]any{"asset_id": float64(1), "command": "uptime"}
		output := &agent.ToolResultBlock{
			Content: []agent.ContentBlock{agent.TextBlock{Text: "ok"}},
		}
		decision := &CheckResult{
			Decision:       Allow,
			DecisionSource: SourceGrantAllow,
			MatchedPattern: "uptime",
		}
		runPreHookAndDecision(t, "run_command", toolUseID, input, decision)

		_, err := auditPostHook(context.Background(), &agent.PostToolUseInput{
			ToolName: "run_command", ToolUseID: toolUseID, Input: input, Output: output,
		})
		So(err, ShouldBeNil)

		waitForAudit(t, mockRepo, 1)
		entry := mockRepo.logs[0]
		So(entry.Decision, ShouldEqual, "allow")
		So(entry.DecisionSource, ShouldEqual, SourceGrantAllow)
		So(entry.MatchedPattern, ShouldEqual, "uptime")
	})
}

func TestAuditPostHook_CleansUpDecisionMap(t *testing.T) {
	Convey("auditPostHook 在写完审计后清理 decisionMap", t, func() {
		toolUseID := "tu_cleanup_1"
		runPreHookAndDecision(t, "run_command", toolUseID, map[string]any{}, nil)
		_, ok := decisionMap.Load(toolUseID)
		So(ok, ShouldBeTrue)

		_, _ = auditPostHook(context.Background(), &agent.PostToolUseInput{
			ToolName: "run_command", ToolUseID: toolUseID,
			Input: map[string]any{}, Output: &agent.ToolResultBlock{},
		})

		_, ok = decisionMap.Load(toolUseID)
		So(ok, ShouldBeFalse)
	})
}
