package aiagent

import (
	"context"

	"github.com/cago-frame/agents/agent"
)

// subagentTruncateSuffix is appended after the first maxLen bytes of the
// (concatenated) text content to signal truncation to the parent agent.
const subagentTruncateSuffix = "…[截断]"

// newSubagentDispatchHook returns a PostToolUse hook that truncates the text
// output of the `dispatch_subagent` tool. Other tools pass through unchanged.
//
// Truncation operates per-block: each TextBlock contributes its bytes against
// the budget. When the budget is exhausted in the middle of a TextBlock, the
// block is truncated at maxLen with a "…[截断]" suffix and any subsequent blocks
// are dropped. Non-text blocks (ToolUse / Image / etc.) are preserved as-is and
// don't consume the byte budget — they're rare in subagent output.
//
// maxLen counts in BYTES, matching how Go strings index. UTF-8 multi-byte
// runes may be split mid-byte; for an ASCII-heavy summary this is fine.
func newSubagentDispatchHook(maxLen int) agent.PostToolUseHook {
	return func(ctx context.Context, in *agent.PostToolUseInput) (*agent.PostToolUseOutput, error) {
		if in.ToolName != "dispatch_subagent" || in.Output == nil {
			return &agent.PostToolUseOutput{}, nil
		}
		truncated, didTruncate := truncateBlocks(in.Output.Content, maxLen)
		if !didTruncate {
			return &agent.PostToolUseOutput{}, nil
		}
		return &agent.PostToolUseOutput{
			ModifiedOutput: &agent.ToolResultBlock{
				ToolUseID: in.Output.ToolUseID,
				IsError:   in.Output.IsError,
				Content:   truncated,
			},
		}, nil
	}
}

// truncateBlocks walks blocks, accumulating text-block bytes against the
// remaining budget. Returns the truncated slice plus a bool indicating whether
// truncation actually happened.
func truncateBlocks(blocks []agent.ContentBlock, maxLen int) ([]agent.ContentBlock, bool) {
	out := make([]agent.ContentBlock, 0, len(blocks))
	remaining := maxLen
	truncated := false
	for i, b := range blocks {
		tb, ok := b.(agent.TextBlock)
		if !ok {
			out = append(out, b)
			continue
		}
		if len(tb.Text) > remaining {
			out = append(out, agent.TextBlock{Text: tb.Text[:remaining] + subagentTruncateSuffix})
			truncated = true
			// Drop all subsequent blocks — beyond the budget.
			_ = i // consumed explicitly for clarity
			break
		}
		out = append(out, tb)
		remaining -= len(tb.Text)
	}
	return out, truncated
}
