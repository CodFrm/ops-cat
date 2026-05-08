package aiagent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

type fakeAuditWriter struct {
	mu  sync.Mutex
	got ai.ToolCallInfo
}

func (f *fakeAuditWriter) WriteToolCall(_ context.Context, info ai.ToolCallInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = info
}

func (f *fakeAuditWriter) waitGot() ai.ToolCallInfo {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		got := f.got
		f.mu.Unlock()
		if got.ToolName != "" {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	return ai.ToolCallInfo{}
}

func TestAuditHook_DrainsSidecarAndWritesAudit(t *testing.T) {
	sc := newSidecar()
	sc.put("call_42", &ai.CheckResult{
		Decision: ai.Allow, MatchedPattern: "ls *", DecisionSource: ai.SourcePolicyAllow,
	})
	w := &fakeAuditWriter{}
	hook := makeAuditHook(sc, w)

	_, err := hook(context.Background(), agent.HookInput{
		Stage:        agent.StagePostToolUse,
		ToolName:     "run_command",
		ToolInput:    json.RawMessage(`{"asset_id":1,"command":"ls /"}`),
		ToolResponse: json.RawMessage(`"output here"`),
		ToolCallID:   "call_42",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := w.waitGot()
	if got.ToolName != "run_command" {
		t.Errorf("ToolName = %q", got.ToolName)
	}
	if got.Decision == nil || got.Decision.Decision != ai.Allow {
		t.Errorf("decision lost: %+v", got.Decision)
	}
	if got.Decision != nil && got.Decision.MatchedPattern != "ls *" {
		t.Errorf("pattern lost: %+v", got.Decision)
	}
}
