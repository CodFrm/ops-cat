package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// newTestProc 创建一个用于测试的 CLIProcess，stdin 写入 buf
func newTestProc() (*CLIProcess, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return &CLIProcess{
		stdin: nopWriteCloser{buf},
	}, buf
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func TestHandleNotification(t *testing.T) {
	Convey("Codex handleNotification 事件分发", t, func() {
		s := NewCodexAppServer()

		collectEvents := func() (func(StreamEvent), *[]StreamEvent) {
			var events []StreamEvent
			return func(e StreamEvent) { events = append(events, e) }, &events
		}

		Convey("v1 agent_message_delta 解析为 content 事件", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "codex/event/agent_message_delta",
				Params: json.RawMessage(`{"msg":{"delta":"Hello world"}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "content")
			So((*events)[0].Content, ShouldEqual, "Hello world")
		})

		Convey("v2 item/agentMessage/delta 顶层 delta 格式", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "item/agentMessage/delta",
				Params: json.RawMessage(`{"delta":"v2 text"}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "content")
			So((*events)[0].Content, ShouldEqual, "v2 text")
		})

		Convey("v2 item/agentMessage/delta 嵌套 item.delta 格式", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "item/agentMessage/delta",
				Params: json.RawMessage(`{"item":{"delta":"nested text"}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "content")
			So((*events)[0].Content, ShouldEqual, "nested text")
		})

		Convey("v2 item/agentMessage/delta 空 delta 不产生事件", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "item/agentMessage/delta",
				Params: json.RawMessage(`{"delta":""}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 0)
		})

		Convey("item/started commandExecution 产生 tool_start", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "item/started",
				Params: json.RawMessage(`{"item":{"type":"commandExecution","command":"ls -la"}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "tool_start")
			So((*events)[0].ToolName, ShouldEqual, "Bash")
			So((*events)[0].ToolInput, ShouldEqual, "ls -la")
		})

		Convey("item/completed commandExecution 产生 tool_result", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "item/completed",
				Params: json.RawMessage(`{"item":{"type":"commandExecution","output":"file.txt","exitCode":1}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "tool_result")
			So((*events)[0].Content, ShouldContainSubstring, "exit code 1")
		})

		Convey("item/started MCP tool call 产生 tool_start", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "item/started",
				Params: json.RawMessage(`{"item":{"type":"mcpToolCall","id":"call_123","server":"opskat","tool":"get_asset","arguments":{"id":7}}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "tool_start")
			So((*events)[0].ToolName, ShouldEqual, "get_asset")
			So((*events)[0].ToolInput, ShouldContainSubstring, "7")
		})

		Convey("item/completed MCP tool call 产生 tool_result（嵌套 result 结构）", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "item/completed",
				Params: json.RawMessage(`{"item":{"type":"mcpToolCall","id":"call_123","server":"opskat","tool":"get_asset","result":{"content":[{"type":"text","text":"{\"ID\":7,\"Name\":\"server1\"}"}]}}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "tool_result")
			So((*events)[0].ToolName, ShouldEqual, "get_asset")
			So((*events)[0].Content, ShouldContainSubstring, "server1")
		})

		Convey("item/started 未知类型用 type 作为 toolName", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "item/started",
				Params: json.RawMessage(`{"item":{"type":"unknownTool","command":"do something"}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "tool_start")
			So((*events)[0].ToolName, ShouldEqual, "unknownTool")
			So((*events)[0].ToolInput, ShouldEqual, "do something")
		})

		Convey("item/started agentMessage 不产生事件", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "item/started",
				Params: json.RawMessage(`{"item":{"type":"agentMessage"}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 0)
		})

		Convey("v1 codex/event/item_started MCP tool call 产生 tool_start", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "codex/event/item_started",
				Params: json.RawMessage(`{"item":{"type":"mcpToolCall","server":"opskat","tool":"run_command","arguments":{"asset_id":1}}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "tool_start")
			So((*events)[0].ToolName, ShouldEqual, "run_command")
		})

		Convey("v1 codex/event/item_completed MCP tool call 产生 tool_result", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "codex/event/item_completed",
				Params: json.RawMessage(`{"item":{"type":"mcpToolCall","server":"opskat","tool":"run_command","result":{"content":[{"type":"text","text":"success"}]}}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "tool_result")
			So((*events)[0].ToolName, ShouldEqual, "run_command")
			So((*events)[0].Content, ShouldEqual, "success")
		})

		Convey("turn/completed 返回 done=true", func() {
			onEvent, _ := collectEvents()
			msg := codexJSONRPC{Method: "turn/completed"}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeTrue)
		})

		Convey("turn/failed 返回 done=true 并发送 error 事件", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "turn/failed",
				Params: json.RawMessage(`{"error":"something went wrong"}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeTrue)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "error")
			So((*events)[0].Error, ShouldEqual, "something went wrong")
		})

		Convey("静默忽略的事件不产生事件也不结束 turn", func() {
			onEvent, events := collectEvents()
			for _, method := range []string{
				"thread/started",
				"thread/status/changed",
				"serverRequest/resolved",
				"configWarning",
				"codex/event/request_user_input",
			} {
				done := s.handleNotification(codexJSONRPC{Method: method}, onEvent)
				So(done, ShouldBeFalse)
			}
			So(*events, ShouldHaveLength, 0)
		})
	})
}

func TestHandleUserInputRequest(t *testing.T) {
	Convey("Codex MCP 工具权限确认流程", t, func() {
		s := NewCodexAppServer()
		proc, stdinBuf := newTestProc()
		s.proc = proc

		makeParams := func(questionID string, options []string) json.RawMessage {
			opts := make([]map[string]string, len(options))
			for i, o := range options {
				opts[i] = map[string]string{"label": o, "description": ""}
			}
			p := map[string]any{
				"threadId": "thread-1",
				"turnId":   "turn-1",
				"itemId":   "item-1",
				"questions": []map[string]any{{
					"id":       questionID,
					"header":   "Approve app tool call?",
					"question": "run ls",
					"options":  opts,
				}},
			}
			data, _ := json.Marshal(p)
			return data
		}

		Convey("自动批准不发送额外事件（item/started 已处理）", func() {
			var events []StreamEvent
			onEvent := func(e StreamEvent) { events = append(events, e) }

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.ctx = ctx

			params := makeParams("confirm-1", []string{"Allow", "Allow for this session", "Cancel"})

			s.handleUserInputRequest(nil, params, onEvent)

			So(events, ShouldHaveLength, 0)
		})

		Convey("带 requestID 时通过 WriteJSON 发送 JSON-RPC response", func() {
			var events []StreamEvent
			onEvent := func(e StreamEvent) { events = append(events, e) }

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.ctx = ctx
			stdinBuf.Reset()

			params := makeParams("mcp_q1", []string{"Allow", "Allow for this session", "Cancel"})
			requestID := int64(7)

			s.handleUserInputRequest(&requestID, params, onEvent)

			// 不发送额外事件
			So(events, ShouldHaveLength, 0)

			// 验证写入 stdin 的 JSON-RPC response
			var written codexJSONRPC
			err := json.Unmarshal(bytes.TrimSpace(stdinBuf.Bytes()), &written)
			So(err, ShouldBeNil)
			So(written.ID, ShouldNotBeNil)
			So(*written.ID, ShouldEqual, int64(7))
			So(written.Method, ShouldBeEmpty)

			var result map[string]any
			_ = json.Unmarshal(written.Result, &result)
			answers := result["answers"].(map[string]any)
			qAnswer := answers["mcp_q1"].(map[string]any)
			answerList := qAnswer["answers"].([]any)
			So(answerList[0], ShouldEqual, "Allow for this session")
		})

		Convey("无效 JSON 不 panic 不产生事件", func() {
			var events []StreamEvent
			onEvent := func(e StreamEvent) { events = append(events, e) }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.ctx = ctx

			s.handleUserInputRequest(nil, json.RawMessage(`invalid`), onEvent)
			So(events, ShouldHaveLength, 0)
		})

		Convey("空 questions 不 panic 不产生事件", func() {
			var events []StreamEvent
			onEvent := func(e StreamEvent) { events = append(events, e) }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.ctx = ctx

			s.handleUserInputRequest(nil, json.RawMessage(`{"questions":[]}`), onEvent)
			So(events, ShouldHaveLength, 0)
		})
	})
}

func TestMcpResultText(t *testing.T) {
	Convey("mcpResultText 从嵌套结构提取结果", t, func() {
		Convey("单个 text content", func() {
			item := &codexItem{
				Result: json.RawMessage(`{"content":[{"type":"text","text":"hello world"}]}`),
			}
			So(item.mcpResultText(), ShouldEqual, "hello world")
		})

		Convey("多个 text content 合并", func() {
			item := &codexItem{
				Result: json.RawMessage(`{"content":[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]}`),
			}
			So(item.mcpResultText(), ShouldEqual, "line1\nline2")
		})

		Convey("空 content 数组", func() {
			item := &codexItem{
				Result: json.RawMessage(`{"content":[]}`),
			}
			So(item.mcpResultText(), ShouldEqual, "")
		})

		Convey("nil Result", func() {
			item := &codexItem{}
			So(item.mcpResultText(), ShouldEqual, "")
		})

		Convey("无效 JSON", func() {
			item := &codexItem{
				Result: json.RawMessage(`invalid`),
			}
			So(item.mcpResultText(), ShouldEqual, "")
		})

		Convey("跳过空 text 的 content", func() {
			item := &codexItem{
				Result: json.RawMessage(`{"content":[{"type":"text","text":""},{"type":"text","text":"real data"}]}`),
			}
			So(item.mcpResultText(), ShouldEqual, "real data")
		})

		Convey("真实 Codex 返回格式", func() {
			item := &codexItem{
				Result: json.RawMessage(`{"content":[{"type":"text","text":"{\"ID\":7,\"Name\":\"k3s-master-1\"}"}],"structuredContent":null}`),
			}
			So(item.mcpResultText(), ShouldContainSubstring, "k3s-master-1")
		})
	})
}

func TestHandleItemStarted(t *testing.T) {
	Convey("handleItemStarted 事件分发", t, func() {
		s := NewCodexAppServer()
		collectEvents := func() (func(StreamEvent), *[]StreamEvent) {
			var events []StreamEvent
			return func(e StreamEvent) { events = append(events, e) }, &events
		}

		Convey("fileRead 产生 tool_start", func() {
			onEvent, events := collectEvents()
			s.handleItemStarted(&codexItem{Type: "fileRead", Path: "/etc/hosts"}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "Read")
			So((*events)[0].ToolInput, ShouldEqual, "/etc/hosts")
		})

		Convey("fileWrite 产生 tool_start", func() {
			onEvent, events := collectEvents()
			s.handleItemStarted(&codexItem{Type: "fileWrite", Path: "/tmp/out.txt"}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "Write")
			So((*events)[0].ToolInput, ShouldEqual, "/tmp/out.txt")
		})

		Convey("mcpToolCall 用 tool 字段作为名称", func() {
			onEvent, events := collectEvents()
			s.handleItemStarted(&codexItem{
				Type: "mcpToolCall",
				Tool: "list_assets",
				Args: json.RawMessage(`{"group_id":1}`),
			}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "list_assets")
			So((*events)[0].ToolInput, ShouldEqual, `{"group_id":1}`)
		})

		Convey("mcpToolCall tool 为空时 fallback 到 MCP Tool", func() {
			onEvent, events := collectEvents()
			s.handleItemStarted(&codexItem{Type: "mcpToolCall"}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "MCP Tool")
		})

		Convey("reasoning 不产生事件", func() {
			onEvent, events := collectEvents()
			s.handleItemStarted(&codexItem{Type: "reasoning"}, onEvent)
			So(*events, ShouldHaveLength, 0)
		})

		Convey("userMessage 不产生事件", func() {
			onEvent, events := collectEvents()
			s.handleItemStarted(&codexItem{Type: "userMessage"}, onEvent)
			So(*events, ShouldHaveLength, 0)
		})

		Convey("未知类型优先用 tool 字段", func() {
			onEvent, events := collectEvents()
			s.handleItemStarted(&codexItem{Type: "newToolType", Tool: "my_tool", Args: json.RawMessage(`{"k":"v"}`)}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "my_tool")
			So((*events)[0].ToolInput, ShouldEqual, `{"k":"v"}`)
		})

		Convey("未知类型 tool 为空时 fallback 到 type", func() {
			onEvent, events := collectEvents()
			s.handleItemStarted(&codexItem{Type: "customExec", Command: "echo hi"}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "customExec")
			So((*events)[0].ToolInput, ShouldEqual, "echo hi")
		})

		Convey("未知类型 fallback input 优先级: args > command > path", func() {
			onEvent, events := collectEvents()
			s.handleItemStarted(&codexItem{Type: "x", Path: "/a"}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolInput, ShouldEqual, "/a")
		})

		Convey("commandExecution 空 command 不产生事件", func() {
			onEvent, events := collectEvents()
			s.handleItemStarted(&codexItem{Type: "commandExecution"}, onEvent)
			So(*events, ShouldHaveLength, 0)
		})
	})
}

func TestHandleItemCompleted(t *testing.T) {
	Convey("handleItemCompleted 事件分发", t, func() {
		s := NewCodexAppServer()
		collectEvents := func() (func(StreamEvent), *[]StreamEvent) {
			var events []StreamEvent
			return func(e StreamEvent) { events = append(events, e) }, &events
		}

		Convey("commandExecution 正常退出", func() {
			onEvent, events := collectEvents()
			exitCode := 0
			s.handleItemCompleted(&codexItem{Type: "commandExecution", Output: "ok", ExitCode: &exitCode}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "Bash")
			So((*events)[0].Content, ShouldEqual, "ok")
		})

		Convey("commandExecution 非零退出码", func() {
			onEvent, events := collectEvents()
			exitCode := 2
			s.handleItemCompleted(&codexItem{Type: "commandExecution", Output: "error msg", ExitCode: &exitCode}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Content, ShouldContainSubstring, "exit code 2")
			So((*events)[0].Content, ShouldContainSubstring, "error msg")
		})

		Convey("fileRead 产生 tool_result", func() {
			onEvent, events := collectEvents()
			s.handleItemCompleted(&codexItem{Type: "fileRead", Content: json.RawMessage(`"file content here"`)}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "Read")
			So((*events)[0].Content, ShouldEqual, "file content here")
		})

		Convey("fileWrite 产生 tool_result", func() {
			onEvent, events := collectEvents()
			s.handleItemCompleted(&codexItem{Type: "fileWrite", Path: "/tmp/out.txt"}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "Write")
			So((*events)[0].Content, ShouldEqual, "/tmp/out.txt")
		})

		Convey("mcpToolCall 从 result.content 提取结果", func() {
			onEvent, events := collectEvents()
			s.handleItemCompleted(&codexItem{
				Type:   "mcpToolCall",
				Tool:   "run_command",
				Result: json.RawMessage(`{"content":[{"type":"text","text":"cmd output"}]}`),
			}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "run_command")
			So((*events)[0].Content, ShouldEqual, "cmd output")
		})

		Convey("mcpToolCall tool 为空时 fallback 到 MCP Tool", func() {
			onEvent, events := collectEvents()
			s.handleItemCompleted(&codexItem{
				Type:   "mcpToolCall",
				Result: json.RawMessage(`{"content":[{"type":"text","text":"data"}]}`),
			}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "MCP Tool")
		})

		Convey("mcpToolCall 无 result 时 content 为空", func() {
			onEvent, events := collectEvents()
			s.handleItemCompleted(&codexItem{Type: "mcpToolCall", Tool: "t"}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Content, ShouldEqual, "")
		})

		Convey("agentMessage 不产生事件", func() {
			onEvent, events := collectEvents()
			s.handleItemCompleted(&codexItem{Type: "agentMessage", Text: "should ignore"}, onEvent)
			So(*events, ShouldHaveLength, 0)
		})

		Convey("reasoning 不产生事件", func() {
			onEvent, events := collectEvents()
			s.handleItemCompleted(&codexItem{Type: "reasoning"}, onEvent)
			So(*events, ShouldHaveLength, 0)
		})

		Convey("未知类型优先用 tool 字段和 mcpResultText", func() {
			onEvent, events := collectEvents()
			s.handleItemCompleted(&codexItem{
				Type:   "futureType",
				Tool:   "new_tool",
				Result: json.RawMessage(`{"content":[{"type":"text","text":"future result"}]}`),
			}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "new_tool")
			So((*events)[0].Content, ShouldEqual, "future result")
		})

		Convey("未知类型 fallback output > contentString", func() {
			onEvent, events := collectEvents()
			s.handleItemCompleted(&codexItem{Type: "x", Output: "out data"}, onEvent)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].ToolName, ShouldEqual, "x")
			So((*events)[0].Content, ShouldEqual, "out data")
		})
	})
}

func TestHandleNotificationExecCommand(t *testing.T) {
	Convey("v1 exec_command 事件", t, func() {
		s := NewCodexAppServer()
		collectEvents := func() (func(StreamEvent), *[]StreamEvent) {
			var events []StreamEvent
			return func(e StreamEvent) { events = append(events, e) }, &events
		}

		Convey("exec_command_begin 产生 tool_start", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "codex/event/exec_command_begin",
				Params: json.RawMessage(`{"msg":{"command":"docker ps"}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "tool_start")
			So((*events)[0].ToolName, ShouldEqual, "Bash")
			So((*events)[0].ToolInput, ShouldEqual, "docker ps")
		})

		Convey("exec_command_end 正常退出产生 tool_result", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "codex/event/exec_command_end",
				Params: json.RawMessage(`{"msg":{"exit_code":0,"stdout":"CONTAINER ID\n","stderr":""}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "tool_result")
			So((*events)[0].ToolName, ShouldEqual, "Bash")
			So((*events)[0].Content, ShouldContainSubstring, "CONTAINER ID")
		})

		Convey("exec_command_end 非零退出码包含 stderr", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "codex/event/exec_command_end",
				Params: json.RawMessage(`{"msg":{"exit_code":1,"stdout":"","stderr":"not found"}}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Content, ShouldContainSubstring, "exit code 1")
			So((*events)[0].Content, ShouldContainSubstring, "not found")
		})

		Convey("exec_command_end stdout+stderr 合并", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "codex/event/exec_command_end",
				Params: json.RawMessage(`{"msg":{"exit_code":0,"stdout":"out","stderr":"warn"}}`),
			}
			s.handleNotification(msg, onEvent)
			So((*events)[0].Content, ShouldEqual, "out\nwarn")
		})
	})
}

