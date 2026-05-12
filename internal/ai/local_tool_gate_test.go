package ai

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"
)

// fakeConfirm 记录每次 confirm 调用并按预设响应回复。
type fakeConfirm struct {
	calls    []LocalToolApprovalRequest
	response ApprovalResponse
}

func (f *fakeConfirm) fn(_ context.Context, req LocalToolApprovalRequest) ApprovalResponse {
	f.calls = append(f.calls, req)
	return f.response
}

func ctxWithConv(id int64) context.Context {
	return WithConversationID(context.Background(), id)
}

// gateOutcome 是 driveGate 返回的"中间件跑完后能观测到的状态"汇总。
//   - allowed: 终端 stub 是否被调用（即 c.Next() 推到底）
//   - denied:  c.AbortWithDeny 是否触发
//   - denyMsg: deny 时的 reason（通过 dispatcher 端到端组装出 deny block 的 text 抽出）
type gateOutcome struct {
	allowed bool
	denied  bool
	denyMsg string
}

// driveGate 把 LocalToolGate.Middleware() 端到端跑一遍：用一个真 ToolDispatcher
// 注册 [gate, fakeTerminal]，调 Run，从 DispatchResult.Output 反向推断 allowed/denied。
//
// 不直接读 ToolContext 私有字段，是为了避免和 cago 内部实现耦合 —— 走 dispatcher
// 与生产路径一致。
func driveGate(ctx context.Context, g *LocalToolGate, toolName string, input map[string]any) gateOutcome {
	var allowed bool
	stubName := toolName
	td := &agent.ToolDispatcher{
		Tools: []agent.Tool{stubLocalTool{name: stubName, called: &allowed}},
		Middleware: []agent.ToolHookEntry[agent.ToolMiddleware]{
			{Matcher: ".*", Fn: g.Middleware()},
		},
	}
	res := td.Run(ctx, agent.DispatchInput{
		ToolName:  toolName,
		ToolUseID: "tu_test",
		Input:     input,
	})
	out := gateOutcome{allowed: allowed}
	if res.Output != nil && res.Output.IsError && !allowed {
		out.denied = true
		// dispatcher 把 AbortWithDeny(reason) 渲染成 "denied: " + reason
		// 块——抽出原文便于断言。
		for _, blk := range res.Output.Content {
			if tb, ok := blk.(agent.TextBlock); ok {
				out.denyMsg = tb.Text
				break
			}
		}
	}
	return out
}

type stubLocalTool struct {
	name   string
	called *bool
}

func (s stubLocalTool) Name() string         { return s.name }
func (s stubLocalTool) Description() string  { return "stub" }
func (s stubLocalTool) Schema() agent.Schema { return agent.Schema{Type: "object"} }
func (s stubLocalTool) Call(_ context.Context, _ map[string]any) (*agent.ToolResultBlock, error) {
	if s.called != nil {
		*s.called = true
	}
	return &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: "ran"}}}, nil
}

func TestLocalToolGate_Pass_NoConfirm_WhenPatternMatches(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "allowAll"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "bash", "ls *")

	out := driveGate(ctxWithConv(1), g, "bash", map[string]any{"command": "ls -la"})
	assert.True(t, out.allowed)
	assert.Empty(t, fc.calls, "白名单命中时不应触发 confirm")
}

func TestLocalToolGate_Deny_ReturnsDecisionDenyWithReason(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)

	out := driveGate(ctxWithConv(1), g, "bash", map[string]any{"command": "rm -rf /"})
	assert.False(t, out.allowed)
	assert.True(t, out.denied)
	assert.Contains(t, out.denyMsg, "USER DENIED")
	assert.Len(t, fc.calls, 1)
}

