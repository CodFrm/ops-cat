package aiagent

import (
	"sync"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
	"github.com/stretchr/testify/assert"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

type fakeUsageStasher struct {
	saved map[string]*conversation_entity.TokenUsage
}

func (f *fakeUsageStasher) stashPendingUsage(id string, u *conversation_entity.TokenUsage) {
	if f.saved == nil {
		f.saved = map[string]*conversation_entity.TokenUsage{}
	}
	f.saved[id] = u
}

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
	br := newBridge(rec, nil, nil)
	br.translate(99, agent.Event{Kind: agent.EventTextDelta, Text: "hello"})
	if len(rec.events) != 1 || rec.events[0].Type != "content" || rec.events[0].Content != "hello" {
		t.Fatalf("got %+v", rec.events)
	}
}

func TestBridge_ThinkingThenTextSynthesizesThinkingDone(t *testing.T) {
	rec := &recordEmitter{}
	br := newBridge(rec, nil, nil)
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
	br := newBridge(rec, nil, nil)
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
	br := newBridge(rec, nil, nil)
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
	}, nil)

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

// 回归：cago runloop 在每次 Session.Stream 启动时会遍历 req.History（来自 store
// 的持久化历史），对其中所有 Kind=MessageKindFollowUp 的消息重新 emit
// EventUserPromptSubmit（cago/agents/agent/builtin_runloop.go:69-118 — 给 hook /
// observer 一次"看到所有历史 follow-up"的机会）。
// bridge 的 displayContent FIFO 是 per-System 内存结构，对历史回放本来就没有对应
// 推送 → popDisplay() 返回 ""。如果 bridge 把这些空串累积进 pendingFollowUps 并
// emit queue_consumed_batch，前端会按 FIFO 写出 N 条空 user 气泡（用户实际看到的
// 截图症状）。
//
// 期望：popDisplay 返回 "" 时跳过该事件，不累积也不 emit。
func TestBridge_HistoricalFollowUpReplayDoesNotEmitGhostBatch(t *testing.T) {
	rec := &recordEmitter{}
	popCount := 0
	br := newBridge(rec, func() string {
		popCount++
		return "" // 模拟历史回放：FIFO 没有对应推送
	}, nil)

	// 3 条历史 follow-up 重放
	for range 3 {
		br.translate(1, agent.Event{
			Kind:    agent.EventUserPromptSubmit,
			Message: &agent.Message{Kind: agent.MessageKindFollowUp, Text: "history"},
		})
	}
	// 紧跟一个非 follow-up 事件（模拟新 prompt 的 EventUserPromptSubmit 之后会
	// 进入正常翻译路径；这里用 TextDelta 触发潜在的 flush 时机）。
	br.translate(1, agent.Event{Kind: agent.EventTextDelta, Text: "hi"})

	// 不应该出现任何 queue_consumed_batch
	for _, ev := range rec.events {
		if ev.Type == "queue_consumed_batch" {
			t.Fatalf("historical replay leaked a ghost queue_consumed_batch: %+v", ev)
		}
	}
}

// 回归：历史回放（empty pop）和真实 Steer 注入（non-empty pop）混合时，bridge
// 仅累积真实注入。前端 queue_consumed_batch 应只包含 Steer 推送的 displayContent。
func TestBridge_MixedHistoricalAndLiveFollowUpsOnlyEmitLive(t *testing.T) {
	rec := &recordEmitter{}
	// FIFO 模拟：head=空(历史) → "live1" → 空(历史) → "live2"
	queue := []string{"", "live1", "", "live2"}
	br := newBridge(rec, func() string {
		if len(queue) == 0 {
			return ""
		}
		v := queue[0]
		queue = queue[1:]
		return v
	}, nil)

	for range 4 {
		br.translate(1, agent.Event{
			Kind:    agent.EventUserPromptSubmit,
			Message: &agent.Message{Kind: agent.MessageKindFollowUp, Text: "x"},
		})
	}
	br.translate(1, agent.Event{Kind: agent.EventTextDelta, Text: "ok"})

	var batch *ai.StreamEvent
	for i := range rec.events {
		if rec.events[i].Type == "queue_consumed_batch" {
			batch = &rec.events[i]
			break
		}
	}
	if batch == nil {
		t.Fatalf("expected queue_consumed_batch for live drains, got events: %+v", rec.events)
	}
	if got, want := batch.QueueContents, []string{"live1", "live2"}; len(got) != len(want) {
		t.Fatalf("QueueContents = %v, want %v", got, want)
	}
	for i, want := range []string{"live1", "live2"} {
		if batch.QueueContents[i] != want {
			t.Fatalf("QueueContents[%d]=%q, want %q", i, batch.QueueContents[i], want)
		}
	}
}

