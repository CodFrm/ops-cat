package aiagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

// stubLookupAssetName 在测试期把 lookupAssetName 替换成不打 DB 的版本。defer 还原，
// 让相邻测试之间不串。
func stubLookupAssetName(t *testing.T, name string) {
	t.Helper()
	prev := lookupAssetName
	lookupAssetName = func(_ context.Context, _ int64) string { return name }
	t.Cleanup(func() { lookupAssetName = prev })
}

// fakeCheckPerm 返回固定 CheckResult，规避 ai.CheckPermission 拉起 DB / asset_svc。
// callKind/callAssetID 记录最后一次调用的入参，用于断言 kind→assetType 映射正确。
type fakeCheckPerm struct {
	result        ai.CheckResult
	callAssetType string
	callAssetID   int64
	callCommand   string
	calls         int
}

func (f *fakeCheckPerm) fn(_ context.Context, assetType string, assetID int64, command string) ai.CheckResult {
	f.calls++
	f.callAssetType = assetType
	f.callAssetID = assetID
	f.callCommand = command
	return f.result
}

// fakeApprovalRequester captures RequestSingle calls and returns a fixed response.
type fakeApprovalRequester struct {
	called    bool
	gotItems  []ai.ApprovalItem
	gotKind   string
	gotConvID int64
	resp      ai.ApprovalResponse
}

func (f *fakeApprovalRequester) RequestSingle(_ context.Context, convID int64, kind string,
	items []ai.ApprovalItem, _ string) ai.ApprovalResponse {
	f.called = true
	f.gotConvID = convID
	f.gotKind = kind
	f.gotItems = items
	return f.resp
}

func TestPolicyHook_AllowResultPlantedInSidecar(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{}
	check := &fakeCheckPerm{result: ai.CheckResult{
		Decision: ai.Allow, DecisionSource: ai.SourcePolicyAllow,
	}}
	hook := makePolicyHook(sc, gw, check.fn, nil)

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
	// run_command 必须被映射成 SSH 类型给 ai.CheckPermission；这条断言看住 kind→assetType 表的回归。
	if check.callAssetType != asset_entity.AssetTypeSSH {
		t.Errorf("assetType = %q, want %q", check.callAssetType, asset_entity.AssetTypeSSH)
	}
}

func TestPolicyHook_DenyShortCircuits(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{}
	check := &fakeCheckPerm{result: ai.CheckResult{
		Decision: ai.Deny, Message: "nope",
	}}
	hook := makePolicyHook(sc, gw, check.fn, nil)

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
	if gw.called {
		t.Fatal("Deny path must not invoke approval gateway")
	}
}

func TestPolicyHook_NonPolicyToolPasses(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{}
	check := &fakeCheckPerm{result: ai.CheckResult{Decision: ai.Allow}}
	hook := makePolicyHook(sc, gw, check.fn, nil)

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
	if check.calls != 0 {
		t.Fatal("非 policy 工具不应调用 checkPerm")
	}
}

