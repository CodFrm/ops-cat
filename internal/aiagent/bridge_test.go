package aiagent

import (
	"context"
	"errors"
	"testing"

	"github.com/cago-frame/agents/agent"
	"github.com/stretchr/testify/assert"

	"github.com/opskat/opskat/internal/ai"
)

type captureEmitter struct {
	convID int64
	events []ai.StreamEvent
}

func (c *captureEmitter) Emit(convID int64, ev ai.StreamEvent) {
	c.convID = convID
	c.events = append(c.events, ev)
}

func TestBridge_TextDelta(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "hi"})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "content", em.events[0].Type)
	assert.Equal(t, "hi", em.events[0].Content)
	assert.Equal(t, int64(42), em.convID)
}

func TestBridge_ThinkingDelta(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventThinkingDelta, Delta: "let me think"})
	assert.Equal(t, "thinking", em.events[0].Type)
	assert.Equal(t, "let me think", em.events[0].Content)
}

func TestBridge_ErrorEmitsErrorOnly(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventError, Error: errors.New("boom")})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "error", em.events[0].Type)
	assert.Contains(t, em.events[0].Error, "boom")
}

func TestBridge_DoneEmitted(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventDone})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "done", em.events[0].Type)
}

func TestBridge_RetryEvent(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{
		Kind: agent.EventRetry,
		Retry: &agent.RetryEvent{
			Attempt: 2,
			Delay:   0,
			Cause:   errors.New("503"),
		},
	})
	assert.Equal(t, "retry", em.events[0].Type)
	assert.Contains(t, em.events[0].Content, "2/")
	assert.Contains(t, em.events[0].Error, "503")
}

func TestBridge_QueueConsumedBatch_AggregatesUserAppends(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)

	// Two user-message appends in a row (e.g., from Steer'd queue draining)
	userMsgA := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "expanded A"},
		agent.MetadataBlock{Key: "display", Value: "raw A"},
	}}
	userMsgB := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "expanded B"},
		agent.MetadataBlock{Key: "display", Value: "raw B"},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Index: 0, Message: &userMsgA})
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Index: 1, Message: &userMsgB})

	// At this point nothing should have been emitted (still pending)
	assert.Len(t, em.events, 0, "user appends are buffered until a runner event arrives")

	// Next runner event triggers the flush BEFORE the event itself
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "ack"})

	// Expected: queue_consumed_batch with both raw displays, then content
	assert.Len(t, em.events, 2)
	assert.Equal(t, "queue_consumed_batch", em.events[0].Type)
	assert.Equal(t, []string{"raw A", "raw B"}, em.events[0].QueueContents)
	assert.Equal(t, "content", em.events[1].Type)
	assert.Equal(t, "ack", em.events[1].Content)
}

func TestBridge_QueueConsumedBatch_NonUserAppendIgnored(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)

	// Assistant message append shouldn't queue anything
	asstMsg := agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{agent.TextBlock{Text: "hi"}}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Index: 0, Message: &asstMsg})

	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "hello"})

	// Expected: only "content", no batch flush
	assert.Len(t, em.events, 1)
	assert.Equal(t, "content", em.events[0].Type)
}

func TestBridge_PreToolUse_EmitsToolStart(t *testing.T) {
	// 前端 aiStore 的 streamEvent reducer 只识别 "tool_start"，
	// 收到 "tool_call" 会被静默丢弃 —— 工具气泡完全不渲染。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{
		Kind: agent.EventPreToolUse,
		Tool: &agent.ToolEvent{
			Name:      "ssh_exec",
			Input:     map[string]any{"cmd": "ls"},
			ToolUseID: "call_1",
		},
	})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "tool_start", em.events[0].Type)
	assert.Equal(t, "ssh_exec", em.events[0].ToolName)
	assert.Equal(t, "call_1", em.events[0].ToolCallID)
	assert.Contains(t, em.events[0].ToolInput, "ls")
}