func TestHandleNotificationRealCodexData(t *testing.T) {
	Convey("真实 Codex 数据格式（来自实际日志）", t, func() {
		s := NewCodexAppServer()
		collectEvents := func() (func(StreamEvent), *[]StreamEvent) {
			var events []StreamEvent
			return func(e StreamEvent) { events = append(events, e) }, &events
		}

		Convey("MCP 完整流程: started → completed", func() {
			onEvent, events := collectEvents()

			// item/started（真实格式）
			started := codexJSONRPC{
				Method: "item/started",
				Params: json.RawMessage(`{"item":{"type":"mcpToolCall","id":"call_Udxr1k5eghPjvQFkkZBlJY09","server":"opskat","tool":"run_command","status":"inProgress","arguments":{"asset_id":7,"command":"ls -la"},"result":null,"error":null,"durationMs":null},"threadId":"019d19de-555a-7410-bcda-bcca3c13a188","turnId":"019d19e0-a0c9-7592-af35-f3495a5da0c5"}`),
			}
			done := s.handleNotification(started, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 1)
			So((*events)[0].Type, ShouldEqual, "tool_start")
			So((*events)[0].ToolName, ShouldEqual, "run_command")
			So((*events)[0].ToolInput, ShouldContainSubstring, "asset_id")

			// item/completed（真实格式）
			completed := codexJSONRPC{
				Method: "item/completed",
				Params: json.RawMessage(`{"item":{"type":"mcpToolCall","id":"call_Udxr1k5eghPjvQFkkZBlJY09","server":"opskat","tool":"run_command","status":"completed","arguments":{"asset_id":7,"command":"ls -la"},"result":{"content":[{"type":"text","text":"total 8\ndrwxr-xr-x 2 root root 4096 Mar 23 file.txt\n"}],"structuredContent":null},"error":null,"durationMs":10199},"threadId":"019d19de-555a-7410-bcda-bcca3c13a188","turnId":"019d19e0-a0c9-7592-af35-f3495a5da0c5"}`),
			}
			done = s.handleNotification(completed, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 2)
			So((*events)[1].Type, ShouldEqual, "tool_result")
			So((*events)[1].ToolName, ShouldEqual, "run_command")
			So((*events)[1].Content, ShouldContainSubstring, "file.txt")
		})

		Convey("agentMessage completed 不产生事件", func() {
			onEvent, events := collectEvents()
			msg := codexJSONRPC{
				Method: "item/completed",
				Params: json.RawMessage(`{"item":{"type":"agentMessage","id":"msg_abc","text":"some commentary","phase":"commentary","memoryCitation":null},"threadId":"t1","turnId":"t2"}`),
			}
			done := s.handleNotification(msg, onEvent)
			So(done, ShouldBeFalse)
			So(*events, ShouldHaveLength, 0)
		})

		Convey("reasoning started/completed 不产生事件", func() {
			onEvent, events := collectEvents()
			started := codexJSONRPC{
				Method: "item/started",
				Params: json.RawMessage(`{"item":{"type":"reasoning","id":"rs_abc","summary":[],"content":[]}}`),
			}
			completed := codexJSONRPC{
				Method: "item/completed",
				Params: json.RawMessage(`{"item":{"type":"reasoning","id":"rs_abc","summary":[],"content":[]}}`),
			}
			s.handleNotification(started, onEvent)
			s.handleNotification(completed, onEvent)
			So(*events, ShouldHaveLength, 0)
		})
	})
}

