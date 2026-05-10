package aiagent

import (
	"context"

	"github.com/cago-frame/agents/agent"
)

// PolicyChecker is a thin abstraction over OpsKat's existing command-policy
// layer. The hook tests it via a fake; production wiring (Task 21) builds an
// adapter around `internal/ai.CheckPermission` that extracts assetType /
// assetID / command from the cago tool input map.
type PolicyChecker interface {
	Check(ctx context.Context, toolName string, input map[string]any) (allowed bool, reason string, err error)
}

// newPolicyHook returns a PreToolUse hook that gates every tool call through
// PolicyChecker. Allowed → DecisionPass; denied → DecisionDeny + reason. A
// non-nil error from the checker propagates as a hook error (cago surfaces it
// as EventError + StopHook).
func newPolicyHook(c PolicyChecker) agent.PreToolUseHook {
	return func(ctx context.Context, in *agent.PreToolUseInput) (*agent.PreToolUseOutput, error) {
		allowed, reason, err := c.Check(ctx, in.ToolName, in.Input)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return &agent.PreToolUseOutput{Decision: agent.DecisionDeny, DenyReason: reason}, nil
		}
		return &agent.PreToolUseOutput{Decision: agent.DecisionPass}, nil
	}
}
