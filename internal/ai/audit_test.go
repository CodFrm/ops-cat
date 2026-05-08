package ai

import (
	"context"
	"sync"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/audit_entity"
	"github.com/opskat/opskat/internal/repository/audit_repo"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

// --- mock audit repo ---

type mockAuditRepo struct {
	mu   sync.Mutex
	logs []*audit_entity.AuditLog
}

func (m *mockAuditRepo) Create(_ context.Context, log *audit_entity.AuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, log)
	return nil
}

func (m *mockAuditRepo) List(_ context.Context, _ audit_repo.ListOptions) ([]*audit_entity.AuditLog, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.logs, int64(len(m.logs)), nil
}

func (m *mockAuditRepo) ListSessions(_ context.Context, _ int64) ([]audit_repo.SessionInfo, error) {
	return nil, nil
}

func TestContext_AuditSource(t *testing.T) {
	convey.Convey("审计来源 context", t, func() {
		convey.Convey("默认返回空字符串", func() {
			ctx := context.Background()
			assert.Equal(t, "", GetAuditSource(ctx))
		})

		convey.Convey("设置后可以获取", func() {
			ctx := WithAuditSource(context.Background(), "ai")
			assert.Equal(t, "ai", GetAuditSource(ctx))
		})
	})
}

func TestContext_ConversationID(t *testing.T) {
	convey.Convey("会话 ID context", t, func() {
		convey.Convey("默认返回 0", func() {
			ctx := context.Background()
			assert.Equal(t, int64(0), GetConversationID(ctx))
		})

		convey.Convey("设置后可以获取", func() {
			ctx := WithConversationID(context.Background(), 42)
			assert.Equal(t, int64(42), GetConversationID(ctx))
		})
	})
}

func TestContext_GrantSessionID(t *testing.T) {
	convey.Convey("授权会话 ID context", t, func() {
		convey.Convey("默认返回空字符串", func() {
			ctx := context.Background()
			assert.Equal(t, "", GetGrantSessionID(ctx))
		})

		convey.Convey("设置后可以获取", func() {
			ctx := WithGrantSessionID(context.Background(), "grant-abc-123")
			assert.Equal(t, "grant-abc-123", GetGrantSessionID(ctx))
		})
	})
}

func TestCheckResult_DecisionString(t *testing.T) {
	convey.Convey("CheckResult.DecisionString", t, func() {
		convey.Convey("Allow 返回 allow", func() {
			r := CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
			assert.Equal(t, "allow", r.DecisionString())
		})

		convey.Convey("Deny 返回 deny", func() {
			r := CheckResult{Decision: Deny, DecisionSource: SourceUserDeny}
			assert.Equal(t, "deny", r.DecisionString())
		})

		convey.Convey("NeedConfirm 返回空字符串", func() {
			r := CheckResult{Decision: NeedConfirm}
			assert.Equal(t, "", r.DecisionString())
		})
	})
}

func TestCheckResult_Context(t *testing.T) {
	convey.Convey("CheckResult context 传递", t, func() {
		convey.Convey("setCheckResult 填充占位指针", func() {
			holder := &CheckResult{}
			ctx := withCheckResult(context.Background(), holder)

			setCheckResult(ctx, CheckResult{
				Decision:       Allow,
				DecisionSource: SourceGrantAllow,
				MatchedPattern: "cat *",
			})

			assert.Equal(t, Allow, holder.Decision)
			assert.Equal(t, SourceGrantAllow, holder.DecisionSource)
			assert.Equal(t, "cat *", holder.MatchedPattern)
		})

		convey.Convey("无占位指针时 setCheckResult 不 panic", func() {
			ctx := context.Background()
			assert.NotPanics(t, func() {
				setCheckResult(ctx, CheckResult{Decision: Allow})
			})
		})
	})
}

func TestExtractCommandForAudit(t *testing.T) {
	convey.Convey("从工具参数提取命令", t, func() {
		convey.Convey("run_command 提取 command 字段", func() {
			cmd := ExtractCommandForAudit("run_command", map[string]any{
				"asset_id": float64(1),
				"command":  "uptime",
			})
			assert.Equal(t, "uptime", cmd)
		})

		convey.Convey("upload_file 生成上传描述", func() {
			cmd := ExtractCommandForAudit("upload_file", map[string]any{
				"asset_id":    float64(1),
				"local_path":  "/tmp/config.yml",
				"remote_path": "/etc/app/config.yml",
			})
			assert.Equal(t, "upload /tmp/config.yml → /etc/app/config.yml", cmd)
		})

		convey.Convey("download_file 生成下载描述", func() {
			cmd := ExtractCommandForAudit("download_file", map[string]any{
				"asset_id":    float64(1),
				"remote_path": "/var/log/app.log",
				"local_path":  "./app.log",
			})
			assert.Equal(t, "download /var/log/app.log → ./app.log", cmd)
		})

		convey.Convey("exec (opsctl) 提取 command 字段", func() {
			cmd := ExtractCommandForAudit("exec", map[string]any{
				"asset_id": float64(1),
				"command":  "df -h",
			})
			assert.Equal(t, "df -h", cmd)
		})

		convey.Convey("exec_sql 提取 sql 字段", func() {
			cmd := ExtractCommandForAudit("exec_sql", map[string]any{
				"asset_id": float64(1),
				"sql":      "SELECT * FROM users LIMIT 10",
			})
			assert.Equal(t, "SELECT * FROM users LIMIT 10", cmd)
		})

		convey.Convey("exec_redis 提取 command 字段", func() {
			cmd := ExtractCommandForAudit("exec_redis", map[string]any{
				"asset_id": float64(1),
				"command":  "GET mykey",
			})
			assert.Equal(t, "GET mykey", cmd)
		})

		convey.Convey("exec_k8s 规范化 kubectl 命令", func() {
			cmd := ExtractCommandForAudit("exec_k8s", map[string]any{
				"asset_id": float64(1),
				"command":  "get pods -A",
			})
			assert.Equal(t, "kubectl get pods -A", cmd)
		})

		convey.Convey("其他工具返回空字符串", func() {
			cmd := ExtractCommandForAudit("list_assets", map[string]any{})
			assert.Equal(t, "", cmd)

			cmd = ExtractCommandForAudit("add_asset", map[string]any{
				"name": "web-01",
				"host": "10.0.0.1",
			})
			assert.Equal(t, "", cmd)
		})
	})
}

func TestTruncateString(t *testing.T) {
	convey.Convey("字符串截断", t, func() {
		convey.Convey("短字符串不截断", func() {
			assert.Equal(t, "hello", truncateString("hello", 10))
		})

		convey.Convey("超长字符串截断到指定长度并追加标记", func() {
			result := truncateString("abcdefghij", 5)
			assert.Equal(t, "abcde\n...[truncated]", result)
		})

		convey.Convey("空字符串返回空", func() {
			assert.Equal(t, "", truncateString("", 10))
		})
	})
}

