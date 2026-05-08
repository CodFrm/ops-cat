package aiagent

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

func TestSubagentObserver_TagsToolStartWithAgentRole(t *testing.T) {
	rec := &recordEmitter{}
	obs := makeSubagentObserver(rec, 11, "ops-explorer", "explore /etc")

	obs(context.Background(), agent.Event{
		Kind: agent.EventPreToolUse,
		Tool: &agent.ToolEvent{ID: "x1", Name: "list_assets"},
	})

	var ts ai.StreamEvent
	for _, e := range rec.events {
		if e.Type == "tool_start" {
			ts = e
		}
	}
	if ts.AgentRole != "ops-explorer" {
		t.Fatalf("AgentRole = %q", ts.AgentRole)
	}
}

func TestSubagentObserver_EmitsAgentStart(t *testing.T) {
	rec := &recordEmitter{}
	_ = makeSubagentObserverEmitStart(rec, 1, "ops-explorer", "task X")
	if len(rec.events) != 1 || rec.events[0].Type != "agent_start" {
		t.Fatalf("expected one agent_start, got %+v", rec.events)
	}
	if rec.events[0].AgentRole != "ops-explorer" || rec.events[0].AgentTask != "task X" {
		t.Fatalf("fields lost: %+v", rec.events[0])
	}
}

func TestMakeAgentEndHook_EmitsAgentEndOnDispatchPost(t *testing.T) {
	rec := &recordEmitter{}
	hook := MakeAgentEndHook(rec)
	long := strings.Repeat("Y", 4096)
	_, _ = hook(context.Background(), agent.HookInput{
		Stage:        agent.StagePostToolUse,
		ToolName:     "dispatch_subagent",
		ToolResponse: []byte(strconv.Quote(long)),
	})
	if len(rec.events) != 1 || rec.events[0].Type != "agent_end" {
		t.Fatalf("expected agent_end, got %+v", rec.events)
	}
	if len(rec.events[0].Content) > 2048+3 {
		t.Fatalf("summary not truncated to 2048 (+ellipsis): %d", len(rec.events[0].Content))
	}
}

func TestMakeAgentEndHook_IgnoresOtherTools(t *testing.T) {
	rec := &recordEmitter{}
	hook := MakeAgentEndHook(rec)
	_, _ = hook(context.Background(), agent.HookInput{
		Stage:        agent.StagePostToolUse,
		ToolName:     "run_command",
		ToolResponse: []byte(`"ok"`),
	})
	if len(rec.events) != 0 {
		t.Fatalf("expected no events for non-dispatch tool, got %+v", rec.events)
	}
}
