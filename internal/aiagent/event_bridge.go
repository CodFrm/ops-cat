package aiagent

import (
	"encoding/json"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/conversation_entity"
)

// usageStasher 是 *System 的子集接口（避免 bridge 反向依赖 *System 全部 API）。
// 测试时传 nil 跳过 usage 缓存写入，bridge 仍照常 emit 给前端的 "usage" 事件。
type usageStasher interface {
	stashPendingUsage(cagoID string, u *conversation_entity.TokenUsage)
}

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
//   - usage：把 EventUsage 的 token 统计 stash 进 *System 的 pendingUsage 缓存，
//     键为 lastAssistantMsgID。下一次 cago Save 触发 gormStore.Save 时 drain。
//   - lastAssistantMsgID：最近一条 EventMessageEnd 中 origin=model 的 Message.ID。
//     EventUsage 不带 Message — 用这个回填 cago_id。多轮 stream 复用同一 bridge
//     实例，仅由后续 EventMessageEnd 覆盖；EventUsage 不清空它。
type bridge struct {
	emit               EventEmitter
	popDisplay         func() string
	usage              usageStasher
	thinkingActive     bool
	pendingFollowUps   []string
	lastAssistantMsgID string
}

func newBridgeLegacy(em EventEmitter, popDisplay func() string, usage usageStasher) *bridge {
	if popDisplay == nil {
		popDisplay = func() string { return "" }
	}
	return &bridge{emit: em, popDisplay: popDisplay, usage: usage}
}

func (b *bridge) translate(convID int64, ev agent.Event) {
	// follow-up 类 UserPromptSubmit 直接累积到 pendingFollowUps，不做后续翻译。
	// drainInjections 在 runloop 单 goroutine 内连续 emit，期间不会插入其他 Kind
	// 事件，所以累积窗口安全。
	//
	// 注意：cago runloop 在每次 Session.Stream 启动时也会遍历 req.History，对其中
	// 所有 Kind=MessageKindFollowUp 的历史消息重新 emit EventUserPromptSubmit
	// （cago/agents/agent/builtin_runloop.go:69-118 — 给 hook/observer 一次"看到所
	// 有历史 follow-up"的机会）。这些回放没有对应的 Steer 推送，popDisplay 会返回
	// ""；如果照样累积进 pendingFollowUps，前端会按 FIFO 写出 N 条空 user 气泡。
	// 真实的 mid-stream Steer 注入一定先 pushPendingDisplay 再 sess.FollowUp，所
	// 以 popDisplay 返回非空才是 live drain，返回空一律视作历史回放跳过。
	if ev.Kind == agent.EventUserPromptSubmit && ev.Message != nil &&
		ev.Message.Kind == agent.MessageKindFollowUp {
		display := b.popDisplay()
		if display == "" {
			return
		}
		b.pendingFollowUps = append(b.pendingFollowUps, display)
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
	case agent.EventMessageEnd:
		// cago 内部观察事件：每条 assistant model 消息收尾时触发，紧跟着的
		// EventUsage 会用这条 ID 回填 cago_id。bridge 不向前端 emit —
		// 文本/工具调用已通过 TextDelta/PreToolUse 等流式事件展示，
		// MessageEnd 只是为 usage 提供锚点。
		if ev.Message != nil &&
			ev.Message.Role == agent.RoleAssistant &&
			ev.Message.Origin == agent.MessageOriginModel {
			b.lastAssistantMsgID = ev.Message.ID
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
		if b.usage != nil && b.lastAssistantMsgID != "" {
			b.usage.stashPendingUsage(b.lastAssistantMsgID, &conversation_entity.TokenUsage{
				InputTokens:         u.PromptTokens,
				OutputTokens:        u.CompletionTokens,
				CacheReadTokens:     u.CachedTokens,
				CacheCreationTokens: u.CacheCreationTokens,
			})
		}
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
