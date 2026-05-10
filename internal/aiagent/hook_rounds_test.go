package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"
)

func TestRoundsCounter_Cap(t *testing.T) {
	rc := newRoundsCounter(2)
	h := rc.Hook()
	for i := 0; i < 2; i++ {
		out, err := h(context.Background(), &agent.PreToolUseInput{})
		assert.NoError(t, err)
		assert.Equal(t, agent.DecisionPass, out.Decision)
	}
	out, err := h(context.Background(), &agent.PreToolUseInput{})
	assert.NoError(t, err)
	assert.Equal(t, agent.DecisionDeny, out.Decision)
	assert.Contains(t, out.DenyReason, "回合上限")
}

func TestRoundsCounter_ResetClearsBudget(t *testing.T) {
	rc := newRoundsCounter(1)
	h := rc.Hook()
	_, _ = h(context.Background(), &agent.PreToolUseInput{})
	out1, _ := h(context.Background(), &agent.PreToolUseInput{})
	assert.Equal(t, agent.DecisionDeny, out1.Decision)
	rc.Reset()
	out2, _ := h(context.Background(), &agent.PreToolUseInput{})
	assert.Equal(t, agent.DecisionPass, out2.Decision)
}

func TestRoundsCounter_ZeroMaxAllowsAll(t *testing.T) {
	// max=0 means unlimited (defensive default; explicit semantics)
	rc := newRoundsCounter(0)
	h := rc.Hook()
	for i := 0; i < 10; i++ {
		out, _ := h(context.Background(), &agent.PreToolUseInput{})
		assert.Equal(t, agent.DecisionPass, out.Decision, "max=0 should allow unlimited rounds")
	}
}
