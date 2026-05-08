package aiagent

import (
	"context"
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
