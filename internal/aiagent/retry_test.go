package aiagent

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/provider/providertest"

	"github.com/opskat/opskat/internal/ai"
)

// allowedJitterRange returns the [lo, hi] window calcRetryDelay can produce
// for a given base, given the ±20% jitter (multiplier in [0.8, 1.2)).
func allowedJitterRange(base time.Duration) (lo, hi time.Duration) {
	return time.Duration(float64(base) * 0.8), time.Duration(float64(base) * 1.2)
}

func TestCalcRetryDelay_FollowsScheduleWithJitter(t *testing.T) {
	cases := []struct {
		attempt int
		base    time.Duration
	}{
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 15 * time.Second},
		{5, 15 * time.Second},
		{10, 15 * time.Second},
		{12, 15 * time.Second}, // overflow clamps to last
	}
	for _, c := range cases {
		got := calcRetryDelay(c.attempt, errors.New("transient"))
		lo, hi := allowedJitterRange(c.base)
		if got < lo || got > hi {
			t.Errorf("calcRetryDelay(%d): got %v, want in [%v, %v]", c.attempt, got, lo, hi)
		}
	}
}

func TestCalcRetryDelay_HonorsRetryAfter(t *testing.T) {
	pe := &provider.ProviderError{
		Err:        errors.New("rate limited"),
		RetryAfter: "7",
		StatusCode: http.StatusTooManyRequests,
	}
	got := calcRetryDelay(1, pe)
	lo, hi := allowedJitterRange(7 * time.Second)
	if got < lo || got > hi {
		t.Errorf("Retry-After=7: got %v, want in [%v, %v]", got, lo, hi)
	}
}

func TestCalcRetryDelay_IgnoresEmptyOrInvalidRetryAfter(t *testing.T) {
	for _, ra := range []string{"", "notanumber", "0", "-1"} {
		pe := &provider.ProviderError{
			Err:        errors.New("x"),
			RetryAfter: ra,
			StatusCode: 429,
		}
		got := calcRetryDelay(1, pe)
		lo, hi := allowedJitterRange(2 * time.Second)
		if got < lo || got > hi {
			t.Errorf("RetryAfter=%q: got %v, expected schedule fallback in [%v, %v]", ra, got, lo, hi)
		}
	}
}

// newRetryTestSession spins up a minimal cago Session backed by the given mock
// provider. No tools, no hooks — RunWithRetry only cares that sess.Stream can
// be called and that the mock controls success/failure.
func newRetryTestSession(t *testing.T, mock *providertest.Mock) *agent.Session {
	t.Helper()
	a := agent.NewWithBackend(agent.NewBuiltinBackend(mock))
	return a.Session()
}

// drainAll consumes every Stream event so Stream.Result() returns the final
// outcome. Mirrors the drain pattern in System.Stream.
func drainAll(stream *agent.Stream) {
	for stream.Next() {
		_ = stream.Event()
	}
}

// TestRunWithRetry_NonRetryableShortCircuits verifies that a 4xx (non-429)
// ProviderError stops after the first attempt with no "retry" event emitted.
// Without this guard, a misconfigured request (e.g. bad auth) would burn the
// full 10-attempt budget for ~90 seconds before failing.
func TestRunWithRetry_NonRetryableShortCircuits(t *testing.T) {
	wantErr := &provider.ProviderError{StatusCode: 400, Err: errors.New("bad request")}
	mock := providertest.New().QueueError(wantErr)
	sess := newRetryTestSession(t, mock)
	rec := &recordEmitter{}

	start := time.Now()
	_, err := RunWithRetry(context.Background(), sess, "p", rec, 1, drainAll)
	if err == nil {
		t.Fatal("expected error for non-retryable status")
	}
	var pe *provider.ProviderError
	if !errors.As(err, &pe) || pe.StatusCode != 400 {
		t.Fatalf("error type lost: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("non-retryable should not wait, took %v", time.Since(start))
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, ev := range rec.events {
		if ev.Type == "retry" {
			t.Errorf("non-retryable error must not emit retry event, got %+v", ev)
		}
	}
}

// TestRunWithRetry_RetryableThenSuccess scripts a 503 followed by a clean
// stream and asserts (a) RunWithRetry eventually succeeds, (b) at least one
// "retry" event was emitted with the proper "1/10" content. Uses a single
// retry so the test budget stays under 4s (delay[0] = 2s ±20%).
func TestRunWithRetry_RetryableThenSuccess(t *testing.T) {
	mock := providertest.New().
		QueueError(&provider.ProviderError{StatusCode: 503, Err: errors.New("upstream down")}).
		QueueStream(provider.StreamChunk{ContentDelta: "ok"}, provider.StreamChunk{FinishReason: provider.FinishStop})
	sess := newRetryTestSession(t, mock)
	rec := &recordEmitter{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RunWithRetry(ctx, sess, "p", rec, 1, drainAll)
	if err != nil {
		t.Fatalf("RunWithRetry: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var sawRetry bool
	for _, ev := range rec.events {
		if ev.Type == "retry" {
			sawRetry = true
			if ev.Content != "1/10" {
				t.Errorf("retry counter = %q, want 1/10", ev.Content)
			}
			if ev.Error == "" {
				t.Error("retry event missing Error field for diagnostics")
			}
		}
	}
	if !sawRetry {
		t.Error("expected a retry event before success")
	}
}

// TestRunWithRetry_CtxCancelDuringBackoff verifies the select{} on ctx.Done
// inside the retry sleep returns ctx.Err() promptly instead of waiting out the
// full backoff. Cancellation is the user's "Stop" button; it must take effect
// even if the agent is mid-backoff.
func TestRunWithRetry_CtxCancelDuringBackoff(t *testing.T) {
	mock := providertest.New().
		QueueError(&provider.ProviderError{StatusCode: 503, Err: errors.New("flapping")})
	sess := newRetryTestSession(t, mock)
	rec := &recordEmitter{}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel ~50ms in — well before the 1.6s minimum backoff.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := RunWithRetry(ctx, sess, "p", rec, 1, drainAll)
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if elapsed > time.Second {
		t.Errorf("ctx cancel did not interrupt backoff in time, took %v", elapsed)
	}
	// One retry event should still have been emitted before the cancel hit.
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var sawRetry bool
	for _, ev := range rec.events {
		if ev.Type == "retry" {
			sawRetry = true
		}
	}
	if !sawRetry {
		t.Error("retry event should be emitted before ctx-cancel exits the wait")
	}
}

// staticEmitter is a no-op EventEmitter used where event capture is irrelevant.
var _ EventEmitter = EmitterFunc(func(int64, ai.StreamEvent) {})

func TestShouldRetry(t *testing.T) {
	cases := []struct {
		err  error
		want bool
		desc string
	}{
		{&provider.ProviderError{StatusCode: 429}, true, "429 retries"},
		{&provider.ProviderError{StatusCode: 500}, true, "500 retries"},
		{&provider.ProviderError{StatusCode: 503}, true, "503 retries"},
		{&provider.ProviderError{StatusCode: 400}, false, "4xx (non-429) doesn't retry"},
		{&provider.ProviderError{StatusCode: 401}, false, "auth doesn't retry"},
		{errors.New("transient"), true, "non-ProviderError retries (permissive)"},
	}
	for _, c := range cases {
		if got := shouldRetry(c.err); got != c.want {
			t.Errorf("%s: shouldRetry(%v) = %v, want %v", c.desc, c.err, got, c.want)
		}
	}
}
