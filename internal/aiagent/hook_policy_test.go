package aiagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// fakeChecker implements PolicyChecker by returning a fixed result.
type fakeChecker struct{ result ai.CheckResult }

func newFakeChecker(r ai.CheckResult) *fakeChecker { return &fakeChecker{result: r} }
func (f *fakeChecker) Check(_ context.Context, _ int64, _ string) ai.CheckResult {
	return f.result
}

// fakeApprovalRequester captures RequestSingle calls and returns a fixed response.
type fakeApprovalRequester struct {
	called bool
	resp   ai.ApprovalResponse
}

func (f *fakeApprovalRequester) RequestSingle(_ context.Context, _ int64, _ string,
	_ []ai.ApprovalItem, _ string) ai.ApprovalResponse {
	f.called = true
	return f.resp
}

func TestPolicyHook_AllowResultPlantedInSidecar(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{}
	hook := makePolicyHook(&Deps{}, sc, gw, newFakeChecker(ai.CheckResult{
		Decision: ai.Allow, DecisionSource: ai.SourcePolicyAllow,
	}))

	out, err := hook(context.Background(), agent.HookInput{
		Stage:      agent.StagePreToolUse,
		ToolName:   "run_command",
		ToolInput:  json.RawMessage(`{"asset_id":1,"command":"ls /tmp"}`),
		ToolCallID: "call_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != nil && out.Decision == agent.DecisionDeny {
		t.Fatal("Allow path must not Deny")
	}
	if r := sc.drain("call_1"); r == nil || r.Decision != ai.Allow {
		t.Fatalf("sidecar lost CheckResult: %+v", r)
	}
}

func TestPolicyHook_DenyShortCircuits(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{}
	hook := makePolicyHook(&Deps{}, sc, gw, newFakeChecker(ai.CheckResult{
		Decision: ai.Deny, Message: "nope",
	}))

	out, err := hook(context.Background(), agent.HookInput{
		Stage:      agent.StagePreToolUse,
		ToolName:   "run_command",
		ToolInput:  json.RawMessage(`{"asset_id":1,"command":"rm -rf /"}`),
		ToolCallID: "call_x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.Decision != agent.DecisionDeny {
		t.Fatalf("expected Deny, got %+v", out)
	}
	if out.Reason != "nope" {
		t.Fatalf("reason = %q", out.Reason)
	}
}

func TestPolicyHook_NonPolicyToolPasses(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{}
	hook := makePolicyHook(&Deps{}, sc, gw, newFakeChecker(ai.CheckResult{Decision: ai.Allow}))

	out, err := hook(context.Background(), agent.HookInput{
		Stage:      agent.StagePreToolUse,
		ToolName:   "list_assets", // not a command-execution tool
		ToolInput:  json.RawMessage(`{}`),
		ToolCallID: "call_y",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("non-policy tool should return nil HookOutput, got %+v", out)
	}
}
