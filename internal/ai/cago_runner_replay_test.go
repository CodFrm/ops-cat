package ai

import (
	"testing"

	"github.com/cago-frame/agents/agent"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSplitForReplay(t *testing.T) {
	Convey("splitForReplay", t, func() {
		Convey("空列表 → 全空", func() {
			h, last := splitForReplay(nil)
			So(h, ShouldBeNil)
			So(last, ShouldEqual, "")
		})
		Convey("末尾是 user → 切走最后一条作为 lastUserText", func() {
			msgs := []Message{
				{Role: RoleUser, Content: "hello"},
				{Role: RoleAssistant, Content: "hi"},
				{Role: RoleUser, Content: "now what"},
			}
			h, last := splitForReplay(msgs)
			So(h, ShouldHaveLength, 2)
			So(last, ShouldEqual, "now what")
		})
		Convey("末尾不是 user（边界） → 全部归 history", func() {
			msgs := []Message{
				{Role: RoleUser, Content: "hello"},
				{Role: RoleAssistant, Content: "hi"},
			}
			h, last := splitForReplay(msgs)
			So(h, ShouldHaveLength, 2)
			So(last, ShouldEqual, "")
		})
	})
}

func TestConvertToCagoMessages(t *testing.T) {
	Convey("convertToCagoMessages", t, func() {
		Convey("user 消息 → RoleUser + TextBlock", func() {
			out := convertToCagoMessages([]Message{{Role: RoleUser, Content: "hi"}})
			So(out, ShouldHaveLength, 1)
			So(out[0].Role, ShouldEqual, agent.RoleUser)
			So(out[0].Content, ShouldHaveLength, 1)
			tb, ok := out[0].Content[0].(*agent.TextBlock)
			So(ok, ShouldBeTrue)
			So(tb.Text, ShouldEqual, "hi")
		})

		Convey("assistant 消息 + thinking + tool_call → Thinking/Text/ToolUse blocks 顺序拼接", func() {
			tc := ToolCall{
				ID:   "tu_1",
				Type: "function",
			}
			tc.Function.Name = "run_command"
			tc.Function.Arguments = `{"asset_id":1,"command":"uptime"}`
			out := convertToCagoMessages([]Message{
				{
					Role:      RoleAssistant,
					Content:   "let me run that",
					Thinking:  "the user wants uptime",
					ToolCalls: []ToolCall{tc},
				},
			})
			So(out, ShouldHaveLength, 1)
			So(out[0].Role, ShouldEqual, agent.RoleAssistant)
			So(out[0].Content, ShouldHaveLength, 3)

			tk, ok := out[0].Content[0].(*agent.ThinkingBlock)
			So(ok, ShouldBeTrue)
			So(tk.Text, ShouldEqual, "the user wants uptime")

			tb, ok := out[0].Content[1].(*agent.TextBlock)
			So(ok, ShouldBeTrue)
			So(tb.Text, ShouldEqual, "let me run that")

			tu, ok := out[0].Content[2].(*agent.ToolUseBlock)
			So(ok, ShouldBeTrue)
			So(tu.ID, ShouldEqual, "tu_1")
			So(tu.Name, ShouldEqual, "run_command")
			So(tu.State, ShouldEqual, agent.ToolUseReady)
			So(tu.Input["command"], ShouldEqual, "uptime")
		})

		Convey("assistant 消息 ToolCall.Arguments 非法 JSON → ToolUseMalformed", func() {
			tc := ToolCall{ID: "tu_x", Type: "function"}
			tc.Function.Name = "run_command"
			tc.Function.Arguments = "not-json"
			out := convertToCagoMessages([]Message{
				{Role: RoleAssistant, ToolCalls: []ToolCall{tc}},
			})
			So(out, ShouldHaveLength, 1)
			So(out[0].Content, ShouldHaveLength, 1)
			tu, ok := out[0].Content[0].(*agent.ToolUseBlock)
			So(ok, ShouldBeTrue)
			So(tu.State, ShouldEqual, agent.ToolUseMalformed)
			So(tu.RawArgs, ShouldEqual, "not-json")
		})

		Convey("tool 消息 → RoleTool + ToolResultBlock", func() {
			out := convertToCagoMessages([]Message{
				{Role: RoleTool, ToolCallID: "tu_1", Content: "ok-result"},
			})
			So(out, ShouldHaveLength, 1)
			So(out[0].Role, ShouldEqual, agent.RoleTool)
			tr, ok := out[0].Content[0].(*agent.ToolResultBlock)
			So(ok, ShouldBeTrue)
			So(tr.ToolUseID, ShouldEqual, "tu_1")
			inner, ok := tr.Content[0].(*agent.TextBlock)
			So(ok, ShouldBeTrue)
			So(inner.Text, ShouldEqual, "ok-result")
		})

		Convey("system 消息被跳过（cago 不放进 Conversation）", func() {
			out := convertToCagoMessages([]Message{
				{Role: RoleSystem, Content: "you are concise"},
				{Role: RoleUser, Content: "hi"},
			})
			So(out, ShouldHaveLength, 1)
			So(out[0].Role, ShouldEqual, agent.RoleUser)
		})
	})
}
