package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"

	"github.com/opskat/opskat/internal/ai"
)

type fakeChecker struct {
	out PolicyOutcome
}

func (f *fakeChecker) Check(_ context.Context, _ string, _ map[string]any) (PolicyOutcome, error) {
	return f.out, nil
}

func TestPolicyHook_AllowsSafeCommand(t *testing.T) {
	store := newToolDecisionStore()
	h := newPolicyHook(&fakeChecker{out: PolicyOutcome{
		Decision:       PolicyAllow,
		DecisionSource: ai.SourcePolicyAllow,
	}}, nil, store)
	out, err := h(context.Background(), &agent.PreToolUseInput{
		ToolName: "ssh.exec", ToolUseID: "tu-allow",
		Input: map[string]any{"cmd": "ls"},
	})
	assert.NoError(t, err)
	assert.Equal(t, agent.DecisionPass, out.Decision)
	dec := store.Pop("tu-allow")
	assert.NotNil(t, dec)
	assert.Equal(t, ai.Allow, dec.Decision)
	assert.Equal(t, ai.SourcePolicyAllow, dec.DecisionSource)
}

func TestPolicyHook_DeniesDangerous(t *testing.T) {
	store := newToolDecisionStore()
	h := newPolicyHook(&fakeChecker{out: PolicyOutcome{
		Decision:       PolicyDeny,
		Reason:         "rm -rf 禁用",
		DecisionSource: ai.SourcePolicyDeny,
		MatchedPattern: "rm *",
	}}, nil, store)
	out, err := h(context.Background(), &agent.PreToolUseInput{
		ToolName: "ssh.exec", ToolUseID: "tu-deny",
		Input: map[string]any{"cmd": "rm -rf /"},
	})
	assert.NoError(t, err)
	assert.Equal(t, agent.DecisionDeny, out.Decision)
	assert.Equal(t, "rm -rf 禁用", out.DenyReason)
	dec := store.Pop("tu-deny")
	assert.NotNil(t, dec)
	assert.Equal(t, ai.Deny, dec.Decision)
	assert.Equal(t, "rm *", dec.MatchedPattern)
}

// PolicyConfirm 且 gw=nil 时按 deny 兜底（避免无渠道时静默放行）。
func TestPolicyHook_ConfirmWithoutGatewayDenies(t *testing.T) {
	h := newPolicyHook(&fakeChecker{out: PolicyOutcome{Decision: PolicyConfirm}}, nil, nil)
	out, err := h(context.Background(), &agent.PreToolUseInput{
		ToolName: "run_command",
		Input:    map[string]any{"command": "rm -rf /"},
	})
	assert.NoError(t, err)
	assert.Equal(t, agent.DecisionDeny, out.Decision)
}