func TestTruncateOutput(t *testing.T) {
	Convey("truncateOutput 截断逻辑", t, func() {
		Convey("短内容不截断", func() {
			So(truncateOutput("hello", 20), ShouldEqual, "hello")
		})

		Convey("刚好 maxLines 不截断", func() {
			input := "line1\nline2\nline3"
			So(truncateOutput(input, 3), ShouldEqual, input)
		})

		Convey("超过 maxLines 截断并显示剩余行数", func() {
			input := "1\n2\n3\n4\n5\n6"
			result := truncateOutput(input, 3)
			So(result, ShouldContainSubstring, "1\n2\n3")
			So(result, ShouldContainSubstring, "2 more lines")
		})

		Convey("无换行不截断", func() {
			So(truncateOutput("no newlines here", 1), ShouldEqual, "no newlines here")
		})

		Convey("空字符串", func() {
			So(truncateOutput("", 10), ShouldEqual, "")
		})
	})
}

func TestMCPToolApprovalOptionSelection(t *testing.T) {
	Convey("MCP 确认 option label 选择逻辑", t, func() {
		// 复现 handleUserInputRequest 中的 option 选择逻辑
		selectAnswer := func(behavior string, options []string) string {
			type opt struct {
				Label string
			}
			opts := make([]opt, len(options))
			for i, o := range options {
				opts[i] = opt{Label: o}
			}

			answer := "Cancel"
			if len(opts) > 0 {
				switch behavior {
				case "allow":
					answer = opts[0].Label
				case "allowAll":
					if len(opts) > 1 {
						answer = opts[1].Label
					}
				case "deny":
					if len(opts) > 2 {
						answer = opts[2].Label
					}
				default:
					answer = opts[len(opts)-1].Label
				}
			}
			return answer
		}

		Convey("4 个 option (Approve Once / Approve this Session / Deny / Cancel)", func() {
			opts := []string{"Approve Once", "Approve this Session", "Deny", "Cancel"}
			So(selectAnswer("allow", opts), ShouldEqual, "Approve Once")
			So(selectAnswer("allowAll", opts), ShouldEqual, "Approve this Session")
			So(selectAnswer("deny", opts), ShouldEqual, "Deny")
			So(selectAnswer("unknown", opts), ShouldEqual, "Cancel")
		})

		Convey("3 个 option (Allow / Allow for this session / Cancel)", func() {
			opts := []string{"Allow", "Allow for this session", "Cancel"}
			So(selectAnswer("allow", opts), ShouldEqual, "Allow")
			So(selectAnswer("allowAll", opts), ShouldEqual, "Allow for this session")
			So(selectAnswer("deny", opts), ShouldEqual, "Cancel")
		})

		Convey("只有 2 个 option 时 deny 回退到默认 Cancel", func() {
			opts := []string{"Allow", "Cancel"}
			So(selectAnswer("allow", opts), ShouldEqual, "Allow")
			So(selectAnswer("deny", opts), ShouldEqual, "Cancel")
		})

		Convey("空 options 返回默认 Cancel", func() {
			So(selectAnswer("allow", nil), ShouldEqual, "Cancel")
		})
	})
}

