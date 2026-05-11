package ai

import (
	"errors"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
	. "github.com/smartystreets/goconvey/convey"
)

// drain 把 emit 串变成切片，便于断言。
func drain(t *EventTranslator, evs ...agent.Event) []StreamEvent {
	var out []StreamEvent
	emit := func(e StreamEvent) { out = append(out, e) }
	for _, ev := range evs {
		t.Translate(ev, emit)
	}
	return out
}

func TestEventTranslator_TextDelta(t *testing.T) {
	Convey("EventTextDelta → content", t, func() {
		out := drain(NewEventTranslator(), agent.Event{
			Kind:  agent.EventTextDelta,
			Delta: "hi",
		})
		So(out, ShouldHaveLength, 1)
		So(out[0].Type, ShouldEqual, "content")
		So(out[0].Content, ShouldEqual, "hi")
	})
}

func TestEventTranslator_ThinkingThenContent(t *testing.T) {
	Convey("thinking 后跟 content 时插入 thinking_done", t, func() {
		out := drain(NewEventTranslator(),
			agent.Event{Kind: agent.EventThinkingDelta, Delta: "let me think"},
			agent.Event{Kind: agent.EventTextDelta, Delta: "answer"},
		)
		So(out, ShouldHaveLength, 3)
		So(out[0].Type, ShouldEqual, "thinking")
		So(out[0].Content, ShouldEqual, "let me think")
		So(out[1].Type, ShouldEqual, "thinking_done")
		So(out[2].Type, ShouldEqual, "content")
		So(out[2].Content, ShouldEqual, "answer")
	})
}

func TestEventTranslator_ThinkingThenToolStart(t *testing.T) {
	Convey("thinking 后直接调工具时也插入 thinking_done", t, func() {
		out := drain(NewEventTranslator(),
			agent.Event{Kind: agent.EventThinkingDelta, Delta: "..."},
			agent.Event{Kind: agent.EventPreToolUse, Tool: &agent.ToolEvent{
				ToolUseID: "tu_1",
				Name:      "run_command",
				Input:     map[string]any{"asset_id": float64(1), "command": "uptime"},
			}},
		)
		So(out, ShouldHaveLength, 3)
		So(out[1].Type, ShouldEqual, "thinking_done")
		So(out[2].Type, ShouldEqual, "tool_start")
		So(out[2].ToolName, ShouldEqual, "run_command")
		So(out[2].ToolCallID, ShouldEqual, "tu_1")
		So(out[2].ToolInput, ShouldContainSubstring, `"command":"uptime"`)
	})
}

func TestEventTranslator_PostToolUse(t *testing.T) {
	Convey("EventPostToolUse → tool_result（拼出 TextBlock 内容）", t, func() {
		out := drain(NewEventTranslator(),
			agent.Event{Kind: agent.EventPostToolUse, Tool: &agent.ToolEvent{
				ToolUseID: "tu_2",
				Name:      "exec_sql",
				Output: &agent.ToolResultBlock{
					Content: []agent.ContentBlock{
						&agent.TextBlock{Text: "row1\n"},
						&agent.TextBlock{Text: "row2\n"},
					},
				},
			}},
		)
		So(out, ShouldHaveLength, 1)
		So(out[0].Type, ShouldEqual, "tool_result")
		So(out[0].ToolName, ShouldEqual, "exec_sql")
		So(out[0].ToolCallID, ShouldEqual, "tu_2")
		So(out[0].Content, ShouldEqual, "row1\nrow2\n")
	})
}

