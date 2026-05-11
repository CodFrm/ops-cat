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
// Truncation operates per-block: each TextBlock contributes its runes against
// the budget. When the budget is exhausted in the middle of a TextBlock, the
// block is truncated at maxRunes with a "…[截断]" suffix and any subsequent
// blocks are dropped. Non-text blocks (ToolUse / Image / etc.) are preserved
// as-is and don't consume the budget — they're rare in subagent output.
//
// maxRunes counts in UNICODE CODE POINTS, so CJK characters stay intact at the
// truncation boundary. (Byte counting would split UTF-8 mid-rune → mojibake.)
func newSubagentDispatchHook(maxRunes int) agent.PostToolUseHook {
	return func(ctx context.Context, in *agent.PostToolUseInput) (*agent.PostToolUseOutput, error) {
		if in.ToolName != "dispatch_subagent" || in.Output == nil {
			return &agent.PostToolUseOutput{}, nil
		}
		truncated, didTruncate := truncateBlocks(in.Output.Content, maxRunes)
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

// truncateBlocks walks blocks, accumulating text-block runes against the
// remaining budget. Returns the truncated slice plus a bool indicating whether
// truncation actually happened.
func truncateBlocks(blocks []agent.ContentBlock, maxRunes int) ([]agent.ContentBlock, bool) {
	out := make([]agent.ContentBlock, 0, len(blocks))
	remaining := maxRunes
	truncated := false
	for _, b := range blocks {
		tb, ok := b.(agent.TextBlock)
		if !ok {
			out = append(out, b)
			continue
		}
		runes := []rune(tb.Text)
		if len(runes) > remaining {
			out = append(out, agent.TextBlock{Text: string(runes[:remaining]) + subagentTruncateSuffix})
			truncated = true
			break
		}
		out = append(out, tb)
		remaining -= len(runes)
	}
	return out, truncated
}
