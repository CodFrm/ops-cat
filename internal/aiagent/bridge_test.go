package aiagent

import (
	"context"
	"errors"
	"testing"
	"time"

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
			Delay:   1500 * time.Millisecond,
			Cause:   errors.New("503"),
		},
	})
	require := assert.New(t)
	require.Equal("retry", em.events[0].Type)
	require.Contains(em.events[0].Content, "2/")
	require.Contains(em.events[0].Error, "503")
	// 新字段：前端 RetryCountdownBubble 用 RetryDelayMs/RetryAttempt 做倒计时。
	// 没透传过去前端只能拿 "N/?" 字符串做不了进度条。
	require.Equal(2, em.events[0].RetryAttempt)
	require.Equal(int64(1500), em.events[0].RetryDelayMs)
}

func TestBridge_QueueConsumedBatch_AggregatesUserAppends(t *testing.T) {
	em := &captureEmitter{}
	b := newBridge(42, em)

	// Two user-message appends in a row (e.g., from Steer'd queue draining)
	userMsgA := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "expanded A"},
		agent.DisplayTextBlock{Text: "raw A"},
	}}
	userMsgB := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "expanded B"},
		agent.DisplayTextBlock{Text: "raw B"},
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

	// Assistant message append shouldn't queue anything, but it must emit
	// message_indexed so the frontend can fill the streaming placeholder's
	// sortOrder（regenerate/edit 都按 sortOrder 走，没它就静默失败）。
	asstMsg := agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{agent.TextBlock{Text: "hi"}}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Index: 3, Message: &asstMsg})

	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "hello"})

	// Expected: message_indexed (from assistant append) then content.
	assert.Equal(t, []string{"message_indexed", "content"}, eventTypes(em.events))
	idx := em.events[0]
	assert.Equal(t, "assistant", idx.Role)
	if assert.NotNil(t, idx.SortOrder) {
		assert.Equal(t, 3, *idx.SortOrder)
	}
}

func TestBridge_AssistantAppend_EmitsMessageIndexed(t *testing.T) {
	// cago openPartial 在 turn 起手就 conv.Append(空 assistant + PartialReason=streaming)，
	// Recorder 同步把这行写库（有 SortOrder）。bridge 必须把这个 sort_order 透传给
	// 前端，否则 stop-before-content 时占位 sortOrder 一直为空，regenerate/edit 都
	// 拿不到 idx 跑去 RegenerateAIMessage(convId, sortOrder) —— 后端不知道截哪儿。
	em := &captureEmitter{}
	b := newBridge(42, em)

	asst := agent.Message{Role: agent.RoleAssistant, PartialReason: agent.PartialStreaming}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Index: 7, Message: &asst})

	assert.Len(t, em.events, 1)
	ev := em.events[0]
	assert.Equal(t, "message_indexed", ev.Type)
	assert.Equal(t, "assistant", ev.Role)
	if assert.NotNil(t, ev.SortOrder) {
		assert.Equal(t, 7, *ev.SortOrder)
	}
}

func TestBridge_AssistantAppend_IgnoresFinalizedAndOtherChanges(t *testing.T) {
	// ChangeFinalized / ChangeTruncated 不需要再发 message_indexed — Append 已经
	// 一次性把 sort_order 通报给前端，Finalize 只是改 PartialReason，前端按现有
	// stopped/error 路径打状态即可。
	em := &captureEmitter{}
	b := newBridge(42, em)

	asst := agent.Message{Role: agent.RoleAssistant}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeFinalized, Index: 4, Message: &asst})
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeTruncated, Index: 2})

	assert.Len(t, em.events, 0)
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

func TestBridge_TurnEnd_AndOthersObservational(t *testing.T) {
	// TurnEnd 自身不直接对应前端事件（usage 由 MessageEnd 承担，cancel/done 都在 EventDone 路径上）。
	// ToolDelta / Compacted 同样是观察事件。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTurnEnd})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventToolDelta})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventCompacted})
	assert.Len(t, em.events, 0)
}

func TestBridge_Cancelled_EmitsStopped(t *testing.T) {
	// cago 任何取消路径都会先 EventCancelled 再 emitDone。
	// 前端 "done" handler 把 running tool block 标 "completed"，"stopped" 才标 "cancelled" —
	// 不发 stopped 的话，cancel 出来的工具气泡看起来像成功完成的。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{
		Kind:          agent.EventCancelled,
		StopReason:    agent.StopCancelled,
		PartialReason: "user",
	})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "stopped", em.events[0].Type)
}

