package ai

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClaudeEventParser(t *testing.T) {
	Convey("Claude stream-json 事件解析", t, func() {
		parser := NewClaudeEventParser()

		Convey("解析 system init 事件", func() {
			events, done := parser.ParseLine(`{"type":"system","subtype":"init","session_id":"abc-123"}`)
			So(events, ShouldBeEmpty)
			So(done, ShouldBeFalse)
			So(parser.SessionID, ShouldEqual, "abc-123")
		})

		Convey("解析 text_delta 事件", func() {
			events, done := parser.ParseLine(`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}}`)
			So(done, ShouldBeFalse)
			So(events, ShouldHaveLength, 1)
			So(events[0].Type, ShouldEqual, "content")
			So(events[0].Content, ShouldEqual, "Hello")
		})

		Convey("解析 tool_use 开始事件", func() {
			events, done := parser.ParseLine(`{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","name":"Bash","id":"tool_1"}}}`)
			So(done, ShouldBeFalse)
			So(events, ShouldHaveLength, 1)
			So(events[0].Type, ShouldEqual, "tool_start")
			So(events[0].ToolName, ShouldEqual, "Bash")
		})

		Convey("解析 input_json_delta 累积工具输入", func() {
			// 开始 tool_use
			parser.ParseLine(`{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","name":"Bash","id":"tool_1"}}}`)

			// 输入 delta
			events, _ := parser.ParseLine(`{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","text":"{\"command\":\"ls -la\"}"}}}`)
			So(events, ShouldBeEmpty) // delta 不直接产生事件

			// content_block_stop → 发送带 input 的 tool_start
			events, done := parser.ParseLine(`{"type":"stream_event","event":{"type":"content_block_stop","index":1}}`)
			So(done, ShouldBeFalse)
			So(events, ShouldHaveLength, 1)
			So(events[0].Type, ShouldEqual, "tool_start")
			So(events[0].ToolName, ShouldEqual, "Bash")
			So(events[0].ToolInput, ShouldEqual, "ls -la")
		})

		Convey("解析 result 事件标记完成", func() {
			events, done := parser.ParseLine(`{"type":"result","result":"分析完成","session_id":"abc-123"}`)
			So(events, ShouldBeEmpty)
			So(done, ShouldBeTrue)
		})

		Convey("解析 assistant 消息不重复发送", func() {
			events, done := parser.ParseLine(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"服务器分析结果"}]}}`)
			So(done, ShouldBeFalse)
			So(events, ShouldBeEmpty)
		})

		Convey("空行不报错", func() {
			events, done := parser.ParseLine("")
			So(events, ShouldBeEmpty)
			So(done, ShouldBeFalse)
		})

		Convey("无效 JSON 返回错误事件", func() {
			events, done := parser.ParseLine("not json")
			So(done, ShouldBeFalse)
			So(events, ShouldHaveLength, 1)
			So(events[0].Type, ShouldEqual, "error")
		})
	})
}
