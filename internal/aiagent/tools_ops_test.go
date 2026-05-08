package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/opskat/opskat/internal/ai"
)

func TestWrapToolDef_PassesArgsAndReturnsString(t *testing.T) {
	def := ai.ToolDef{
		Name:        "echo_test",
		Description: "echo for tests",
		Params: []ai.ParamDef{
			{Name: "msg", Type: ai.ParamString, Description: "msg", Required: true},
		},
		Handler: func(_ context.Context, args map[string]any) (string, error) {
			return "got:" + args["msg"].(string), nil
		},
	}
	tool := wrapToolDef(def, &Deps{})
	if tool.Name() != "echo_test" {
		t.Fatalf("Name=%q", tool.Name())
	}
	got, err := tool.Call(context.Background(), json.RawMessage(`{"msg":"hi"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if s, ok := got.(string); !ok || s != "got:hi" {
		t.Fatalf("Call returned %v (%T)", got, got)
	}
}

func TestWrapToolDef_TruncatesLongResult(t *testing.T) {
	big := strings.Repeat("X", 64*1024) // 64KB
	def := ai.ToolDef{
		Name: "big_test", Description: "big",
		Handler: func(_ context.Context, _ map[string]any) (string, error) { return big, nil },
	}
	tool := wrapToolDef(def, &Deps{})
	got, _ := tool.Call(context.Background(), json.RawMessage(`{}`))
	s := got.(string)
	if !strings.Contains(s, "Output truncated") {
		t.Fatal("missing truncation marker")
	}
	if !strings.Contains(s, "exceeds 32768 byte limit") {
		t.Fatalf("missing limit hint, got tail %q", s[len(s)-200:])
	}
}

func TestWrapToolDef_BuildsJSONSchemaFromParamDefs(t *testing.T) {
	def := ai.ToolDef{
		Name: "x", Description: "x",
		Params: []ai.ParamDef{
			{Name: "a", Type: ai.ParamString, Description: "a", Required: true},
			{Name: "b", Type: ai.ParamNumber, Description: "b"},
		},
		Handler: func(context.Context, map[string]any) (string, error) { return "", nil },
	}
	tool := wrapToolDef(def, &Deps{})
	var schema struct {
		Type       string `json:"type"`
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Type != "object" {
		t.Fatalf("type=%s", schema.Type)
	}
	if schema.Properties["a"].Type != "string" {
		t.Fatalf("a.type=%s", schema.Properties["a"].Type)
	}
	if schema.Properties["b"].Type != "number" {
		t.Fatalf("b.type=%s", schema.Properties["b"].Type)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "a" {
		t.Fatalf("required=%v", schema.Required)
	}
}

// TestWrapToolDef_NilArgsCallsHandlerWithEmptyMap 守住 raw == "" 时给 handler 喂
// 空 map（而不是 nil map），避免 handler 里 `args["x"]` panic on nil map dereference。
func TestWrapToolDef_NilArgsCallsHandlerWithEmptyMap(t *testing.T) {
	var got map[string]any
	def := ai.ToolDef{
		Name: "noargs", Description: "x",
		Handler: func(_ context.Context, args map[string]any) (string, error) {
			got = args
			return "ok", nil
		},
	}
	tool := wrapToolDef(def, &Deps{})
	if _, err := tool.Call(context.Background(), nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got == nil {
		t.Error("handler received nil args; want empty map")
	}
}

// TestWrapToolDef_InvalidJSONReturnsError 守住 raw 解析失败的分支：把错误以
// `nil, error` 返回，让 cago tool runtime 把"参数错误"作为常规失败信号回模型，
// 而不是 panic 整个 stream。
func TestWrapToolDef_InvalidJSONReturnsError(t *testing.T) {
	def := ai.ToolDef{
		Name: "x", Description: "x",
		Handler: func(context.Context, map[string]any) (string, error) { return "", nil },
	}
	tool := wrapToolDef(def, &Deps{})
	_, err := tool.Call(context.Background(), json.RawMessage(`{not-json`))
	if err == nil {
		t.Fatal("invalid args: want error, got nil")
	}
}

// TestWrapToolDef_HandlerErrorBecomesContentNotErr 验证 handler 返错时的关键转换：
// wrap 返回 `(string, nil)` 而不是 `(_, error)`——这样 cago 会把错误文本当成 tool
// result 推回 LLM，让模型自己看到错误并重试 / 调整参数。如果改成抛 error，
// cago 会把整轮 abort，模型再也看不到失败原因。这是和老 ai.AuditingExecutor
// 行为一致的关键约定。
func TestWrapToolDef_HandlerErrorBecomesContentNotErr(t *testing.T) {
	def := ai.ToolDef{
		Name: "x", Description: "x",
		Handler: func(context.Context, map[string]any) (string, error) {
			return "", errors.New("backend down")
		},
	}
	tool := wrapToolDef(def, &Deps{})
	got, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("handler error must be folded into result, got err=%v", err)
	}
	s, _ := got.(string)
	if !strings.Contains(s, "Tool execution error") || !strings.Contains(s, "backend down") {
		t.Errorf("expected error-as-content, got %q", s)
	}
}

// TestInjectDeps_NilDepsPassThrough 守住父 ctx 不被替换：injectDeps(ctx, nil)
// 必须返回原 ctx（不创建空 wrapper），保证 ctx.Done / Value chain 不被破坏。
func TestInjectDeps_NilDepsPassThrough(t *testing.T) {
	type k struct{}
	parent := context.WithValue(context.Background(), k{}, "v")
	got := injectDeps(parent, nil)
	if got.Value(k{}) != "v" {
		t.Error("nil deps must pass parent ctx through unchanged")
	}
}

// TestInjectDeps_OnlyPolicyCheckerInjected 是唯一能从外部 exported getter
// （ai.GetPolicyChecker）反查 ctx 的字段；其余 cache key 都是 unexported。
// 但只要这条通过，加上"全填充时不 panic + 走过分支"的覆盖率提升，就能锁住
// "逐字段 if 注入"的语义不被改成"全有或全无"。
func TestInjectDeps_OnlyPolicyCheckerInjected(t *testing.T) {
	checker := &ai.CommandPolicyChecker{}
	deps := &Deps{PolicyChecker: checker}
	ctx := injectDeps(context.Background(), deps)
	if got := ai.GetPolicyChecker(ctx); got != checker {
		t.Errorf("PolicyChecker not injected: got %p, want %p", got, checker)
	}
}

// TestInjectDeps_FullDepsExercisesAllBranches 让 injectDeps 走过每一条 if 分支
// （之前只有 &Deps{} 全空被覆盖，53.8%）。不能从 OpsKat 包外验证 unexported
// SSH/Mongo/Kafka key 是否被设；这条测试的价值是覆盖到分支 + 守住"全字段不
// panic"的契约。
func TestInjectDeps_FullDepsExercisesAllBranches(t *testing.T) {
	deps := &Deps{
		SSHCache:      ai.NewSSHClientCache(),
		MongoCache:    ai.NewMongoDBClientCache(),
		KafkaService:  nil, // kafka_svc.New 需要真 pool — 留 nil，分支由 PolicyChecker 等带过
		PolicyChecker: &ai.CommandPolicyChecker{},
	}
	defer func() { _ = deps.Close() }()
	ctx := injectDeps(context.Background(), deps)
	if ctx == nil {
		t.Fatal("injectDeps returned nil ctx")
	}
	// PolicyChecker 是 exported getter 唯一能直接验的字段。
	if ai.GetPolicyChecker(ctx) == nil {
		t.Error("PolicyChecker missing after injectDeps")
	}
}

func TestOpsTools_HasAllExpectedNames(t *testing.T) {
	tools := OpsTools(&Deps{})
	want := map[string]bool{
		"list_assets": false, "get_asset": false, "run_command": false,
		"exec_k8s": false, "add_asset": false, "update_asset": false,
		"list_groups": false, "get_group": false, "add_group": false, "update_group": false,
		"upload_file": false, "download_file": false,
		"exec_sql": false, "exec_redis": false, "exec_mongo": false,
		"kafka_cluster": false, "kafka_topic": false, "kafka_consumer_group": false,
		"kafka_acl": false, "kafka_schema": false, "kafka_connect": false, "kafka_message": false,
		"request_permission": false, "batch_command": false, "exec_tool": false,
	}
	for _, tt := range tools {
		if _, ok := want[tt.Name()]; ok {
			want[tt.Name()] = true
		}
	}
	for n, seen := range want {
		if !seen {
			t.Errorf("missing tool: %s", n)
		}
	}
}