func TestBridge_QueueConsumedBatch_UserWithoutDisplayUsesEmptyString(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)

	userMsg := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "no display block here"},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Index: 0, Message: &userMsg})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "x"})

	// Expected: batch with one empty string
	assert.Equal(t, "queue_consumed_batch", em.events[0].Type)
	assert.Equal(t, []string{""}, em.events[0].QueueContents)
}

// types returns the ordered list of event types emitted so far.
func eventTypes(events []ai.StreamEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}

func TestBridge_PostToolUse_Success(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{
		Kind: agent.EventPostToolUse,
		Tool: &agent.ToolEvent{
			Name:      "ssh_exec",
			ToolUseID: "call_1",
			Output: &agent.ToolResultBlock{
				ToolUseID: "call_1",
				Content:   []agent.ContentBlock{agent.TextBlock{Text: "ok"}},
			},
		},
	})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "tool_result", em.events[0].Type)
	assert.Equal(t, "ssh_exec", em.events[0].ToolName)
	assert.Equal(t, "call_1", em.events[0].ToolCallID)
	assert.Empty(t, em.events[0].Error, "success path must not set Error")
	assert.Contains(t, em.events[0].Content, "ok")
}

func TestBridge_PostToolUse_ErrorCarriesErrorField(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{
		Kind: agent.EventPostToolUse,
		Tool: &agent.ToolEvent{
			Name:      "ssh_exec",
			ToolUseID: "call_1",
			Output: &agent.ToolResultBlock{
				ToolUseID: "call_1",
				IsError:   true,
				Content:   []agent.ContentBlock{agent.TextBlock{Text: "permission denied"}},
			},
		},
	})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "tool_result", em.events[0].Type)
	assert.Equal(t, "ssh_exec", em.events[0].ToolName)
	assert.Equal(t, "call_1", em.events[0].ToolCallID)
	assert.Contains(t, em.events[0].Error, "permission denied", "error path must populate Error field")
	assert.Contains(t, em.events[0].Content, "permission denied")
}

func TestBridge_PreToolUse_NilToolDoesNothing(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventPreToolUse, Tool: nil})
	assert.Len(t, em.events, 0)
}

func TestBridge_PostToolUse_NilToolOrOutputDoesNothing(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	// Nil Tool
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventPostToolUse, Tool: nil})
	// Tool with nil Output
	b.OnRunnerEvent(context.Background(), agent.Event{
		Kind: agent.EventPostToolUse,
		Tool: &agent.ToolEvent{Name: "x", ToolUseID: "y", Output: nil},
	})
	assert.Len(t, em.events, 0)
}

func TestBridge_ThinkingDone_EmittedBeforeNextNonThinkingEvent(t *testing.T) {
	// cago 没有显式 thinking-end 事件，bridge 通过 "下一个非 thinking 事件" 推断边界。
	// 这条契约保护：思考→正文 的过渡，前端依赖 thinking_done 把 thinking 块从 running 切到 completed。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventThinkingDelta, Delta: "..."})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventThinkingDelta, Delta: "more"})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "answer"})

	assert.Equal(t, []string{"thinking", "thinking", "thinking_done", "content"}, eventTypes(em.events))
}

func TestBridge_ThinkingDone_NotEmittedBetweenThinkingDeltas(t *testing.T) {
	// 连续 thinking_delta 之间不应该插 thinking_done，否则前端会反复重建思考块。
	em := &captureEmitter{}
	b := newBridge(42, em)
	for i := 0; i < 3; i++ {
		b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventThinkingDelta, Delta: "x"})
	}
	assert.Equal(t, []string{"thinking", "thinking", "thinking"}, eventTypes(em.events))
}

func TestBridge_ThinkingDone_BeforeToolStart(t *testing.T) {
	// 思考完直接调工具：thinking → tool_start 之间也要有 thinking_done。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventThinkingDelta, Delta: "plan"})
	b.OnRunnerEvent(context.Background(), agent.Event{
		Kind: agent.EventPreToolUse,
		Tool: &agent.ToolEvent{Name: "x", ToolUseID: "1", Input: map[string]any{}},
	})
	assert.Equal(t, []string{"thinking", "thinking_done", "tool_start"}, eventTypes(em.events))
}

