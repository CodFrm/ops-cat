package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/tool"
)

func TestWrapLocalTool_RenamesBashWriteEdit(t *testing.T) {
	cases := []struct {
		orig    string
		want    string
		descIn  string
		descHas string
	}{
		{"bash", "local_bash", "Execute a bash command", "LOCAL MACHINE ONLY"},
		{"write", "local_write", "Write to a file", "LOCAL MACHINE ONLY"},
		{"edit", "local_edit", "Edit a file", "LOCAL MACHINE ONLY"},
	}
	for _, tc := range cases {
		t.Run(tc.orig, func(t *testing.T) {
			in := &tool.RawTool{NameStr: tc.orig, DescStr: tc.descIn}
			out := WrapLocalTool(in)
			if out.Name() != tc.want {
				t.Errorf("Name(): got %q want %q", out.Name(), tc.want)
			}
			if !strings.Contains(out.Description(), tc.descHas) {
				t.Errorf("Description() missing warning %q; got %q", tc.descHas, out.Description())
			}
			if !strings.Contains(out.Description(), tc.descIn) {
				t.Errorf("Description() should preserve original; got %q", out.Description())
			}
		})
	}
}

func TestWrapLocalTool_LeavesOtherToolsAlone(t *testing.T) {
	for _, name := range []string{"read", "grep", "find", "ls", "bash_output", "kill_shell", "task_create", "run_command"} {
		t.Run(name, func(t *testing.T) {
			in := &tool.RawTool{NameStr: name, DescStr: "orig"}
			out := WrapLocalTool(in)
			if out.Name() != name {
				t.Errorf("expected %q untouched, got %q", name, out.Name())
			}
			if out.Description() != "orig" {
				t.Errorf("expected description untouched, got %q", out.Description())
			}
			if out != tool.Tool(in) {
				t.Errorf("expected same pointer for non-renamed tool to avoid useless clone")
			}
		})
	}
}

func TestWrapLocalTool_NonRawToolPassThrough(t *testing.T) {
	custom := stubTool{name: "bash"}
	out := WrapLocalTool(custom)
	if out.Name() != "bash" {
		t.Errorf("non-RawTool should pass through unchanged; got name=%q", out.Name())
	}
}

func TestWrapLocalTool_DoesNotMutateOriginal(t *testing.T) {
	in := &tool.RawTool{NameStr: "bash", DescStr: "orig desc"}
	WrapLocalTool(in)
	if in.NameStr != "bash" {
		t.Errorf("original RawTool.NameStr mutated to %q", in.NameStr)
	}
	if in.DescStr != "orig desc" {
		t.Errorf("original RawTool.DescStr mutated to %q", in.DescStr)
	}
}

type stubTool struct {
	name string
}

func (s stubTool) Name() string        { return s.name }
func (s stubTool) Description() string { return "" }
func (s stubTool) Schema() agent.Schema {
	return agent.Schema{Type: "object"}
}
func (s stubTool) Call(ctx context.Context, in map[string]any) (*agent.ToolResultBlock, error) {
	return nil, nil
}
