package aiagent

import (
	"context"
	"encoding/json"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// makeAuditHook returns a PostToolUse hook that reads the CheckResult planted
// by the policy hook and writes a fire-and-forget audit row.
func makeAuditHook(sc *sidecar, writer ai.AuditWriter) agent.HookFunc {
	return func(ctx context.Context, in agent.HookInput) (*agent.HookOutput, error) {
		decision := sc.drain(in.ToolCallID)

		var result string
		_ = json.Unmarshal(in.ToolResponse, &result)

		go writer.WriteToolCall(ctx, ai.ToolCallInfo{
			ToolName: in.ToolName,
			ArgsJSON: string(in.ToolInput),
			Result:   result,
			Decision: decision,
		})
		return nil, nil
	}
}
