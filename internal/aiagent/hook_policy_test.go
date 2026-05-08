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

// TestPolicyHook_NeedConfirm_AllowFromUser exercises the NeedConfirm → gateway →
// allow path: the user approves once, hook converts the result to Allow, plants
// it in the sidecar, and returns nil (no Deny).
func TestPolicyHook_NeedConfirm_AllowFromUser(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{resp: ai.ApprovalResponse{Decision: "allow"}}
	hook := makePolicyHook(&Deps{}, sc, gw, newFakeChecker(ai.CheckResult{
		Decision: ai.NeedConfirm,
	}))

	out, err := hook(context.Background(), agent.HookInput{
		Stage:      agent.StagePreToolUse,
		ToolName:   "run_command",
		ToolInput:  json.RawMessage(`{"asset_id":2,"command":"ls"}`),
		ToolCallID: "call_nc1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gw.called {
		t.Fatal("approval gateway was not invoked for NeedConfirm")
	}
	if out != nil && out.Decision == agent.DecisionDeny {
		t.Fatalf("user-allow must not Deny, got %+v", out)
	}
	r := sc.drain("call_nc1")
	if r == nil || r.Decision != ai.Allow || r.DecisionSource != ai.SourceUserAllow {
		t.Fatalf("sidecar should hold user-allow, got %+v", r)
	}
}

// TestPolicyHook_NeedConfirm_AllowAllPersistsGrant verifies the allowAll branch
// reaches saveGrantPatternFromResponse (currently a stub) and still allows the
// tool call. Whatever side effects that stub gains later, this test pins down
// the contract that the hook returns nil and writes Allow to the sidecar.
func TestPolicyHook_NeedConfirm_AllowAllPersistsGrant(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{resp: ai.ApprovalResponse{Decision: "allowAll"}}
	hook := makePolicyHook(&Deps{}, sc, gw, newFakeChecker(ai.CheckResult{
		Decision: ai.NeedConfirm,
	}))

	out, err := hook(context.Background(), agent.HookInput{
		Stage:      agent.StagePreToolUse,
		ToolName:   "run_command",
		ToolInput:  json.RawMessage(`{"asset_id":3,"command":"uptime"}`),
		ToolCallID: "call_nc2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != nil && out.Decision == agent.DecisionDeny {
		t.Fatal("allowAll must not Deny")
	}
	if r := sc.drain("call_nc2"); r == nil || r.Decision != ai.Allow {
		t.Fatalf("sidecar should hold Allow on allowAll, got %+v", r)
	}
}

// TestPolicyHook_NeedConfirm_DenyFromUser covers the user-deny branch. The hook
// must Deny and plant SourceUserDeny so the audit log records intent.
func TestPolicyHook_NeedConfirm_DenyFromUser(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{resp: ai.ApprovalResponse{Decision: "deny"}}
	hook := makePolicyHook(&Deps{}, sc, gw, newFakeChecker(ai.CheckResult{
		Decision: ai.NeedConfirm,
	}))

	out, err := hook(context.Background(), agent.HookInput{
		Stage:      agent.StagePreToolUse,
		ToolName:   "run_command",
		ToolInput:  json.RawMessage(`{"asset_id":4,"command":"reboot"}`),
		ToolCallID: "call_nc3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.Decision != agent.DecisionDeny {
		t.Fatalf("expected Deny, got %+v", out)
	}
	if out.Reason != "user denied" {
		t.Fatalf("reason = %q, want %q", out.Reason, "user denied")
	}
	r := sc.drain("call_nc3")
	if r == nil || r.Decision != ai.Deny || r.DecisionSource != ai.SourceUserDeny {
		t.Fatalf("sidecar should hold user-deny, got %+v", r)
	}
}

// TestExtractAssetAndCommand_AllPolicyTools is a table-driven check that every
// command-execution tool name is recognized and produces the expected
// (assetID, summary, kind) triple. Catches typos in tool-name dispatch and
// argument-key drift.
func TestExtractAssetAndCommand_AllPolicyTools(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantAsset int64
		wantSum   string
		wantKind  string
	}{
		{"run_command", `{"asset_id":1,"command":"ls"}`, 1, "ls", "exec"},
		{"exec_sql", `{"asset_id":2,"sql":"select 1"}`, 2, "select 1", "sql"},
		{"exec_redis", `{"asset_id":3,"command":"GET k"}`, 3, "GET k", "redis"},
		{"exec_mongo", `{"asset_id":4,"operation":"find users"}`, 4, "find users", "mongo"},
		{"exec_k8s", `{"asset_id":5,"command":"get pods"}`, 5, "get pods", "k8s"},
		{"upload_file", `{"asset_id":6,"local_path":"/a","remote_path":"/b"}`, 6, "upload /a → /b", "cp"},
		{"download_file", `{"asset_id":7,"local_path":"/a","remote_path":"/b"}`, 7, "download /b → /a", "cp"},
		{"kafka_cluster", `{"asset_id":8,"operation":"list","topic":""}`, 8, "list:", "kafka"},
		{"kafka_topic", `{"asset_id":9,"operation":"describe","topic":"t"}`, 9, "describe:t", "kafka"},
		{"kafka_consumer_group", `{"asset_id":10,"operation":"list","topic":""}`, 10, "list:", "kafka"},
		{"kafka_acl", `{"asset_id":11,"operation":"list","topic":""}`, 11, "list:", "kafka"},
		{"kafka_schema", `{"asset_id":12,"operation":"list","topic":""}`, 12, "list:", "kafka"},
		{"kafka_connect", `{"asset_id":13,"operation":"list","topic":""}`, 13, "list:", "kafka"},
		{"kafka_message", `{"asset_id":14,"operation":"produce","topic":"t"}`, 14, "produce:t", "kafka"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, sum, kind, ok := extractAssetAndCommand(c.name, json.RawMessage(c.input))
			if !ok {
				t.Fatalf("%s: not recognized as policy-gated", c.name)
			}
			if id != c.wantAsset {
				t.Errorf("assetID = %d, want %d", id, c.wantAsset)
			}
			if sum != c.wantSum {
				t.Errorf("summary = %q, want %q", sum, c.wantSum)
			}
			if kind != c.wantKind {
				t.Errorf("kind = %q, want %q", kind, c.wantKind)
			}
		})
	}
}

// TestExtractAssetAndCommand_UnknownToolNotGated guards the default branch:
// a tool name not in the policy switch returns ok=false so the hook short-
// circuits without calling the checker.
func TestExtractAssetAndCommand_UnknownToolNotGated(t *testing.T) {
	_, _, _, ok := extractAssetAndCommand("list_assets", json.RawMessage(`{}`))
	if ok {
		t.Fatal("non-policy tool must return ok=false")
	}
}

// TestExtractAssetAndCommand_AssetIDNumberCoercions checks the getNum helper
// handles JSON numbers (always float64 after Unmarshal) and missing keys
// without panicking.
func TestExtractAssetAndCommand_AssetIDNumberCoercions(t *testing.T) {
	id, _, _, ok := extractAssetAndCommand("run_command", json.RawMessage(`{"asset_id":42,"command":"x"}`))
	if !ok || id != 42 {
		t.Fatalf("float64 asset_id: id=%d ok=%v", id, ok)
	}
	id, _, _, ok = extractAssetAndCommand("run_command", json.RawMessage(`{"command":"x"}`))
	if !ok || id != 0 {
		t.Fatalf("missing asset_id should default to 0, got %d ok=%v", id, ok)
	}
}
