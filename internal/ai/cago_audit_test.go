package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/opskat/opskat/internal/repository/audit_repo"
	. "github.com/smartystreets/goconvey/convey"
)

// waitForAudit 阻塞等到 mockAuditRepo 收到至少 want 条记录，或超时。
// rawTool 的 audit 写入是 fire-and-forget goroutine，必须显式等。
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

func TestRawTool_WritesAuditOnSuccess(t *testing.T) {
	Convey("rawTool 包装后的 handler 在成功路径写出审计记录", t, func() {
		mockRepo := &mockAuditRepo{}
		origRepo := audit_repo.Audit()
		audit_repo.RegisterAudit(mockRepo)
		t.Cleanup(func() {
			if origRepo != nil {
				audit_repo.RegisterAudit(origRepo)
			}
		})

		tl := rawTool(
			"run_command",
			"desc",
			makeSchema(
				paramSpec{name: "asset_id", typ: "number", required: true, desc: "id"},
				paramSpec{name: "command", typ: "string", required: true, desc: "cmd"},
			),
			true,
			func(_ context.Context, args map[string]any) (string, error) {
				return "ok-output", nil
			},
		)

		ctx := WithAuditSource(context.Background(), "ai")
		ctx = WithConversationID(ctx, 99)

		out, err := tl.Call(ctx, map[string]any{"asset_id": float64(7), "command": "uptime"})
		So(err, ShouldBeNil)
		So(out.IsError, ShouldBeFalse)
		So(out.Content, ShouldHaveLength, 1)
		txt, _ := out.Content[0].(*agent.TextBlock)
		So(txt.Text, ShouldEqual, "ok-output")

		waitForAudit(t, mockRepo, 1)
		entry := mockRepo.logs[0]
		So(entry.ToolName, ShouldEqual, "run_command")
		So(entry.Source, ShouldEqual, "ai")
		So(entry.ConversationID, ShouldEqual, int64(99))
		So(entry.Command, ShouldEqual, "uptime") // 来自 commandExtractors
		So(entry.Success, ShouldEqual, 1)
		So(entry.Error, ShouldEqual, "")
		So(entry.AssetID, ShouldEqual, int64(7))
	})
}

func TestRawTool_WritesAuditOnError(t *testing.T) {
	Convey("rawTool 包装后的 handler 在 error 路径仍写出审计", t, func() {
		mockRepo := &mockAuditRepo{}
		origRepo := audit_repo.Audit()
		audit_repo.RegisterAudit(mockRepo)
		t.Cleanup(func() {
			if origRepo != nil {
				audit_repo.RegisterAudit(origRepo)
			}
		})

		tl := rawTool("exec_sql", "desc", makeSchema(
			paramSpec{name: "asset_id", typ: "number", required: true, desc: "id"},
			paramSpec{name: "sql", typ: "string", required: true, desc: "sql"},
		), true, func(_ context.Context, args map[string]any) (string, error) {
			return "", errors.New("connection refused")
		})

		out, err := tl.Call(context.Background(), map[string]any{"asset_id": float64(1), "sql": "SELECT 1"})
		So(err, ShouldBeNil) // textResult 把 error 包成 IsError=true，不上抛
		So(out.IsError, ShouldBeTrue)

		waitForAudit(t, mockRepo, 1)
		entry := mockRepo.logs[0]
		So(entry.ToolName, ShouldEqual, "exec_sql")
		So(entry.Command, ShouldEqual, "SELECT 1")
		So(entry.Success, ShouldEqual, 0)
		So(entry.Error, ShouldEqual, "connection refused")
	})
}

func TestRawTool_CapturesCheckResultDecision(t *testing.T) {
	Convey("handler 通过 setCheckResult 设置的决策能被 rawTool 写到审计里", t, func() {
		mockRepo := &mockAuditRepo{}
		origRepo := audit_repo.Audit()
		audit_repo.RegisterAudit(mockRepo)
		t.Cleanup(func() {
			if origRepo != nil {
				audit_repo.RegisterAudit(origRepo)
			}
		})

		tl := rawTool("run_command", "desc", makeSchema(
			paramSpec{name: "asset_id", typ: "number", required: true, desc: "id"},
			paramSpec{name: "command", typ: "string", required: true, desc: "cmd"},
		), true, func(ctx context.Context, args map[string]any) (string, error) {
			setCheckResult(ctx, CheckResult{
				Decision:       Allow,
				DecisionSource: SourceGrantAllow,
				MatchedPattern: "uptime",
			})
			return "ok", nil
		})

		_, err := tl.Call(context.Background(), map[string]any{"asset_id": float64(1), "command": "uptime"})
		So(err, ShouldBeNil)

		waitForAudit(t, mockRepo, 1)
		entry := mockRepo.logs[0]
		So(entry.Decision, ShouldEqual, "allow")
		So(entry.DecisionSource, ShouldEqual, SourceGrantAllow)
		So(entry.MatchedPattern, ShouldEqual, "uptime")
	})
}
