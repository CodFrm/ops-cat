package ai

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestLocalToolGate_Pass_NoConfirm_WhenPatternMatches(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "allowAll"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "bash", "ls *")

	out, err := g.Hook()(ctxWithConv(1), &agent.PreToolUseInput{
		ToolName: "bash",
		Input:    map[string]any{"command": "ls -la"},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionPass, out.Decision)
	assert.Empty(t, fc.calls, "白名单命中时不应触发 confirm")
}

func TestLocalToolGate_Deny_ReturnsDecisionDenyWithReason(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)

	out, err := g.Hook()(ctxWithConv(1), &agent.PreToolUseInput{
		ToolName: "bash",
		Input:    map[string]any{"command": "rm -rf /"},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionDeny, out.Decision)
	assert.NotEmpty(t, out.DenyReason)
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
	_, err := g.Hook()(ctxWithConv(7), &agent.PreToolUseInput{
		ToolName: "bash",
		Input:    map[string]any{"command": "git pull"},
	})
	require.NoError(t, err)
	assert.Len(t, fc.calls, 1)

	// 第二次同对话：命中白名单，不再调 confirm
	out, err := g.Hook()(ctxWithConv(7), &agent.PreToolUseInput{
		ToolName: "bash",
		Input:    map[string]any{"command": "git status"},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionPass, out.Decision)
	assert.Len(t, fc.calls, 1, "命中白名单后不应再次 confirm")
}

func TestLocalToolGate_ConvIDIsolated(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "bash", "ls *")

	// conv=2 同样命令：未命中白名单，触发 confirm
	out, err := g.Hook()(ctxWithConv(2), &agent.PreToolUseInput{
		ToolName: "bash",
		Input:    map[string]any{"command": "ls -la"},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionDeny, out.Decision)
	assert.Len(t, fc.calls, 1)
}

func TestLocalToolGate_Bash_ComplexCommand_AllSubMustMatch(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "bash", "git *") // 仅白名单 git 系列

	// `git pull && npm test`：npm test 未命中 → 触发 confirm
	_, err := g.Hook()(ctxWithConv(1), &agent.PreToolUseInput{
		ToolName: "bash",
		Input:    map[string]any{"command": "git pull && npm test"},
	})
	require.NoError(t, err)
	require.Len(t, fc.calls, 1)
	assert.ElementsMatch(t, []string{"git pull", "npm test"}, fc.calls[0].SubCommands)

	// 补 npm 模式后再来一次复合：全部命中，免审批
	g.remember(1, "bash", "npm *")
	out, err := g.Hook()(ctxWithConv(1), &agent.PreToolUseInput{
		ToolName: "bash",
		Input:    map[string]any{"command": "git pull && npm test"},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionPass, out.Decision)
	assert.Len(t, fc.calls, 1, "两种 pattern 都命中后不应再触发 confirm")
}

func TestLocalToolGate_Bash_QuotesNotSplit(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)

	_, err := g.Hook()(ctxWithConv(1), &agent.PreToolUseInput{
		ToolName: "bash",
		Input:    map[string]any{"command": `echo "a && b"`},
	})
	require.NoError(t, err)
	require.Len(t, fc.calls, 1)
	// mvdan.cc/sh 不会把引号内的 && 当作分隔符
	assert.Len(t, fc.calls[0].SubCommands, 1)
}

func TestLocalToolGate_Write_PathMatch(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "write", "/tmp/*")

	out, err := g.Hook()(ctxWithConv(1), &agent.PreToolUseInput{
		ToolName: "write",
		Input:    map[string]any{"path": "/tmp/foo.txt", "content": "hello"},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionPass, out.Decision)
	assert.Empty(t, fc.calls)
}

func TestLocalToolGate_Write_PathMiss_TriggersConfirm(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "write", "/tmp/*")

	_, err := g.Hook()(ctxWithConv(1), &agent.PreToolUseInput{
		ToolName: "write",
		Input:    map[string]any{"path": "/etc/passwd", "content": "x"},
	})
	require.NoError(t, err)
	require.Len(t, fc.calls, 1)
	assert.Equal(t, "/etc/passwd", fc.calls[0].Command)
	assert.NotEmpty(t, fc.calls[0].Detail, "write 应带 content 预览")
}

func TestLocalToolGate_NoConfirmFunc_DeniesUnknown(t *testing.T) {
	g := NewLocalToolGate(nil) // 未挂 confirm
	out, err := g.Hook()(ctxWithConv(1), &agent.PreToolUseInput{
		ToolName: "bash",
		Input:    map[string]any{"command": "ls"},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionDeny, out.Decision)
	assert.NotEmpty(t, out.DenyReason)
}

func TestLocalToolGate_EmptyInput_PassThrough(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)

	out, err := g.Hook()(ctxWithConv(1), &agent.PreToolUseInput{
		ToolName: "bash",
		Input:    map[string]any{"command": ""},
	})
	require.NoError(t, err)
	assert.Equal(t, agent.DecisionPass, out.Decision)
	assert.Empty(t, fc.calls, "空输入不应触发 confirm，留给工具自行报错")
}

func TestLocalToolGate_Reset_ClearsConversation(t *testing.T) {
	fc := &fakeConfirm{response: ApprovalResponse{Decision: "deny"}}
	g := NewLocalToolGate(fc.fn)
	g.remember(1, "bash", "ls *")
	g.Reset(1)

	_, err := g.Hook()(ctxWithConv(1), &agent.PreToolUseInput{
		ToolName: "bash",
		Input:    map[string]any{"command": "ls"},
	})
	require.NoError(t, err)
	assert.Len(t, fc.calls, 1, "Reset 后应重新触发 confirm")
}

func TestDefaultBashPattern(t *testing.T) {
	assert.Equal(t, "git *", defaultBashPattern("git pull origin main"))
	assert.Equal(t, "ls *", defaultBashPattern("ls -la /tmp"))
	assert.Equal(t, "rm *", defaultBashPattern("rm /tmp/foo"))
	assert.Equal(t, "", defaultBashPattern(""))
}
