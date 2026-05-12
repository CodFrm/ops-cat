package ai

import (
	"encoding/json"
	"strings"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
)

// EventTranslator 把 cago agent.Event 翻译成 OpsKat 现有 StreamEvent。
//
// 前端契约（type 字段名 + 内部字段）保持不变；这里只是从 cago 的 event 流里
// 反推出旧 provider 的事件形态，保证前端 ai:event:{convID} 监听者无感知。
//
// 翻译是有状态的：thinking → content / tool_start 之间的转移需要插入一条
// "thinking_done"，与旧 anthropic.go 一致。EventTranslator 实例每个对话每轮 1 个。
//
// 审批事件（approval_request / approval_result）由 app_ai.go 的 confirmFunc 直接
// emit，不走 cago event 流，因此不在本适配器范围。
type EventTranslator struct {
	inThinking bool
}

// NewStreamTranslator 创建一个新的事件翻译器。
func NewStreamTranslator() *EventTranslator {
	return &EventTranslator{}
}

// Translate 处理一条 cago agent.Event，调用 emit 0..N 次发出对应 StreamEvent。
//
// 多发场景：thinking → content 转移会先发 "thinking_done" 再发 "content"。
// 不映射的 EventKind（如 EventMessageEnd / EventToolDelta）直接静默——
// 前者由 EventTextDelta / EventTurnEnd 隐含，后者是 cago 流式工具参数增量，
// 旧 OpsKat StreamEvent 没有对应字段，可丢弃。
func (t *EventTranslator) Translate(ev agent.Event, emit func(StreamEvent)) {
	switch ev.Kind {
	case agent.EventTextDelta:
		t.flushThinking(emit)
		emit(StreamEvent{Type: "content", Content: ev.Delta})

	case agent.EventThinkingDelta:
		t.inThinking = true
		emit(StreamEvent{Type: "thinking", Content: ev.Delta})

	case agent.EventPreToolUse:
		t.flushThinking(emit)
		if ev.Tool == nil {
			return
		}
		var input string
		if ev.Tool.Input != nil {
			if b, err := json.Marshal(ev.Tool.Input); err == nil {
				input = string(b)
			}
		}
		emit(StreamEvent{
			Type:       "tool_start",
			ToolName:   ev.Tool.Name,
			ToolInput:  input,
			ToolCallID: ev.Tool.ToolUseID,
		})

	case agent.EventPostToolUse:
		if ev.Tool == nil {
			return
		}
		emit(StreamEvent{
			Type:       "tool_result",
			ToolName:   ev.Tool.Name,
			ToolCallID: ev.Tool.ToolUseID,
			Content:    extractToolResultText(ev.Tool.Output),
		})

	case agent.EventTurnEnd:
		t.flushThinking(emit)
		if ev.Usage != nil {
			emit(StreamEvent{Type: "usage", Usage: convertUsage(ev.Usage)})
		}

	case agent.EventRetry:
		msg := ""
		if ev.Retry != nil && ev.Retry.Cause != nil {
			msg = ev.Retry.Cause.Error()
		} else if ev.Error != nil {
			msg = ev.Error.Error()
		}
		emit(StreamEvent{Type: "retry", Error: msg})

	case agent.EventCancelled:
		emit(StreamEvent{Type: "stopped"})

	case agent.EventError:
		msg := ""
		if ev.Error != nil {
			msg = ev.Error.Error()
		}
		emit(StreamEvent{Type: "error", Error: msg})

	case agent.EventCompacted:
		// 新事件类型——前端尚未识别就当未知 type 忽略，不会破坏渲染。
		emit(StreamEvent{Type: "compacted"})

	case agent.EventSteerConsumed:
		// cago 把一条 Steer 队列消息追加到 conv —— 翻译为前端契约里的
		// queue_consumed：UI 据此收尾当前 assistant 气泡、插 user 气泡、开新流，
		// 并从 pendingQueue 弹出首条。Delta 是 displayText（含 @mention 原样），
		// 没有 display 时回落 LLM 文本，前端把它当 user 消息内容渲染。
		t.flushThinking(emit)
		emit(StreamEvent{Type: "queue_consumed", Content: ev.Delta})

	case agent.EventDone:
		t.flushThinking(emit)
		emit(StreamEvent{Type: "done"})

	case agent.EventMessageEnd, agent.EventToolDelta:
		// 见上注释，不映射。
	}
}

// flushThinking 若之前发了 thinking 现在要发非 thinking 内容，先发 "thinking_done" 收尾。
func (t *EventTranslator) flushThinking(emit func(StreamEvent)) {
	if t.inThinking {
		emit(StreamEvent{Type: "thinking_done"})
		t.inThinking = false
	}
}

// extractToolResultText 把 cago ToolResultBlock 的 Content（异构 ContentBlock 切片）
// 拼成一段文本——OpsKat 前端 tool_result 当前只渲染纯文本。
// 非文本 block（image / 嵌套 tool_result 等）目前 OpsKat 没有 tool 真在用，
// 遇到就跳过；如未来需要透传图像/结构化结果再扩展。
func extractToolResultText(blk *agent.ToolResultBlock) string {
	if blk == nil {
		return ""
	}
	var sb strings.Builder
	for _, c := range blk.Content {
		if tb, ok := c.(*agent.TextBlock); ok {
			sb.WriteString(tb.Text)
			continue
		}
		// 兼容值类型 TextBlock，blocks.TextBlock 的方法接收器是值
		if tb, ok := c.(agent.TextBlock); ok {
			sb.WriteString(tb.Text)
		}
	}
	return sb.String()
}

// convertUsage 把 cago provider.Usage 折算成 OpsKat Usage。
//
// 映射规则：
//   - InputTokens         ← PromptTokens
//   - OutputTokens        ← CompletionTokens（ReasoningTokens 已合并进 CompletionTokens 由 provider 上送）
//   - CacheCreationTokens ← CacheCreationTokens
//   - CacheReadTokens     ← CachedTokens
func convertUsage(u *provider.Usage) *Usage {
	if u == nil {
		return nil
	}
	return &Usage{
		InputTokens:         u.PromptTokens,
		OutputTokens:        u.CompletionTokens,
		CacheCreationTokens: u.CacheCreationTokens,
		CacheReadTokens:     u.CachedTokens,
	}
}
