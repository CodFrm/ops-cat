package aiagent

import (
	"context"
	"encoding/json"
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
