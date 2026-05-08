package aiagent

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/cago-frame/agents/agent"
)

// makeRoundsHook returns a PreToolUse hook that emits StopRun after maxRounds
// invocations on the same hook instance. Each Stream gets its own hook
// instance via System.Stream so counts don't leak across runs.
func makeRoundsHook(maxRounds int) agent.HookFunc {
	if maxRounds <= 0 {
		maxRounds = 50
	}
	var n int64
	return func(_ context.Context, _ agent.HookInput) (*agent.HookOutput, error) {
		current := atomic.AddInt64(&n, 1)
		if current > int64(maxRounds) {
			return agent.StopRun(fmt.Sprintf("max rounds (%d) reached", maxRounds)), nil
		}
		return nil, nil
	}
}
