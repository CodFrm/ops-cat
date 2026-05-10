package aiagent

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/cago-frame/agents/agent"
)

// roundsCounter is a per-turn PreToolUse cap. NewSystem installs the hook once
// on the parent agent (cago wires hooks at construction time and they cannot
// be swapped per Stream); a paired SessionStart hook calls Reset so the budget
// is per-turn, not per-Conversation. Without auto-reset the closure counter
// would carry across turns and eventually StopRun every PreToolUse, surfacing
// to the user as "tool block stuck running" because the frontend sees a
// tool_start with no matching tool_result.
type roundsCounter struct {
	n         atomic.Int64
	maxRounds int
}

// newRoundsCounter constructs a counter with the given per-turn cap.
// maxRounds <= 0 falls back to 50 (matches the legacy default).
func newRoundsCounter(maxRounds int) *roundsCounter {
	if maxRounds <= 0 {
		maxRounds = 50
	}
	return &roundsCounter{maxRounds: maxRounds}
}

// Reset zeroes the counter. Called by the auto-reset SessionStart hook (and
// available to tests).
func (c *roundsCounter) Reset() { c.n.Store(0) }

// Hook returns the PreToolUse HookFunc.
func (c *roundsCounter) Hook() agent.HookFunc {
	return func(_ context.Context, _ agent.HookInput) (*agent.HookOutput, error) {
		current := c.n.Add(1)
		if current > int64(c.maxRounds) {
			return agent.StopRun(fmt.Sprintf("max rounds (%d) reached", c.maxRounds)), nil
		}
		return nil, nil
	}
}

// ResetHook returns a SessionStart HookFunc that calls Reset. Register
// alongside Hook so the cap is enforced per Stream, not per Conversation.
func (c *roundsCounter) ResetHook() agent.HookFunc {
	return func(_ context.Context, _ agent.HookInput) (*agent.HookOutput, error) {
		c.Reset()
		return nil, nil
	}
}