// 非 follow-up 类型的 EventUserPromptSubmit（如初始 prompt 的 system-style submit）
// 不应进入累积窗口；应该直接放行（bridge 当前没有翻译规则 → silently ignored，但
// 关键是不会被错放进 pendingFollowUps）。
func TestBridge_NonFollowUpUserPromptSubmitNotBuffered(t *testing.T) {
	rec := &recordEmitter{}
	popCount := 0
	br := newBridge(rec, func() string { popCount++; return "should-not-be-popped" }, nil)
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

// EventUsage 在 cago runloop 一轮中紧跟 EventMessageEnd 之后到达；bridge 用最近
// 一条 assistant model MessageEnd 的 ID 作为 usage 缓存键。stash 进 *System 的
// pendingUsage 后由 gormStore.Save drain 出来写到 conversation_messages.token_usage
// 列。bridge 仍照常 emit 给前端的 "usage" 流事件——前端实时显示开销不依赖持久化。
func TestBridge_StashesUsageKeyedByLastAssistantMessageEnd(t *testing.T) {
	rec := &recordEmitter{}
	stash := &fakeUsageStasher{}
	br := newBridge(rec, nil, stash)
	br.translate(7, agent.Event{Kind: agent.EventMessageEnd, Message: &agent.Message{
		ID: "asst-1", Role: agent.RoleAssistant, Origin: agent.MessageOriginModel,
	}})
	br.translate(7, agent.Event{Kind: agent.EventUsage, Usage: provider.Usage{
		PromptTokens: 100, CompletionTokens: 20, CachedTokens: 5, CacheCreationTokens: 3,
	}})
	got, ok := stash.saved["asst-1"]
	if !ok {
		t.Fatalf("usage not stashed under asst-1: %+v", stash.saved)
	}
	assert.Equal(t, 100, got.InputTokens)
	assert.Equal(t, 20, got.OutputTokens)
	assert.Equal(t, 5, got.CacheReadTokens)
	assert.Equal(t, 3, got.CacheCreationTokens)
	// EventUsage still emits "usage" stream event for the frontend
	found := false
	for _, ev := range rec.events {
		if ev.Type == "usage" {
			found = true
			break
		}
	}
	assert.True(t, found, "usage stream event must still emit for frontend")
}

// EventMessageEnd 自身不应该 emit 任何前端事件（cago internalObserver 已经把
// assistant 文本/工具调用通过 TextDelta/PreToolUse 实时流过去了；MessageEnd 只
// 是给 bridge 提供 lastAssistantMsgID 锚点）。
func TestBridge_MessageEndDoesNotEmit(t *testing.T) {
	rec := &recordEmitter{}
	br := newBridge(rec, nil, nil)
	br.translate(1, agent.Event{Kind: agent.EventMessageEnd, Message: &agent.Message{
		ID: "asst-1", Role: agent.RoleAssistant, Origin: agent.MessageOriginModel, Text: "hi",
	}})
	if len(rec.events) != 0 {
		t.Fatalf("EventMessageEnd should not emit anything; got %+v", rec.events)
	}
}

// 非 assistant model 的 MessageEnd（user / tool 角色）不应被记入
// lastAssistantMsgID。否则 EventUsage 可能错误地把 usage 绑到 user/tool 行。
func TestBridge_MessageEndIgnoresNonAssistantModel(t *testing.T) {
	stash := &fakeUsageStasher{}
	br := newBridge(&recordEmitter{}, nil, stash)
	// user origin → 不更新 lastAssistantMsgID
	br.translate(1, agent.Event{Kind: agent.EventMessageEnd, Message: &agent.Message{
		ID: "user-1", Role: agent.RoleUser, Origin: agent.MessageOriginUser,
	}})
	br.translate(1, agent.Event{Kind: agent.EventUsage, Usage: provider.Usage{PromptTokens: 1}})
	if _, ok := stash.saved["user-1"]; ok {
		t.Fatalf("usage stashed under user-1; want skipped because no prior assistant model MessageEnd")
	}
	if len(stash.saved) != 0 {
		t.Fatalf("stash should be empty: %+v", stash.saved)
	}
}
