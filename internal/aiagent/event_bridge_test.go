package aiagent

import (
	"sync"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"

	"github.com/opskat/opskat/internal/ai"
)

type recordEmitter struct {
	mu     sync.Mutex
	events []ai.StreamEvent
}

func (r *recordEmitter) Emit(_ int64, ev ai.StreamEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func TestBridge_TextDeltaToContent(t *testing.T) {
	rec := &recordEmitter{}
	br := newBridge(rec)
	br.translate(99, agent.Event{Kind: agent.EventTextDelta, Text: "hello"})
	if len(rec.events) != 1 || rec.events[0].Type != "content" || rec.events[0].Content != "hello" {
		t.Fatalf("got %+v", rec.events)
	}
}

func TestBridge_ThinkingThenTextSynthesizesThinkingDone(t *testing.T) {
	rec := &recordEmitter{}
	br := newBridge(rec)
	br.translate(1, agent.Event{Kind: agent.EventThinkingDelta, Text: "reflecting"})
	br.translate(1, agent.Event{Kind: agent.EventTextDelta, Text: "answer"})

	if len(rec.events) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(rec.events), rec.events)
	}
	if rec.events[1].Type != "thinking_done" {
		t.Fatalf("expected synthesized thinking_done as second event, got %s", rec.events[1].Type)
	}
}

func TestBridge_UsageMappingExposesCacheCreation(t *testing.T) {
	rec := &recordEmitter{}
	br := newBridge(rec)
	br.translate(1, agent.Event{Kind: agent.EventUsage, Usage: provider.Usage{
		PromptTokens: 100, CompletionTokens: 20, CachedTokens: 50, CacheCreationTokens: 30,
	}})
	if rec.events[0].Usage == nil {
		t.Fatal("missing usage")
	}
	u := rec.events[0].Usage
	if u.InputTokens != 100 || u.OutputTokens != 20 || u.CacheReadTokens != 50 || u.CacheCreationTokens != 30 {
		t.Fatalf("usage mapping wrong: %+v", u)
	}
}

func TestBridge_ToolEventsCarryToolCallID(t *testing.T) {
	rec := &recordEmitter{}
	br := newBridge(rec)
	br.translate(1, agent.Event{Kind: agent.EventPreToolUse, Tool: &agent.ToolEvent{ID: "abc", Name: "run_command", Input: []byte(`{"x":1}`)}})
	br.translate(1, agent.Event{Kind: agent.EventPostToolUse, Tool: &agent.ToolEvent{ID: "abc", Name: "run_command", Response: []byte(`"ok"`)}})
	if rec.events[0].ToolCallID != "abc" || rec.events[1].ToolCallID != "abc" {
		t.Fatalf("ToolCallID lost: %+v", rec.events)
	}
}
