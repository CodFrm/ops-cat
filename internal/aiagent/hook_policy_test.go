package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"
)

type fakeChecker struct {
	allow  bool
	reason string
}

func (f *fakeChecker) Check(ctx context.Context, toolName string, input map[string]any) (allowed bool, reason string, err error) {
	return f.allow, f.reason, nil
}

func TestPolicyHook_AllowsSafeCommand(t *testing.T) {
	h := newPolicyHook(&fakeChecker{allow: true})
	out, err := h(context.Background(), &agent.PreToolUseInput{
		ToolName: "ssh.exec",
		Input:    map[string]any{"cmd": "ls"},
	})
	assert.NoError(t, err)
	assert.Equal(t, agent.DecisionPass, out.Decision)
}

func TestPolicyHook_DeniesDangerous(t *testing.T) {
	h := newPolicyHook(&fakeChecker{allow: false, reason: "rm -rf 禁用"})
	out, err := h(context.Background(), &agent.PreToolUseInput{
		ToolName: "ssh.exec",
		Input:    map[string]any{"cmd": "rm -rf /"},
	})
	assert.NoError(t, err)
	assert.Equal(t, agent.DecisionDeny, out.Decision)
	assert.Equal(t, "rm -rf 禁用", out.DenyReason)
}
