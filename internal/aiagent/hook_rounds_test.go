package aiagent

import (
	"context"
	"testing"

	"github.com/cago-frame/agents/agent"
)

func TestRoundsHook_StopsAtCap(t *testing.T) {
	hook := makeRoundsHook(2) // cap=2

	for i := 0; i < 2; i++ {
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
