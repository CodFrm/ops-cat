package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"
)

type captureAudit struct {
	rows []captureAuditRow
}

type captureAuditRow struct {
	toolName, input, output string
	isError                 bool
}

func (c *captureAudit) Write(ctx context.Context, toolName, inputJSON, outputJSON string, isError bool) error {
	c.rows = append(c.rows, captureAuditRow{toolName, inputJSON, outputJSON, isError})
	return nil
}

func TestAuditHook_WritesPerCall(t *testing.T) {
	a := &captureAudit{}
	h := newAuditHook(a)
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
}

func TestAuditHook_PreservesIsErrorFlag(t *testing.T) {
	a := &captureAudit{}
	h := newAuditHook(a)
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
	// Defensive: cago should always provide Output, but guard.
	a := &captureAudit{}
	h := newAuditHook(a)
	_, err := h(context.Background(), &agent.PostToolUseInput{
		ToolName: "noop",
		Input:    map[string]any{},
		Output:   nil,
	})
	assert.NoError(t, err)
	assert.Len(t, a.rows, 0, "nil Output should skip write rather than panic")
}
