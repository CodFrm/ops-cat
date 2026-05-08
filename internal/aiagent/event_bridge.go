package aiagent

import (
	"encoding/json"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// bridge translates cago agent.Event values into OpsKat ai.StreamEvent values
// and emits them through the EventEmitter. One bridge instance per Stream.
//
// State:
//   - thinkingActive: synthesize thinking_done when a non-thinking event
//     interrupts a thinking-delta sequence.
//   - pendingFollowUps: cago drainInjections 逐条 emit EventUserPromptSubmit
//     (Kind=MessageKindFollowUp)；这里把它们累积成一批，等下一个非 follow-up
//     事件到来时合并 emit 一次 queue_consumed_batch，前端只需追加一个 assistant
//     placeholder（避免逐条 push 出一串空 assistant 气泡）。
//   - popDisplay: 从 *System 的 displayContent FIFO 拿展示原文。cago Message.Text
//     是带 mention 上下文的 LLM body，不能直接给前端展示。
type bridge struct {
	emit             EventEmitter
	popDisplay       func() string
	thinkingActive   bool
	pendingFollowUps []string
}

func newBridge(em EventEmitter, popDisplay func() string) *bridge {
	if popDisplay == nil {
		popDisplay = func() string { return "" }
	}
	return &bridge{emit: em, popDisplay: popDisplay}
}

func (b *bridge) translate(convID int64, ev agent.Event) {
	// follow-up 类 UserPromptSubmit 直接累积到 pendingFollowUps，不做后续翻译。
	// drainInjections 在 runloop 单 goroutine 内连续 emit，期间不会插入其他 Kind
	// 事件，所以累积窗口安全。
	if ev.Kind == agent.EventUserPromptSubmit && ev.Message != nil &&
		ev.Message.Kind == agent.MessageKindFollowUp {
		b.pendingFollowUps = append(b.pendingFollowUps, b.popDisplay())
		return
	}

	// 非 follow-up 事件到来 → 先把累积的 follow-up 一次性 flush 给前端。
	if len(b.pendingFollowUps) > 0 {
		b.emit.Emit(convID, ai.StreamEvent{
			Type:          "queue_consumed_batch",
			QueueContents: b.pendingFollowUps,
		})
		b.pendingFollowUps = nil
	}

	// Synthesize thinking_done when a non-thinking event arrives after thinking deltas.
	if b.thinkingActive && ev.Kind != agent.EventThinkingDelta {
		b.emit.Emit(convID, ai.StreamEvent{Type: "thinking_done"})
		b.thinkingActive = false
	}

	switch ev.Kind {
	case agent.EventTextDelta:
		b.emit.Emit(convID, ai.StreamEvent{Type: "content", Content: ev.Text})
	case agent.EventThinkingDelta:
		b.thinkingActive = true
		b.emit.Emit(convID, ai.StreamEvent{Type: "thinking", Content: ev.Text})
	case agent.EventPreToolUse:
		if ev.Tool != nil {
			b.emit.Emit(convID, ai.StreamEvent{
				Type:       "tool_start",
				ToolName:   ev.Tool.Name,
				ToolInput:  string(ev.Tool.Input),
				ToolCallID: ev.Tool.ID,
			})
		}
	case agent.EventPostToolUse:
		if ev.Tool != nil {
			var content string
			_ = json.Unmarshal(ev.Tool.Response, &content)
			if content == "" {
				content = string(ev.Tool.Response)
			}
			b.emit.Emit(convID, ai.StreamEvent{
				Type:       "tool_result",
				ToolName:   ev.Tool.Name,
				Content:    content,
				ToolCallID: ev.Tool.ID,
			})
		}
	case agent.EventUsage:
		u := ev.Usage
		b.emit.Emit(convID, ai.StreamEvent{
			Type: "usage",
			Usage: &ai.Usage{
				InputTokens:         u.PromptTokens,
				OutputTokens:        u.CompletionTokens,
				CacheReadTokens:     u.CachedTokens,
				CacheCreationTokens: u.CacheCreationTokens,
			},
		})
	case agent.EventDone:
		b.emit.Emit(convID, ai.StreamEvent{Type: "done"})
	case agent.EventError:
		var msg string
		if ev.Err != nil {
			msg = ev.Err.Error()
		}
		b.emit.Emit(convID, ai.StreamEvent{Type: "error", Error: msg})
	}
}
