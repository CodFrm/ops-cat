package aiagent

import (
	"context"
	"sync/atomic"

	"github.com/cago-frame/agents/agent"
)

// roundsCounter caps the number of PreToolUse invocations per turn. Manager
// constructs one per ConvHandle and registers two hooks:
//   - rc.Hook() as a PreToolUse hook (cap enforcement)
//   - an OnRunnerStart hook calling rc.Reset() (clear budget at turn start)
//
// maxRounds==0 disables the cap (every call passes); negative values are treated as
// 0 for safety.
type roundsCounter struct {
	used      int32
	maxRounds int32
}

func newRoundsCounter(maxRounds int) *roundsCounter {
	if maxRounds < 0 {
		maxRounds = 0
	}
	return &roundsCounter{maxRounds: int32(maxRounds)}
}

// Reset zeroes the budget. Safe to call concurrently with Hook invocations.
func (rc *roundsCounter) Reset() {
	atomic.StoreInt32(&rc.used, 0)
}

// ResetHook returns an OnRunnerStart-compatible function that calls Reset.
// Retained for v1 consumers (system.go) until Task 19 replaces them.
func (rc *roundsCounter) ResetHook() func(ctx context.Context, r *agent.Runner) error {
	return func(_ context.Context, _ *agent.Runner) error {
		rc.Reset()
		return nil
	}
}

// Hook returns the cago PreToolUseHook. Each invocation atomically increments
// the used counter; when used > max (and max > 0), returns DecisionDeny.
func (rc *roundsCounter) Hook() agent.PreToolUseHook {
	return func(_ context.Context, _ *agent.PreToolUseInput) (*agent.PreToolUseOutput, error) {
		used := atomic.AddInt32(&rc.used, 1)
		if rc.maxRounds > 0 && used > rc.maxRounds {
			return &agent.PreToolUseOutput{
				Decision:   agent.DecisionDeny,
				DenyReason: "已达回合上限",
			}, nil
		}
		return &agent.PreToolUseOutput{Decision: agent.DecisionPass}, nil
	}
}
