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

	// inThinking marks that the most recently emitted runner delta was a
	// thinking delta. When the next non-thinking event arrives (text, tool,
	// turn end, done, cancel, error), the bridge emits a synthetic
	// "thinking_done" before that event so the frontend can transition the
	// running thinking block to completed. cago has no explicit thinking-end
	// event; we infer the boundary from the next observable event.
	inThinking bool
}

// newBridge constructs a bridge for the given conversation. The bridge does
// not subscribe automatically; callers (Manager / ConvHandle) wire
// OnRunnerEvent to runner.OnEvent and OnConvChange to conv.Watch().
func newBridge(convID int64, em EventEmitter) *eventBridge {
	return &eventBridge{convID: convID, emit: em}
}

// OnConvChange handles a single Conv.Watch Change. Append-of-user-message
// records the display string into pending; everything else is ignored.
// (Recorder owns persistence; bridge only cares about user-append signals
// for the queue_consumed_batch UX feature.)
func (b *eventBridge) OnConvChange(_ context.Context, ch agent.Change) {
	if ch.Kind != agent.ChangeAppended || ch.Message == nil {
		return
	}
	if ch.Message.Role != agent.RoleUser {
		return
	}
	display := extractDisplayText(ch.Message.Content)
	b.mu.Lock()
	b.pending = append(b.pending, display)
	b.mu.Unlock()
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
				Type:    "retry",
				Content: fmt.Sprintf("%d/?", ev.Retry.Attempt),
				Error:   cause,
			})
		}

	case agent.EventDone:
		b.emit.Emit(b.convID, ai.StreamEvent{Type: "done"})

	case agent.EventCancelled, agent.EventTurnEnd,
		agent.EventToolDelta, agent.EventCompacted:
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

// extractDisplayText finds the first MetadataBlock{Key:"display"} in the
// message content and returns its Value as a string. Returns empty string
// when not found or when the value is not a string.
func extractDisplayText(blocks []agent.ContentBlock) string {
	for _, b := range blocks {
		if mb, ok := b.(agent.MetadataBlock); ok && mb.Key == "display" {
			if s, ok := mb.Value.(string); ok {
				return s
			}
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