func TestMCPResponseJSONFormat(t *testing.T) {
	Convey("MCP JSON-RPC response 格式验证", t, func() {
		Convey("response 包含正确的 id 和 answers 结构", func() {
			questionID := "mcp_tool_call_approval_call_xyz"
			answer := "Allow"
			requestID := int64(42)

			responseResult := map[string]any{
				"answers": map[string]any{
					questionID: map[string]any{
						"answers": []string{answer},
					},
				},
			}
			resultData, _ := json.Marshal(responseResult)
			replyMsg := codexJSONRPC{
				ID:     &requestID,
				Result: resultData,
			}

			fullJSON, err := json.Marshal(replyMsg)
			So(err, ShouldBeNil)

			var parsed map[string]any
			_ = json.Unmarshal(fullJSON, &parsed)

			So(parsed["id"], ShouldEqual, float64(42))
			_, hasMethod := parsed["method"]
			So(hasMethod, ShouldBeFalse)

			result := parsed["result"].(map[string]any)
			answers := result["answers"].(map[string]any)
			qAnswer := answers[questionID].(map[string]any)
			answerList := qAnswer["answers"].([]any)
			So(answerList, ShouldHaveLength, 1)
			So(answerList[0], ShouldEqual, "Allow")
		})

		Convey("requestID=0 也能正确序列化（不被 omitempty 忽略）", func() {
			requestID := int64(0)
			replyMsg := codexJSONRPC{
				ID:     &requestID,
				Result: json.RawMessage(`{"answers":{}}`),
			}

			fullJSON, _ := json.Marshal(replyMsg)
			var parsed map[string]any
			_ = json.Unmarshal(fullJSON, &parsed)

			So(parsed["id"], ShouldEqual, float64(0))
		})
	})
}
