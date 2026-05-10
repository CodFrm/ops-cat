package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubagentDispatchHook_TruncatesLongOutput(t *testing.T) {
	h := newSubagentDispatchHook(10)
	out, err := h(context.Background(), &agent.PostToolUseInput{
		ToolName: "dispatch_subagent",
		Output: &agent.ToolResultBlock{
			Content: []agent.ContentBlock{agent.TextBlock{Text: "0123456789ABCDEF"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out.ModifiedOutput)
	require.Len(t, out.ModifiedOutput.Content, 1)
	tb, ok := out.ModifiedOutput.Content[0].(agent.TextBlock)
	require.True(t, ok)
	assert.True(t, len(tb.Text) <= 10+len("…[截断]"), "truncated text length should be at most maxLen + suffix")
	assert.Contains(t, tb.Text, "…[截断]")
	assert.True(t, len(tb.Text) > 10, "should retain the first maxLen runes plus suffix")
	assert.Equal(t, "0123456789", tb.Text[:10])
}

func TestSubagentDispatchHook_ShortOutputUnchanged(t *testing.T) {
	h := newSubagentDispatchHook(100)
	out, err := h(context.Background(), &agent.PostToolUseInput{
		ToolName: "dispatch_subagent",
		Output: &agent.ToolResultBlock{
			Content: []agent.ContentBlock{agent.TextBlock{Text: "short result"}},
		},
	})
	require.NoError(t, err)
	assert.Nil(t, out.ModifiedOutput, "no truncation needed → no ModifiedOutput")
}

func TestSubagentDispatchHook_NonSubagentNoOp(t *testing.T) {
	h := newSubagentDispatchHook(10)
	out, err := h(context.Background(), &agent.PostToolUseInput{
		ToolName: "ssh.exec",
		Output: &agent.ToolResultBlock{
			Content: []agent.ContentBlock{agent.TextBlock{Text: "very long ssh output here"}},
		},
	})
	require.NoError(t, err)
	assert.Nil(t, out.ModifiedOutput, "non-dispatch_subagent tools should pass through")
}

func TestSubagentDispatchHook_NilOutputNoOp(t *testing.T) {
	h := newSubagentDispatchHook(10)
	out, err := h(context.Background(), &agent.PostToolUseInput{
		ToolName: "dispatch_subagent",
		Output:   nil,
	})
	require.NoError(t, err)
	assert.Nil(t, out.ModifiedOutput)
}

func TestSubagentDispatchHook_PreservesToolUseIDAndIsError(t *testing.T) {
	h := newSubagentDispatchHook(5)
	out, err := h(context.Background(), &agent.PostToolUseInput{
		ToolName: "dispatch_subagent",
		Output: &agent.ToolResultBlock{
			ToolUseID: "tu_subagent_42",
			IsError:   true,
			Content:   []agent.ContentBlock{agent.TextBlock{Text: "very long error output"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out.ModifiedOutput)
	assert.Equal(t, "tu_subagent_42", out.ModifiedOutput.ToolUseID)
	assert.True(t, out.ModifiedOutput.IsError)
}
