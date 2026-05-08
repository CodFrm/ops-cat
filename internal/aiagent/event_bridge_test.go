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
	br := newBridge(rec, nil)
	br.translate(99, agent.Event{Kind: agent.EventTextDelta, Text: "hello"})
	if len(rec.events) != 1 || rec.events[0].Type != "content" || rec.events[0].Content != "hello" {
		t.Fatalf("got %+v", rec.events)
	}
}

func TestBridge_ThinkingThenTextSynthesizesThinkingDone(t *testing.T) {
	rec := &recordEmitter{}
	br := newBridge(rec, nil)
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
	br := newBridge(rec, nil)
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
	br := newBridge(rec, nil)
	br.translate(1, agent.Event{Kind: agent.EventPreToolUse, Tool: &agent.ToolEvent{ID: "abc", Name: "run_command", Input: []byte(`{"x":1}`)}})
	br.translate(1, agent.Event{Kind: agent.EventPostToolUse, Tool: &agent.ToolEvent{ID: "abc", Name: "run_command", Response: []byte(`"ok"`)}})
	if rec.events[0].ToolCallID != "abc" || rec.events[1].ToolCallID != "abc" {
		t.Fatalf("ToolCallID lost: %+v", rec.events)
	}
}

// 多条 follow-up 在 cago drainInjections 单 goroutine 同步逐条 emit，bridge 累积到
// pendingFollowUps，等下一个非 follow-up 事件（这里是 PreToolUse）到来时合并 emit
// 一次 queue_consumed_batch — 前端单次写入 N user + 1 asst placeholder，避免逐条
// queue_consumed 留下空 asst 气泡。
func TestBridge_FollowUpsMergedIntoBatchOnNextEvent(t *testing.T) {
	rec := &recordEmitter{}
	displays := []string{"u1", "u2", "u3"}
	idx := 0
	br := newBridge(rec, func() string {
		if idx >= len(displays) {
			return ""
		}
		out := displays[idx]
		idx++
		return out
	})

	// 三条 follow-up：bridge 不应立刻 emit 任何事件
	for range displays {
		br.translate(7, agent.Event{
			Kind:    agent.EventUserPromptSubmit,
			Message: &agent.Message{Kind: agent.MessageKindFollowUp, Text: "irrelevant"},
		})
	}
	if len(rec.events) != 0 {
		t.Fatalf("follow-ups should be buffered, not emitted: %+v", rec.events)
	}

	// 下一个非 follow-up 事件（ToolUse）应触发 flush
	br.translate(7, agent.Event{
		Kind: agent.EventPreToolUse,
		Tool: &agent.ToolEvent{ID: "t1", Name: "run_command", Input: []byte("{}")},
	})

	if len(rec.events) != 2 {
		t.Fatalf("expected [queue_consumed_batch, tool_start], got %d: %+v", len(rec.events), rec.events)
	}
	batch := rec.events[0]
	if batch.Type != "queue_consumed_batch" {
		t.Fatalf("first event should be queue_consumed_batch, got %s", batch.Type)
	}
	if got, want := batch.QueueContents, displays; len(got) != len(want) {
		t.Fatalf("QueueContents len mismatch: got %v want %v", got, want)
	}
	for i := range displays {
		if batch.QueueContents[i] != displays[i] {
			t.Fatalf("QueueContents[%d] = %q, want %q", i, batch.QueueContents[i], displays[i])
		}
	}
	if rec.events[1].Type != "tool_start" {
		t.Fatalf("second event should be tool_start, got %s", rec.events[1].Type)
	}
}

// 非 follow-up 类型的 EventUserPromptSubmit（如初始 prompt 的 system-style submit）
// 不应进入累积窗口；应该直接放行（bridge 当前没有翻译规则 → silently ignored，但
// 关键是不会被错放进 pendingFollowUps）。
func TestBridge_NonFollowUpUserPromptSubmitNotBuffered(t *testing.T) {
	rec := &recordEmitter{}
	popCount := 0
	br := newBridge(rec, func() string { popCount++; return "should-not-be-popped" })
	br.translate(1, agent.Event{
		Kind:    agent.EventUserPromptSubmit,
		Message: &agent.Message{Kind: agent.MessageKindText, Text: "initial"},
	})
	if popCount != 0 {
		t.Fatalf("popDisplay called %d times for non-follow-up; want 0", popCount)
	}
	// 后续 TextDelta 时不应 flush 出 batch（pendingFollowUps 应为空）
	br.translate(1, agent.Event{Kind: agent.EventTextDelta, Text: "hi"})
	for _, ev := range rec.events {
		if ev.Type == "queue_consumed_batch" {
			t.Fatalf("unexpected queue_consumed_batch emitted: %+v", ev)
		}
	}
}
