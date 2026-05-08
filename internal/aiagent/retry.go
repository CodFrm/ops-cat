package aiagent

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"

	"github.com/opskat/opskat/internal/ai"
)

// MaxRetries is the maximum number of retry attempts on transient errors.
// Matches the legacy internal/ai/conversation_runner.go schedule.
const MaxRetries = 10

// retryDelays is the per-attempt base delay (1-indexed). Beyond len-1 it clamps
// to the last value. ±20% jitter is applied at call time.
var retryDelays = []time.Duration{
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
	15 * time.Second,
	15 * time.Second,
	15 * time.Second,
	15 * time.Second,
	15 * time.Second,
	15 * time.Second,
	15 * time.Second,
}

// calcRetryDelay returns the wait before retry attempt `attempt` (1-indexed).
// If the error carries a Retry-After hint (Anthropic 429), it overrides the
// schedule. Always applies ±20% jitter.
func calcRetryDelay(attempt int, err error) time.Duration {
	var pe *provider.ProviderError
	if errors.As(err, &pe) && pe.RetryAfter != "" {
		if seconds, parseErr := strconv.Atoi(pe.RetryAfter); parseErr == nil && seconds > 0 {
			return addJitter(time.Duration(seconds) * time.Second)
		}
	}
	idx := attempt - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(retryDelays) {
		idx = len(retryDelays) - 1
	}
	return addJitter(retryDelays[idx])
}

// addJitter applies ±20% jitter — multiplier in [0.8, 1.2).
func addJitter(base time.Duration) time.Duration {
	mult := 0.8 + rand.Float64()*0.4
	return time.Duration(float64(base) * mult)
}

// shouldRetry reports whether an error from sess.Stream / stream.Result
// warrants another attempt. Non-provider errors are retried (permissive — let
// the retry budget run out instead of failing fast on unexpected types).
// ProviderError is retried only on 429 or 5xx.
func shouldRetry(err error) bool {
	var pe *provider.ProviderError
	if errors.As(err, &pe) {
		return pe.StatusCode == 429 || pe.StatusCode >= 500
	}
	return true
}

// RunWithRetry wraps sess.Stream(prompt) with the legacy retry policy.
// Between attempts it emits a "retry" StreamEvent via em. The drain callback
// runs once per successful Stream-open and is responsible for consuming events
// (typically piping them through the bridge). RunWithRetry then awaits
// stream.Result() and retries on errors.
//
// On ctx cancel, returns ctx.Err() immediately. The final error is the last
// err observed; *agent.Result is non-nil if any attempt produced one.
func RunWithRetry(
	ctx context.Context,
	sess *agent.Session,
	prompt string,
	em EventEmitter,
	convID int64,
	drain func(stream *agent.Stream),
) (*agent.Result, error) {
	var lastErr error
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		stream, err := sess.Stream(ctx, prompt)
		if err != nil {
			lastErr = err
			if !shouldRetry(err) || attempt == MaxRetries {
				return nil, err
			}
		} else {
			drain(stream)
			res, runErr := stream.Result()
			if runErr == nil {
				return res, nil
			}
			lastErr = runErr
			if !shouldRetry(runErr) || attempt == MaxRetries {
				return res, runErr
			}
		}

		delay := calcRetryDelay(attempt, lastErr)
		em.Emit(convID, ai.StreamEvent{
			Type:    "retry",
			Content: fmt.Sprintf("%d/%d", attempt, MaxRetries),
			Error:   lastErr.Error(),
		})
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}