func TestBridge_Cancelled_TimeoutEmitsStopped(t *testing.T) {
	// ctx 超时路径 (StopTimeout) 同样必须发 stopped — 这是历史上完全没覆盖的路径，
	// 之前 cancel 在 bridge 全被吞，前端只会看到 done，cancel 状态彻底丢失。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{
		Kind:       agent.EventCancelled,
		StopReason: agent.StopTimeout,
	})
	assert.Len(t, em.events, 1)
	assert.Equal(t, "stopped", em.events[0].Type)
}

func TestBridge_Cancelled_ThenDone_BothEmitted(t *testing.T) {
	// 真实序列：EventCancelled → EventTurnEnd → EventDone
	// bridge 应发 stopped + done；TurnEnd 仍是 observational。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventCancelled, PartialReason: "user"})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTurnEnd})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventDone})
	assert.Equal(t, []string{"stopped", "done"}, eventTypes(em.events))
}

func TestBridge_ThinkingDone_BeforeStopped(t *testing.T) {
	// 思考中被 cancel：thinking → cancel 之间也要有 thinking_done，前端才能把 thinking 块切到 completed。
	em := &captureEmitter{}
	b := newBridge(42, em)
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventThinkingDelta, Delta: "x"})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventCancelled, PartialReason: "user"})
	assert.Equal(t, []string{"thinking", "thinking_done", "stopped"}, eventTypes(em.events))
}

func TestBridge_QueueConsumedBatch_FlushedBeforeToolStart(t *testing.T) {
	// 队列 flush 必须出现在后续 runner 事件之前 —— 否则前端会先看到 tool_start，
	// 再回填用户消息，时序错乱。
	em := &captureEmitter{}
	b := newBridge(42, em)
	userMsg := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.DisplayTextBlock{Text: "queued"},
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
		agent.DisplayTextBlock{Text: "queued"},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Message: &userMsg})

	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "a"})
	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "b"})

	assert.Equal(t, []string{"queue_consumed_batch", "content", "content"}, eventTypes(em.events))
}

func TestBridge_SkipNextUserAppend_SuppressesQueueConsumed(t *testing.T) {
	// handle.Send 走 Send-new-turn 路径时（前端已自己 push 过 user 气泡），
	// 应该 prime bridge 跳过下一条 user-append——否则带 @-mention 时 display
	// 非空，会被误当作 steer-drain 重发 queue_consumed_batch，前端再 push 一条
	// 重复 user 气泡 + 把当前 streaming assistant 钉成空气泡。
	em := &captureEmitter{}
	b := newBridge(42, em)

	b.SkipNextUserAppend()

	// 模拟 cago.runner.Send 内部 conv.Append 触发的 user-append（带 display）。
	userMsg := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "expanded with mention ctx"},
		agent.DisplayTextBlock{Text: "@asset 检查"},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Message: &userMsg})

	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "hi"})

	// 没有 queue_consumed_batch — 跳过生效。
	assert.Equal(t, []string{"content"}, eventTypes(em.events))
}

func TestBridge_SkipNextUserAppend_OnlyConsumesOneUserAppend(t *testing.T) {
	// 第二条 user-append（典型场景：mid-turn steer drain）必须正常 buffer。
	em := &captureEmitter{}
	b := newBridge(42, em)

	b.SkipNextUserAppend()
	first := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.DisplayTextBlock{Text: "new-turn raw"},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Message: &first})

	second := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.DisplayTextBlock{Text: "drained"},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Message: &second})

	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "x"})

	assert.Equal(t, []string{"queue_consumed_batch", "content"}, eventTypes(em.events))
	assert.Equal(t, []string{"drained"}, em.events[0].QueueContents)
}

func TestBridge_SkipNextUserAppend_DoesNotConsumeAssistantAppend(t *testing.T) {
	// flag 只能被 user-role append 消费——assistant 等其他 append 不应该把 flag 啃掉。
	em := &captureEmitter{}
	b := newBridge(42, em)

	b.SkipNextUserAppend()

	asst := agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{
		agent.TextBlock{Text: "..."},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Index: 1, Message: &asst})

	// 现在才轮到真正的 user-append——flag 还要在。
	userMsg := agent.Message{Role: agent.RoleUser, Content: []agent.ContentBlock{
		agent.DisplayTextBlock{Text: "new-turn raw"},
	}}
	b.OnConvChange(context.Background(), agent.Change{Kind: agent.ChangeAppended, Message: &userMsg})

	b.OnRunnerEvent(context.Background(), agent.Event{Kind: agent.EventTextDelta, Delta: "x"})

	// assistant append 仍然 emit message_indexed（不消耗 skipNextUser 标记）；
	// 接着的 user-append 被 skip 标记吞掉，所以没有 queue_consumed_batch。
	assert.Equal(t, []string{"message_indexed", "content"}, eventTypes(em.events))
}