func TestLocalToolGate_AllowAll_RemembersPatternAndSkipsNextCall(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{
		Decision: "allowAll",
		EditedItems: []ApprovalItem{
			{Type: "bash", Command: "git *"},
		},
	}}
	g := NewLocalToolGate(fc.fn)

	// 首次：触发 confirm，并写入 "git *"
	out := driveGate(ctxWithConv(7), g, "bash", map[string]any{"command": "git pull"})
	assert.True(t, out.allowed)
	assert.Len(t, fc.calls, 1)

	// 第二次同对话：命中白名单，不再调 confirm
	out = driveGate(ctxWithConv(7), g, "bash", map[string]any{"command": "git status"})
	assert.True(t, out.allowed)
	assert.Len(t, fc.calls, 1, "命中白名单后不应再次 confirm")
}

func TestLocalToolGate_ConvIDIsolated(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "bash", "ls *")

	// conv=2 同样命令：未命中白名单，触发 confirm
	out := driveGate(ctxWithConv(2), g, "bash", map[string]any{"command": "ls -la"})
	assert.True(t, out.denied)
	assert.Len(t, fc.calls, 1)
}

func TestLocalToolGate_Bash_ComplexCommand_AllSubMustMatch(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "bash", "git *") // 仅白名单 git 系列

	// `git pull && npm test`：npm test 未命中 → 触发 confirm
	driveGate(ctxWithConv(1), g, "bash", map[string]any{"command": "git pull && npm test"})
	assert.Len(t, fc.calls, 1)
	assert.ElementsMatch(t, []string{"git pull", "npm test"}, fc.calls[0].SubCommands)

	// 补 npm 模式后再来一次复合：全部命中，免审批
	g.remember(1, "bash", "npm *")
	out := driveGate(ctxWithConv(1), g, "bash", map[string]any{"command": "git pull && npm test"})
	assert.True(t, out.allowed)
	assert.Len(t, fc.calls, 1, "两种 pattern 都命中后不应再触发 confirm")
}

func TestLocalToolGate_Bash_QuotesNotSplit(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)

	driveGate(ctxWithConv(1), g, "bash", map[string]any{"command": `echo "a && b"`})
	assert.Len(t, fc.calls, 1)
	// mvdan.cc/sh 不会把引号内的 && 当作分隔符
	assert.Len(t, fc.calls[0].SubCommands, 1)
}

func TestLocalToolGate_Write_PathMatch(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "write", "/tmp/*")

	out := driveGate(ctxWithConv(1), g, "write", map[string]any{"path": "/tmp/foo.txt", "content": "hello"})
	assert.True(t, out.allowed)
	assert.Empty(t, fc.calls)
}

func TestLocalToolGate_Write_PathMiss_TriggersConfirm(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "write", "/tmp/*")

	driveGate(ctxWithConv(1), g, "write", map[string]any{"path": "/etc/passwd", "content": "x"})
	assert.Len(t, fc.calls, 1)
	assert.Equal(t, "/etc/passwd", fc.calls[0].Command)
	assert.NotEmpty(t, fc.calls[0].Detail, "write 应带 content 预览")
}

func TestLocalToolGate_NoConfirmFunc_DeniesUnknown(t *testing.T) {
	g := NewLocalToolGate(nil) // 未挂 confirm
	out := driveGate(ctxWithConv(1), g, "bash", map[string]any{"command": "ls"})
	assert.True(t, out.denied)
	assert.Contains(t, out.denyMsg, "no approval mechanism")
}

func TestLocalToolGate_EmptyInput_PassThrough(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)

	out := driveGate(ctxWithConv(1), g, "bash", map[string]any{"command": ""})
	assert.True(t, out.allowed)
	assert.Empty(t, fc.calls, "空输入不应触发 confirm，留给工具自行报错")
}

func TestLocalToolGate_Reset_ClearsConversation(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "bash", "ls *")
	g.Reset(1)

	driveGate(ctxWithConv(1), g, "bash", map[string]any{"command": "ls"})
	assert.Len(t, fc.calls, 1, "Reset 后应重新触发 confirm")
}

func TestDefaultBashPattern(t *testing.T) {
	assert.Equal(t, "git *", defaultBashPattern("git pull origin main"))
	assert.Equal(t, "ls *", defaultBashPattern("ls -la /tmp"))
	assert.Equal(t, "rm *", defaultBashPattern("rm /tmp/foo"))
	assert.Equal(t, "", defaultBashPattern(""))
}
