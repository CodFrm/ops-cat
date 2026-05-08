package aiagent

import (
	"encoding/json"

	"github.com/cago-frame/agents/agent"

	"github.com/opskat/opskat/internal/ai"
)

// bridge translates cago agent.Event values into OpsKat ai.StreamEvent values
// and emits them through the EventEmitter. One bridge instance per Stream.
//
// State: tracks "thinking active" so we can synthesize a thinking_done event
// when a non-thinking event interrupts a thinking-delta sequence.
type bridge struct {
	emit           EventEmitter
	thinkingActive bool
}

func newBridge(em EventEmitter) *bridge { return &bridge{emit: em} }

func (b *bridge) translate(convID int64, ev agent.Event) {
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
