package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// eventBridge translates cago Runner events and Conversation changes into
// OpsKat ai.StreamEvent payloads emitted via EventEmitter. One instance per
// active conversation. Both ingestion paths are mutex-serialized so emits
// arrive in a consistent order.
//
// Invariants:
//   - emit is the only way out; nothing else writes to the EventEmitter.
//   - User-message appends (from Conv.Watch ChangeAppended) buffer their
//     display-text into pending; the next OnRunnerEvent call flushes them as a
//     single queue_consumed_batch event before the runner event itself.
//   - EventDone is the unique done signal; cancel/error don't re-emit done.
type eventBridge struct {
	convID int64
	emit   EventEmitter

	mu      sync.Mutex
	pending []string

	// skipNextUser 标记下一条 user-role ChangeAppended 不应该缓存到 pending。
	// 由 ConvHandle.Send 在走 Send-new-turn 路径前 prime——前端那时已经本地 push
	// 了 user 气泡，bridge 再发 queue_consumed_batch 会导致：(1) 重复 user 气泡
	// (2) 当前 streaming asst placeholder 被钉成空气泡。
	//
	// 不用 counter 是因为单 conv 同时只有一次 Send 在飞——bool 足够，避免泄漏。
	// 只被 user-role append 消费；其他 role 不动它。
	skipNextUser bool

	// inThinking marks that the most recently emitted runner delta was a
	// thinking delta. When the next non-thinking event arrives (text, tool,
	// turn end, done, cancel, error), the bridge emits a synthetic
	// "thinking_done" before that event so the frontend can transition the
	// running thinking block to completed. cago has no explicit thinking-end
	// event; we infer the boundary from the next observable event.
	inThinking bool
}

// SkipNextUserAppend 让 bridge 跳过下一条 user-role ChangeAppended 的 pending 缓存。
// 只对最近的一次 user-append 生效，被消费即清零。
// 由 ConvHandle.Send（Send-new-turn 分支）和 ConvHandle.Edit 调用。
func (b *eventBridge) SkipNextUserAppend() {
	b.mu.Lock()
	b.skipNextUser = true
	b.mu.Unlock()
}

// newBridge constructs a bridge for the given conversation. The bridge does
// not subscribe automatically; callers (Manager / ConvHandle) wire
// OnRunnerEvent to runner.OnEvent and OnConvChange to conv.Watch().
func newBridge(convID int64, em EventEmitter) *eventBridge {
	return &eventBridge{convID: convID, emit: em}
}

// OnConvChange handles a single Conv.Watch Change.
//
// 两条职责：
//
//	(1) ChangeAppended + RoleUser：把 raw display 缓存到 pending，等下一个
//	    runner 事件一起 flush 成 queue_consumed_batch（steer drain UX）。
//	    Send-new-turn / Edit 路径会先 SkipNextUserAppend，消费一次后跳过。
//	(2) ChangeAppended + RoleAssistant：cago openPartial 在 turn 起手就
//	    append 空 assistant + PartialReason=streaming（Recorder 同步写库拿到
//	    sort_order）。bridge 必须把 sort_order 透传给前端，否则 stop-before
//	    -content 时占位 sortOrder 一直为空，regenerate/edit 全部静默失败。
//
// 其他 Change（Truncated/Finalized）由 Recorder 直接写库，bridge 不重复关心。
func (b *eventBridge) OnConvChange(_ context.Context, ch agent.Change) {
	if ch.Kind != agent.ChangeAppended || ch.Message == nil {
		return
	}
	switch ch.Message.Role {
	case agent.RoleUser:
		display := extractDisplayText(ch.Message.Content)
		b.mu.Lock()
		if b.skipNextUser {
			// Send-new-turn / Edit 路径：前端已经本地 push 了 user 气泡，bridge 不
			// 该再 echo。DisplayTextBlock 同时承担「历史回放渲染」职责，所以它必须
			// 经由 Recorder 落库（Audience 含 ToStore），但这里不能再走
			// queue_consumed_batch 链路。
			b.skipNextUser = false
			b.mu.Unlock()
			return
		}
		b.pending = append(b.pending, display)
		b.mu.Unlock()
	case agent.RoleAssistant:
		idx := ch.Index
		b.emit.Emit(b.convID, ai.StreamEvent{
			Type:      "message_indexed",
			Role:      string(agent.RoleAssistant),
			SortOrder: &idx,
		})
	}
}

