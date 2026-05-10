package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
)

func TestRoundsCounter_StopsAtCap(t *testing.T) {
	c := newRoundsCounter(2)
	hook := c.Hook()

	for i := range 2 {
		out, err := hook(context.Background(), agent.HookInput{Stage: agent.StagePreToolUse})
		if err != nil {
			t.Fatal(err)
		}
		if out != nil && out.Continue != nil && !*out.Continue {
			t.Fatalf("turn %d should not stop", i)
		}
	}

	out, err := hook(context.Background(), agent.HookInput{Stage: agent.StagePreToolUse})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.Continue == nil || *out.Continue {
		t.Fatalf("expected StopRun on 3rd call, got %+v", out)
	}
}

func TestRoundsCounter_ResetClearsCount(t *testing.T) {
	c := newRoundsCounter(1)
	hook := c.Hook()

	// Use the only allowed round.
	if _, err := hook(context.Background(), agent.HookInput{Stage: agent.StagePreToolUse}); err != nil {
		t.Fatal(err)
	}
	// Cap exceeded — must StopRun.
	out, err := hook(context.Background(), agent.HookInput{Stage: agent.StagePreToolUse})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.Continue == nil || *out.Continue {
		t.Fatalf("expected StopRun before reset, got %+v", out)
	}

	c.Reset()

	// After reset the next call must pass.
	out, err = hook(context.Background(), agent.HookInput{Stage: agent.StagePreToolUse})
	if err != nil {
		t.Fatal(err)
	}
	if out != nil && out.Continue != nil && !*out.Continue {
		t.Fatalf("after Reset the next call should not stop, got %+v", out)
	}
}