// TestPolicyHook_NeedConfirm_AllowFromUser 走完 NeedConfirm → 网关 → user allow 全链路：
// hook 把结果转成 Allow、写 sidecar、返回 nil；同时核对发给 gw 的 items 带了 Type/AssetID/Command。
func TestPolicyHook_NeedConfirm_AllowFromUser(t *testing.T) {
	stubLookupAssetName(t, "asset-2")
	sc := newSidecar()
	gw := &fakeApprovalRequester{resp: ai.ApprovalResponse{Decision: "allow"}}
	check := &fakeCheckPerm{result: ai.CheckResult{Decision: ai.NeedConfirm}}
	hook := makePolicyHook(sc, gw, check.fn, nil)

	ctx := WithConvID(context.Background(), 42)
	out, err := hook(ctx, agent.HookInput{
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
	if gw.gotConvID != 42 {
		t.Errorf("gw 收到 convID=%d，期望 42（来自 ctx WithConvID）", gw.gotConvID)
	}
	if len(gw.gotItems) != 1 || gw.gotItems[0].Type != "exec" ||
		gw.gotItems[0].AssetID != 2 || gw.gotItems[0].Command != "ls" {
		t.Errorf("gw items = %+v, 期望 [{Type:exec, AssetID:2, Command:ls}]", gw.gotItems)
	}
	if out != nil && out.Decision == agent.DecisionDeny {
		t.Fatalf("user-allow must not Deny, got %+v", out)
	}
	r := sc.drain("call_nc1")
	if r == nil || r.Decision != ai.Allow || r.DecisionSource != ai.SourceUserAllow {
		t.Fatalf("sidecar should hold user-allow, got %+v", r)
	}
}

// TestPolicyHook_NeedConfirm_AllowAllAllowsTool 验证 allowAll 不 Deny；grant 落库本身在 DB 集成
// 测里看（asset_svc + grant_repo），单测只看 hook 自己的契约：sidecar=Allow，HookOutput≠Deny。
func TestPolicyHook_NeedConfirm_AllowAllAllowsTool(t *testing.T) {
	stubLookupAssetName(t, "asset-3")
	sc := newSidecar()
	gw := &fakeApprovalRequester{resp: ai.ApprovalResponse{Decision: "allowAll"}}
	check := &fakeCheckPerm{result: ai.CheckResult{Decision: ai.NeedConfirm}}
	hook := makePolicyHook(sc, gw, check.fn, nil)

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

// TestPolicyHook_NeedConfirm_DenyFromUser 覆盖用户 deny 分支。
func TestPolicyHook_NeedConfirm_DenyFromUser(t *testing.T) {
	stubLookupAssetName(t, "asset-4")
	sc := newSidecar()
	gw := &fakeApprovalRequester{resp: ai.ApprovalResponse{Decision: "deny"}}
	check := &fakeCheckPerm{result: ai.CheckResult{Decision: ai.NeedConfirm}}
	hook := makePolicyHook(sc, gw, check.fn, nil)

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

// TestAssetTypeForKind 锁住 kind→assetType 映射表，避免新增工具类型时漏改导致策略
// 走错检查路径（比如 redis 命令被当成 SSH 检查会全部 NeedConfirm）。
func TestAssetTypeForKind(t *testing.T) {
	cases := map[string]string{
		"exec":    asset_entity.AssetTypeSSH,
		"cp":      asset_entity.AssetTypeSSH,
		"sql":     asset_entity.AssetTypeDatabase,
		"redis":   asset_entity.AssetTypeRedis,
		"mongo":   asset_entity.AssetTypeMongoDB,
		"kafka":   asset_entity.AssetTypeKafka,
		"k8s":     asset_entity.AssetTypeK8s,
		"unknown": asset_entity.AssetTypeSSH, // 兜底
	}
	for kind, want := range cases {
		if got := assetTypeForKind(kind); got != want {
			t.Errorf("assetTypeForKind(%q) = %q, want %q", kind, got, want)
		}
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

// TestExtractAssetAndCommand_LocalTools checks cago built-in 工具被识别成 local_*
// kind，assetID=0；write / edit 用 path 当 summary，bash 用 command。
func TestExtractAssetAndCommand_LocalTools(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantSum  string
		wantKind string
	}{
		{"bash", `{"command":"ls -al"}`, "ls -al", kindLocalBash},
		{"write", `{"path":"/tmp/foo.txt","content":"x"}`, "/tmp/foo.txt", kindLocalWrite},
		{"edit", `{"path":"/tmp/foo.txt","edits":[]}`, "/tmp/foo.txt", kindLocalEdit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, sum, kind, ok := extractAssetAndCommand(c.name, json.RawMessage(c.input))
			if !ok {
				t.Fatalf("%s: not recognized", c.name)
			}
			if id != 0 {
				t.Errorf("local tool assetID should be 0, got %d", id)
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

// fakeLocalGrants 内存版 LocalGrantStore：grants 标记已保存的 (sessionID, toolName)。
type fakeLocalGrants struct {
	saved map[string]bool // key: sessionID|toolName
	hits  map[string]bool // 预先填好的"命中"集合
}

func newFakeLocalGrants() *fakeLocalGrants {
	return &fakeLocalGrants{saved: map[string]bool{}, hits: map[string]bool{}}
}

func (f *fakeLocalGrants) key(sessionID, toolName string) string {
	return sessionID + "|" + toolName
}

func (f *fakeLocalGrants) Has(_ context.Context, sessionID, toolName string) bool {
	return f.hits[f.key(sessionID, toolName)]
}

func (f *fakeLocalGrants) Save(_ context.Context, sessionID, toolName string) {
	f.saved[f.key(sessionID, toolName)] = true
}

// TestPolicyHook_LocalBash_AlwaysPrompts 说明 bash 永远走 RequestSingle，
// 无论 grants 里命中不命中（这条策略——bash 不能记忆——是用户明确要求的）。
func TestPolicyHook_LocalBash_AlwaysPrompts(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{resp: ai.ApprovalResponse{Decision: "allow"}}
	check := &fakeCheckPerm{result: ai.CheckResult{Decision: ai.Allow}}
	grants := newFakeLocalGrants()
	// 即使 bash 在 grants 中"命中"，hook 也必须忽略它继续走 RequestSingle。
	grants.hits[grants.key("conv_7", "bash")] = true

	hook := makePolicyHook(sc, gw, check.fn, grants)
	ctx := WithConvID(context.Background(), 7)
	out, err := hook(ctx, agent.HookInput{
		Stage:      agent.StagePreToolUse,
		ToolName:   "bash",
		ToolInput:  json.RawMessage(`{"command":"echo hi"}`),
		ToolCallID: "call_b1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gw.called {
		t.Fatal("bash 必须走 RequestSingle，即使 grants hit")
	}
	if check.calls != 0 {
		t.Fatal("local 工具不应调用 checkPerm")
	}
	if len(gw.gotItems) != 1 || gw.gotItems[0].Type != kindLocalBash {
		t.Errorf("gw items = %+v, want type=%s", gw.gotItems, kindLocalBash)
	}
	if out != nil && out.Decision == agent.DecisionDeny {
		t.Fatalf("user-allow must not Deny, got %+v", out)
	}
}

// TestPolicyHook_LocalBash_AllowAllDoesNotPersist 验证 bash 即使收到 allowAll
// 也不会写 grants（用户要求"bash 每次都要确认"）。
func TestPolicyHook_LocalBash_AllowAllDoesNotPersist(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{resp: ai.ApprovalResponse{Decision: "allowAll"}}
	check := &fakeCheckPerm{result: ai.CheckResult{Decision: ai.Allow}}
	grants := newFakeLocalGrants()

	hook := makePolicyHook(sc, gw, check.fn, grants)
	ctx := WithConvID(context.Background(), 9)
	if _, err := hook(ctx, agent.HookInput{
		Stage:      agent.StagePreToolUse,
		ToolName:   "bash",
		ToolInput:  json.RawMessage(`{"command":"uptime"}`),
		ToolCallID: "call_b2",
	}); err != nil {
		t.Fatal(err)
	}
	if len(grants.saved) != 0 {
		t.Fatalf("bash 不能落 grants，got %v", grants.saved)
	}
}

// TestPolicyHook_LocalWrite_GrantHitShortCircuits 验证 write / edit 命中 grants 直接放行
// 且不走 RequestSingle（用户已经说过"本次会话起永久放行此工具"）。
func TestPolicyHook_LocalWrite_GrantHitShortCircuits(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{} // 不应被调用
	check := &fakeCheckPerm{}
	grants := newFakeLocalGrants()
	grants.hits[grants.key("conv_5", "write")] = true

	hook := makePolicyHook(sc, gw, check.fn, grants)
	ctx := WithConvID(context.Background(), 5)
	out, err := hook(ctx, agent.HookInput{
		Stage:      agent.StagePreToolUse,
		ToolName:   "write",
		ToolInput:  json.RawMessage(`{"path":"/tmp/x","content":"y"}`),
		ToolCallID: "call_w1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gw.called {
		t.Fatal("grants hit 后不应再弹审批卡")
	}
	if out != nil && out.Decision == agent.DecisionDeny {
		t.Fatalf("grants hit 必须 Allow，got %+v", out)
	}
	r := sc.drain("call_w1")
	if r == nil || r.Decision != ai.Allow {
		t.Fatalf("sidecar should have Allow, got %+v", r)
	}
}

// TestPolicyHook_LocalWrite_AllowAllPersists 验证 write / edit 在 allowAll 时
// 把 (sessionID, toolName) 写进 grants。下次同会话再调 write 就直接放行。
func TestPolicyHook_LocalWrite_AllowAllPersists(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{resp: ai.ApprovalResponse{Decision: "allowAll"}}
	check := &fakeCheckPerm{}
	grants := newFakeLocalGrants()

	hook := makePolicyHook(sc, gw, check.fn, grants)
	ctx := WithConvID(context.Background(), 11)
	if _, err := hook(ctx, agent.HookInput{
		Stage:      agent.StagePreToolUse,
		ToolName:   "write",
		ToolInput:  json.RawMessage(`{"path":"/tmp/x","content":"y"}`),
		ToolCallID: "call_w2",
	}); err != nil {
		t.Fatal(err)
	}
	wantKey := grants.key("conv_11", "write")
	if !grants.saved[wantKey] {
		t.Fatalf("expected grants saved at %q, got %v", wantKey, grants.saved)
	}
}

// TestPolicyHook_LocalEdit_DenyKeepsGrantsClean 验证用户 deny 时不写 grants，
// 也不会把命令意外放行。
func TestPolicyHook_LocalEdit_DenyKeepsGrantsClean(t *testing.T) {
	sc := newSidecar()
	gw := &fakeApprovalRequester{resp: ai.ApprovalResponse{Decision: "deny"}}
	check := &fakeCheckPerm{}
	grants := newFakeLocalGrants()

	hook := makePolicyHook(sc, gw, check.fn, grants)
	ctx := WithConvID(context.Background(), 13)
	out, err := hook(ctx, agent.HookInput{
		Stage:      agent.StagePreToolUse,
		ToolName:   "edit",
		ToolInput:  json.RawMessage(`{"path":"/tmp/x","edits":[]}`),
		ToolCallID: "call_e1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.Decision != agent.DecisionDeny {
		t.Fatalf("expected Deny, got %+v", out)
	}
	if len(grants.saved) != 0 {
		t.Fatalf("deny 不能写 grants，got %v", grants.saved)
	}
}

// TestLocalKindToToolName 锁住 kind→toolName 映射表，避免新增 local_* kind 时漏改。
func TestLocalKindToToolName(t *testing.T) {
	cases := map[string]string{
		kindLocalBash:  "bash",
		kindLocalWrite: "write",
		kindLocalEdit:  "edit",
		"exec":         "", // 资产工具 kind → 空
	}
	for k, want := range cases {
		if got := localKindToToolName(k); got != want {
			t.Errorf("localKindToToolName(%q) = %q, want %q", k, got, want)
		}
	}
}