// OnRunnerEvent handles a single Runner Event. Before processing the event,
// any pending user-append displays are flushed as a queue_consumed_batch event.
// The flush and the subsequent event emit happen in the same goroutine with no
// goroutines spawned, preserving strict ordering.
func (b *eventBridge) OnRunnerEvent(_ context.Context, ev agent.Event) {
	b.mu.Lock()
	pending := b.pending
	b.pending = nil
	b.mu.Unlock()

	if len(pending) > 0 {
		b.emit.Emit(b.convID, ai.StreamEvent{
			Type:          "queue_consumed_batch",
			QueueContents: pending,
		})
	}

	// thinking_done boundary inference: if the last delta was thinking and now
	// we're about to emit anything else, flush a thinking_done first.
	b.maybeEmitThinkingDone(ev.Kind)

	switch ev.Kind {
	case agent.EventTextDelta:
		b.emit.Emit(b.convID, ai.StreamEvent{Type: "content", Content: ev.Delta})

	case agent.EventThinkingDelta:
		b.inThinking = true
		b.emit.Emit(b.convID, ai.StreamEvent{Type: "thinking", Content: ev.Delta})

	case agent.EventPreToolUse:
		if ev.Tool != nil {
			// 前端 streamEvent reducer 只识别 "tool_start"（建工具气泡）+ "tool_result"
			// （回填结果），错发 "tool_call" 会被静默丢弃，导致工具调用整段不渲染。
			b.emit.Emit(b.convID, ai.StreamEvent{
				Type:       "tool_start",
				ToolName:   ev.Tool.Name,
				ToolInput:  stringifyMap(ev.Tool.Input),
				ToolCallID: ev.Tool.ToolUseID,
			})
		}

	case agent.EventPostToolUse:
		if ev.Tool != nil && ev.Tool.Output != nil {
			content := serializeOutputBlocks(ev.Tool.Output.Content)
			if ev.Tool.Output.IsError {
				b.emit.Emit(b.convID, ai.StreamEvent{
					Type:       "tool_result",
					ToolName:   ev.Tool.Name,
					Content:    content,
					Error:      content,
					ToolCallID: ev.Tool.ToolUseID,
				})
			} else {
				b.emit.Emit(b.convID, ai.StreamEvent{
					Type:       "tool_result",
					ToolName:   ev.Tool.Name,
					Content:    content,
					ToolCallID: ev.Tool.ToolUseID,
				})
			}
		}

	case agent.EventMessageEnd:
		// cago 把每轮 LLM 调用的 token 用量挂在 MessageEnd 的 Event.Usage 上。
		// 不发的话前端 TokenUsageBadge 永远是空 —— 用户拿不到本次成本。
		if ev.Usage != nil {
			b.emit.Emit(b.convID, ai.StreamEvent{
				Type: "usage",
				Usage: &ai.Usage{
					InputTokens:         ev.Usage.PromptTokens,
					OutputTokens:        ev.Usage.CompletionTokens,
					CacheReadTokens:     ev.Usage.CachedTokens,
					CacheCreationTokens: ev.Usage.CacheCreationTokens,
				},
			})
		}

	case agent.EventError:
		errMsg := ""
		if ev.Error != nil {
			errMsg = ev.Error.Error()
		}
		b.emit.Emit(b.convID, ai.StreamEvent{Type: "error", Error: errMsg})

	case agent.EventRetry:
		if ev.Retry != nil {
			cause := ""
			if ev.Retry.Cause != nil {
				cause = ev.Retry.Cause.Error()
			}
			b.emit.Emit(b.convID, ai.StreamEvent{
				Type: "retry",
				// Content 留着 "N/?" 字串作历史回放兼容；新字段 RetryAttempt /
				// RetryDelayMs 让前端能做倒计时 + 更可控的文案格式化。
				Content:      fmt.Sprintf("%d/?", ev.Retry.Attempt),
				Error:        cause,
				RetryAttempt: ev.Retry.Attempt,
				RetryDelayMs: ev.Retry.Delay.Milliseconds(),
			})
		}

	case agent.EventDone:
		b.emit.Emit(b.convID, ai.StreamEvent{Type: "done"})

	case agent.EventCancelled:
		// cago 的任何 cancel 路径（user-stop / ctx 超时 / framework 内部 cancel）都会
		// 先发 EventCancelled 再 emitDone。前端 "done" handler 把 running tool block
		// 标 "completed"，"stopped" 才标 "cancelled" —— 不发 stopped，被 cancel 的工具
		// 气泡看起来像成功完成。bridge 在这里发 stopped 是单源；app_ai.StopAIGeneration
		// 走 h.Cancel(...) 时不再重复直发，仅在 handle 缺失时兜底（见 app_ai.go）。
		b.emit.Emit(b.convID, ai.StreamEvent{Type: "stopped"})

	case agent.EventTurnEnd, agent.EventToolDelta, agent.EventCompacted:
		// Observational or handled elsewhere — nothing to emit.
		// (EventMessageEnd is handled above for the usage emit.)
	}
}

// maybeEmitThinkingDone emits a synthetic "thinking_done" event when leaving
// a thinking run. Idempotent: clears inThinking after emit. Called inside
// OnRunnerEvent before the kind-specific switch so the event order is
// thinking_delta…thinking_done…content / tool_start / done.
func (b *eventBridge) maybeEmitThinkingDone(nextKind agent.EventKind) {
	if !b.inThinking {
		return
	}
	if nextKind == agent.EventThinkingDelta {
		return // still thinking; not a boundary
	}
	b.inThinking = false
	b.emit.Emit(b.convID, ai.StreamEvent{Type: "thinking_done"})
}

// extractDisplayText finds the first DisplayTextBlock in the message content
// and returns its Text. Returns empty string when not found.
//
// DisplayTextBlock 的 Audience 是 ToUI|ToStore（不含 LLM），是 cago 新 API 里替
// 代老 MetadataBlock{Key:"display"} 的「raw 用户输入」承载体。bridge 把它解出来
// 是为了在 steer-drain 时通过 queue_consumed_batch 让前端补 user 气泡 ——
// 前端要的是 raw（"@srv1 状态"），不是 expanded body。
func extractDisplayText(blocks []agent.ContentBlock) string {
	for _, b := range blocks {
		if d, ok := b.(agent.DisplayTextBlock); ok {
			return d.Text
		}
	}
	return ""
}

func stringifyMap(m map[string]any) string {
	if m == nil {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

func serializeOutputBlocks(blocks []agent.ContentBlock) string {
	out := serializeBlocks(blocks)
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}