func TestEventTranslator_TurnEndUsage(t *testing.T) {
	Convey("EventTurnEnd（带 Usage）→ usage", t, func() {
		out := drain(NewEventTranslator(),
			agent.Event{Kind: agent.EventTurnEnd, Usage: &provider.Usage{
				PromptTokens:        100,
				CompletionTokens:    50,
				CachedTokens:        20,
				CacheCreationTokens: 10,
			}},
		)
		So(out, ShouldHaveLength, 1)
		So(out[0].Type, ShouldEqual, "usage")
		So(out[0].Usage, ShouldNotBeNil)
		So(out[0].Usage.InputTokens, ShouldEqual, 100)
		So(out[0].Usage.OutputTokens, ShouldEqual, 50)
		So(out[0].Usage.CacheReadTokens, ShouldEqual, 20)
		So(out[0].Usage.CacheCreationTokens, ShouldEqual, 10)
	})
}

func TestEventTranslator_TurnEndNoUsage(t *testing.T) {
	Convey("EventTurnEnd（无 Usage）静默", t, func() {
		out := drain(NewEventTranslator(), agent.Event{Kind: agent.EventTurnEnd})
		So(out, ShouldBeEmpty)
	})
}

func TestEventTranslator_Retry(t *testing.T) {
	Convey("EventRetry → retry，错误信息透传", t, func() {
		out := drain(NewEventTranslator(), agent.Event{
			Kind: agent.EventRetry,
			Retry: &agent.RetryEvent{
				Attempt: 1,
				Delay:   2 * time.Second,
				Cause:   errors.New("timeout"),
			},
		})
		So(out, ShouldHaveLength, 1)
		So(out[0].Type, ShouldEqual, "retry")
		So(out[0].Error, ShouldEqual, "timeout")
	})
}

func TestEventTranslator_Canceled(t *testing.T) {
	Convey("EventCancelled → stopped", t, func() {
		out := drain(NewEventTranslator(), agent.Event{Kind: agent.EventCancelled})
		So(out, ShouldHaveLength, 1)
		So(out[0].Type, ShouldEqual, "stopped")
	})
}

func TestEventTranslator_Error(t *testing.T) {
	Convey("EventError → error 携带消息", t, func() {
		out := drain(NewEventTranslator(), agent.Event{
			Kind:  agent.EventError,
			Error: errors.New("boom"),
		})
		So(out, ShouldHaveLength, 1)
		So(out[0].Type, ShouldEqual, "error")
		So(out[0].Error, ShouldEqual, "boom")
	})
}

func TestEventTranslator_Compacted(t *testing.T) {
	Convey("EventCompacted → compacted（新 type）", t, func() {
		out := drain(NewEventTranslator(), agent.Event{Kind: agent.EventCompacted})
		So(out, ShouldHaveLength, 1)
		So(out[0].Type, ShouldEqual, "compacted")
	})
}

func TestEventTranslator_Done_FlushesThinking(t *testing.T) {
	Convey("EventDone 收尾，仍处于 thinking 时先发 thinking_done", t, func() {
		out := drain(NewEventTranslator(),
			agent.Event{Kind: agent.EventThinkingDelta, Delta: "..."},
			agent.Event{Kind: agent.EventDone},
		)
		So(out, ShouldHaveLength, 3)
		So(out[1].Type, ShouldEqual, "thinking_done")
		So(out[2].Type, ShouldEqual, "done")
	})
}

func TestEventTranslator_IgnoredKinds(t *testing.T) {
	Convey("EventMessageEnd / EventToolDelta 不映射", t, func() {
		out := drain(NewEventTranslator(),
			agent.Event{Kind: agent.EventMessageEnd},
			agent.Event{Kind: agent.EventToolDelta},
		)
		So(out, ShouldBeEmpty)
	})
}

func TestExtractToolResultText_SkipsNonText(t *testing.T) {
	Convey("extractToolResultText 跳过非文本 block", t, func() {
		text := extractToolResultText(&agent.ToolResultBlock{
			Content: []agent.ContentBlock{
				&agent.TextBlock{Text: "before"},
				&agent.ImageBlock{}, // 非文本，应被跳过
				&agent.TextBlock{Text: "after"},
			},
		})
		So(text, ShouldEqual, "beforeafter")
	})
}