func TestBridge_ThinkingDone_BeforeDoneAndError(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventThinkingDelta, Delta: "x"})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventDone})
	assert.Equal(t, []string{"thinking", "thinking_done", "done"}, eventTypes(em.events))

	em2 := &captureEmitter{}
	b2 := newBridge(42, em2)
	b2.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventThinkingDelta, Delta: "x"})
	b2.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventError, Error: errors.New("oops")})
	assert.Equal(t, []string{"thinking", "thinking_done", "error"}, eventTypes(em2.events))
}

func TestBridge_ThinkingDone_Idempotent(t *testing.T) {
	// 进入正文后再来一次正文，不应该重复发 thinking_done。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventThinkingDelta, Delta: "x"})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "y"})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "z"})

	assert.Equal(t, []string{"thinking", "thinking_done", "content", "content"}, eventTypes(em.events))
}

func TestBridge_MessageEnd_EmitsUsage(t *testing.T) {
	// cago 把 token 用量挂在 EventMessageEnd / EventTurnEnd 的 Event.Usage 上。
	// bridge 之前把这俩都标 observational 直接吞掉 —— 前端 TokenUsageBadge 永远拿不到数据。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{
		Kind: agent.EventMessageEnd,
		Usage: &agent.Usage{
			PromptTokens:        100,
			CompletionTokens:    50,
			CachedTokens:        30,
			CacheCreationTokens: 20,
		},
	})
	assert.Len(t, em.events, 1)
	ev := em.events[0]
	assert.Equal(t, "usage", ev.Type)
	if assert.NotNil(t, ev.Usage) {
		assert.Equal(t, 100, ev.Usage.InputTokens, "PromptTokens → InputTokens")
		assert.Equal(t, 50, ev.Usage.OutputTokens, "CompletionTokens → OutputTokens")
		assert.Equal(t, 30, ev.Usage.CacheReadTokens, "CachedTokens → CacheReadTokens")
		assert.Equal(t, 20, ev.Usage.CacheCreationTokens, "CacheCreationTokens → CacheCreationTokens")
	}
}

func TestBridge_MessageEnd_NilUsageDoesNotEmit(t *testing.T) {
	// 没有 usage 数据时不要发空 usage 事件，否则前端会把空对象 merge 进 tokenUsage。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventMessageEnd, Usage: nil})
	assert.Len(t, em.events, 0)
}

func TestBridge_TurnEnd_AndCancelledAreObservational(t *testing.T) {
	// TurnEnd 自身不直接对应前端事件（usage 由 MessageEnd 承担，stopped 由 app_ai 在 stopGeneration 时发）。
	// Cancelled 也是观察事件 —— 取消由 app_ai 的 stopGeneration 直接发 "stopped"，bridge 不重复发。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTurnEnd})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventCancelled})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventToolDelta})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventCompacted})
	assert.Len(t, em.events, 0)
}

func TestBridge_QueueConsumedBatch_FlushedBeforeToolStart(t *testing.T) {
	// 队列 flush 必须出现在后续 runner 事件之前 —— 否则前端会先看到 tool_start，
	// 再回填用户消息，时序错乱。
	em := &captureEmitter{}
	b := newBridge(42, em)
	userMsg := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.MetadataBlock{Key: "display", Value: "queued"},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Message: &userMsg})

	b.OnRunnerEvent(context.Background(), agent.Event{
		Kind: agent.EventPreToolUse,
		Tool: &agent.ToolEvent{Name: "x", ToolUseID: "1", Input: map[string]any{}},
	})

	assert.Equal(t, []string{"queue_consumed_batch", "tool_start"}, eventTypes(em.events))
}

func TestBridge_QueueConsumedBatch_OnlyFlushedOnce(t *testing.T) {
	// flush 后 pending 应清空，下一个 runner 事件不应该重发 queue_consumed_batch。
	em := &captureEmitter{}
	b := newBridge(42, em)
	userMsg := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.MetadataBlock{Key: "display", Value: "queued"},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Message: &userMsg})

	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "a"})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "b"})

	assert.Equal(t, []string{"queue_consumed_batch", "content", "content"}, eventTypes(em.events))
}
