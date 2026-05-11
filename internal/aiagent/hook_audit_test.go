package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"

	"github.com/opskat/opskat/internal/ai"
)

type captureAudit struct {
	rows []captureAuditRow
}

type captureAuditRow struct {
	toolName, input, output string
	isError                 bool
	decision                *ai.CheckResult
}

func (c *captureAudit) Write(_ context.Context, toolName, inputJSON, outputJSON string, isError bool, decision *ai.CheckResult) error {
	c.rows = append(c.rows, captureAuditRow{toolName, inputJSON, outputJSON, isError, decision})
	return nil
}

func TestAuditHook_WritesPerCall(t *testing.T) {
	a := &captureAudit{}
	h := newAuditHook(a, nil)
	out, err := h(context.Background(), &agent.PostToolUseInput{
		ToolName: "ssh.exec",
		Input:    map[string]any{"cmd": "ls"},
		Output: &agent.ToolResultBlock{
			Content: []agent.ContentBlock{agent.TextBlock{Text: "ok"}},
			IsError: false,
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, out)
	assert.Nil(t, out.ModifiedOutput)
	assert.Len(t, a.rows, 1)
	assert.Equal(t, "ssh.exec", a.rows[0].toolName)
	assert.Contains(t, a.rows[0].input, "cmd")
	assert.Contains(t, a.rows[0].output, "ok")
	assert.False(t, a.rows[0].isError)
	assert.Nil(t, a.rows[0].decision, "no store → decision is nil")
}

func TestAuditHook_PreservesIsErrorFlag(t *testing.T) {
	a := &captureAudit{}
	h := newAuditHook(a, nil)
	_, err := h(context.Background(), &agent.PostToolUseInput{
		ToolName: "redis.exec",
		Input:    map[string]any{"cmd": "BADCMD"},
		Output: &agent.ToolResultBlock{
			Content: []agent.ContentBlock{agent.TextBlock{Text: "syntax error"}},
			IsError: true,
		},
	})
	assert.NoError(t, err)
	assert.Len(t, a.rows, 1)
	assert.True(t, a.rows[0].isError)
}

func TestAuditHook_NilOutputDoesNotPanic(t *testing.T) {
	a := &captureAudit{}
	h := newAuditHook(a, nil)
	_, err := h(context.Background(), &agent.PostToolUseInput{
		ToolName: "noop",
		Input:    map[string]any{},
		Output:   nil,
	})
	assert.NoError(t, err)
	assert.Len(t, a.rows, 0, "nil Output should skip write rather than panic")
}

// audit hook 通过 store Pop 出 policy 的决策记录传给 AuditWriter。
func TestAuditHook_PopsDecisionFromStore(t *testing.T) {
	a := &captureAudit{}
	store := newToolDecisionStore()
	store.Stash("tu-1", &ai.CheckResult{
		Decision:       ai.Allow,
		DecisionSource: ai.SourceUserAllow,
		MatchedPattern: "ls *",
	})
	h := newAuditHook(a, store)
	_, err := h(context.Background(), &agent.PostToolUseInput{
		ToolName:  "ssh.exec",
		ToolUseID: "tu-1",
		Input:     map[string]any{"cmd": "ls"},
		Output:    &agent.ToolResultBlock{Content: []agent.ContentBlock{agent.TextBlock{Text: "ok"}}},
	})
	assert.NoError(t, err)
	assert.Len(t, a.rows, 1)
	assert.NotNil(t, a.rows[0].decision)
	assert.Equal(t, ai.Allow, a.rows[0].decision.Decision)
	assert.Equal(t, ai.SourceUserAllow, a.rows[0].decision.DecisionSource)
	assert.Equal(t, "ls *", a.rows[0].decision.MatchedPattern)
	// Pop 是消费式：第二次 Pop 拿不到。
	assert.Nil(t, store.Pop("tu-1"))
}
