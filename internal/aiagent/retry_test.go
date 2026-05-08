package aiagent

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/cago-frame/agents/provider"
)

// allowedJitterRange returns the [min, max] window calcRetryDelay can produce
// for a given base, given the ±20% jitter (multiplier in [0.8, 1.2)).
func allowedJitterRange(base time.Duration) (min, max time.Duration) {
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
		min, max := allowedJitterRange(c.base)
		if got < min || got > max {
			t.Errorf("calcRetryDelay(%d): got %v, want in [%v, %v]", c.attempt, got, min, max)
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
	min, max := allowedJitterRange(7 * time.Second)
	if got < min || got > max {
		t.Errorf("Retry-After=7: got %v, want in [%v, %v]", got, min, max)
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
		min, max := allowedJitterRange(2 * time.Second)
		if got < min || got > max {
			t.Errorf("RetryAfter=%q: got %v, expected schedule fallback in [%v, %v]", ra, got, min, max)
		}
	}
}

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
