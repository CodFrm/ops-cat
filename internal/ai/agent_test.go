package ai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

// mockProvider 模拟 AI provider，返回预设的响应
type mockProvider struct {
	responses [][]StreamEvent // 每轮对话的响应事件序列
	round     int
	model     string // 可选：测试模型相关分支时设置
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Model() string {
	if m.model != "" {
		return m.model
	}
	return "mock-model"
}

func (m *mockProvider) Chat(_ context.Context, _ []Message, _ []Tool) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		if m.round < len(m.responses) {
			for _, event := range m.responses[m.round] {
				ch <- event
			}
		}
		m.round++
	}()
	return ch, nil
}

// mockExecutor 模拟工具执行
type mockExecutor struct {
	calls []struct {
		Name string
		Args string
	}
	results map[string]string
}

func (m *mockExecutor) Execute(_ context.Context, name string, args string) (string, error) {
	m.calls = append(m.calls, struct {
		Name string
		Args string
	}{name, args})
	if result, ok := m.results[name]; ok {
		return result, nil
	}
	return `{"ok":true}`, nil
}

func TestAgent_SimpleChat(t *testing.T) {
	convey.Convey("Agent 简单对话（无 tool 调用）", t, func() {
		provider := &mockProvider{
			responses: [][]StreamEvent{
				{
					{Type: "content", Content: "你好"},
					{Type: "content", Content: "！"},
					{Type: "done"},
				},
			},
		}
		executor := &mockExecutor{}
		agent := NewAgent(provider, func() ToolExecutor { return executor }, nil, NewDefaultConfig())

		var events []StreamEvent
		err := agent.Chat(context.Background(), []Message{
			{Role: RoleUser, Content: "你好"},
		}, func(e StreamEvent) {
			events = append(events, e)
		}, nil)

		assert.NoError(t, err)
		assert.Len(t, executor.calls, 0) // 没有 tool 调用

		// 应有 content 事件（done 由 ConversationRunner 发出，不在 Chat 中）
		contentEvents := 0
		for _, e := range events {
			if e.Type == "content" {
				contentEvents++
			}
		}
		assert.Equal(t, 2, contentEvents)
	})
}

func TestToolCall_SerializationType(t *testing.T) {
	convey.Convey("ToolCall 序列化包含 type 字段", t, func() {
		tc := ToolCall{ID: "call_1", Type: "function"}
		tc.Function.Name = "list_assets"
		tc.Function.Arguments = `{"asset_type":"ssh"}`

		data, err := json.Marshal(tc)
		assert.NoError(t, err)

		var raw map[string]any
		err = json.Unmarshal(data, &raw)
		assert.NoError(t, err)
		assert.Equal(t, "function", raw["type"])
	})

	convey.Convey("包含 ToolCall 的 Message 序列化 type 字段", t, func() {
		tc := ToolCall{ID: "call_1", Type: "function"}
		tc.Function.Name = "run_command"
		tc.Function.Arguments = `{"command":"ls"}`

		msg := Message{
			Role:      RoleAssistant,
			Content:   "",
			ToolCalls: []ToolCall{tc},
		}

		data, err := json.Marshal(msg)
		assert.NoError(t, err)

		var raw map[string]any
		err = json.Unmarshal(data, &raw)
		assert.NoError(t, err)

		toolCalls := raw["tool_calls"].([]any)
		assert.Len(t, toolCalls, 1)
		call := toolCalls[0].(map[string]any)
		assert.Equal(t, "function", call["type"])
		assert.Equal(t, "call_1", call["id"])
	})
}

func TestAgent_ToolCallMessageIncludesType(t *testing.T) {
	convey.Convey("Agent tool 调用后构造的 assistant 消息包含 type 字段", t, func() {
		var capturedMessages []Message
		provider := &mockProvider{
			responses: [][]StreamEvent{
				{
					{Type: "tool_call", ToolCalls: []ToolCall{
						{ID: "call_1", Type: "function", Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{Name: "list_assets", Arguments: `{}`}},
					}},
					{Type: "done"},
				},
				{
					{Type: "content", Content: "done"},
					{Type: "done"},
				},
			},
		}
		// 替换 provider.Chat 以捕获发送的 messages
		captureProvider := &captureMockProvider{
			inner:    provider,
			captured: &capturedMessages,
		}

		executor := &mockExecutor{results: map[string]string{"list_assets": `[]`}}
		agent := NewAgent(captureProvider, func() ToolExecutor { return executor }, nil, NewDefaultConfig())

		err := agent.Chat(context.Background(), []Message{
			{Role: RoleUser, Content: "test"},
		}, func(e StreamEvent) {}, nil)

		assert.NoError(t, err)
		// 第二轮调用时 messages 应包含 assistant 的 tool_calls
		assert.True(t, len(capturedMessages) >= 3) // system可能没有，至少 user + assistant + tool
		// 找到 assistant 消息
		for _, msg := range capturedMessages {
			if msg.Role == RoleAssistant && len(msg.ToolCalls) > 0 {
				assert.Equal(t, "function", msg.ToolCalls[0].Type)
			}
		}
	})
}

// captureMockProvider 包装 mockProvider，捕获第二轮发送的 messages
type captureMockProvider struct {
	inner    *mockProvider
	captured *[]Message
}

func (c *captureMockProvider) Name() string { return "capture_mock" }

func (c *captureMockProvider) Model() string { return c.inner.Model() }

func (c *captureMockProvider) Chat(ctx context.Context, msgs []Message, tools []Tool) (<-chan StreamEvent, error) {
	// 第二轮调用时捕获完整 messages
	if c.inner.round > 0 {
		*c.captured = append(*c.captured, msgs...)
	}
	return c.inner.Chat(ctx, msgs, tools)
}

func TestAgent_ToolStartAndResultCarryToolCallID(t *testing.T) {
	convey.Convey("tool_start 与 tool_result 事件都带上 ToolCallID（前端用以跨 turn 还原 tool_calls）", t, func() {
		provider := &mockProvider{
			responses: [][]StreamEvent{
				{
					{Type: "tool_call", ToolCalls: []ToolCall{
						{ID: "call_xyz", Type: "function", Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{Name: "list_assets", Arguments: `{}`}},
					}},
					{Type: "done"},
				},
				{{Type: "content", Content: "ok"}, {Type: "done"}},
			},
		}
		executor := &mockExecutor{results: map[string]string{"list_assets": `[]`}}
		agent := NewAgent(provider, func() ToolExecutor { return executor }, nil, NewDefaultConfig())

		var events []StreamEvent
		err := agent.Chat(context.Background(), []Message{
			{Role: RoleUser, Content: "list"},
		}, func(e StreamEvent) { events = append(events, e) }, nil)
		assert.NoError(t, err)

		var startEvent, resultEvent *StreamEvent
		for i := range events {
			switch events[i].Type {
			case "tool_start":
				startEvent = &events[i]
			case "tool_result":
				resultEvent = &events[i]
			}
		}
		assert.NotNil(t, startEvent)
		assert.NotNil(t, resultEvent)
		assert.Equal(t, "call_xyz", startEvent.ToolCallID)
		assert.Equal(t, "call_xyz", resultEvent.ToolCallID)
	})
}

func TestAgent_ToolCallMessageCarriesReasoningContent(t *testing.T) {
	buildResponses := func() [][]StreamEvent {
		return [][]StreamEvent{
			{
				{Type: "thinking", Content: "先看一下"},
				{Type: "thinking", Content: "有哪些资产"},
				{Type: "thinking_done"},
				{Type: "tool_call", ToolCalls: []ToolCall{
					{ID: "call_1", Type: "function", Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: "list_assets", Arguments: `{}`}},
				}},
				{Type: "done"},
			},
			{{Type: "content", Content: "完成"}, {Type: "done"}},
		}
	}

	convey.Convey("DeepSeek-v4 模型：tool 调用轮 assistant 消息同时携带 Thinking 与 ReasoningContent", t, func() {
		var capturedMessages []Message
		provider := &mockProvider{responses: buildResponses(), model: "deepseek-v4-pro"}
		captureProvider := &captureMockProvider{inner: provider, captured: &capturedMessages}
		executor := &mockExecutor{results: map[string]string{"list_assets": `[]`}}
		agent := NewAgent(captureProvider, func() ToolExecutor { return executor }, nil, NewDefaultConfig())

		err := agent.Chat(context.Background(), []Message{{Role: RoleUser, Content: "列出资产"}},
			func(e StreamEvent) {}, nil)
		assert.NoError(t, err)

		var assistantMsg *Message
		for i := range capturedMessages {
			if capturedMessages[i].Role == RoleAssistant {
				assistantMsg = &capturedMessages[i]
				break
			}
		}
		assert.NotNil(t, assistantMsg)
		expected := "先看一下有哪些资产"
		assert.Equal(t, expected, assistantMsg.Thinking)
		assert.Equal(t, expected, assistantMsg.ReasoningContent, "DeepSeek-v4 多轮要求原样回传 reasoning_content")
	})

	convey.Convey("非 DeepSeek-v4 模型：仅写 Thinking，不写 ReasoningContent，避免对 OpenAI/Anthropic 等 provider 引入未知字段", t, func() {
		var capturedMessages []Message
		provider := &mockProvider{responses: buildResponses(), model: "gpt-4o"}
		captureProvider := &captureMockProvider{inner: provider, captured: &capturedMessages}
		executor := &mockExecutor{results: map[string]string{"list_assets": `[]`}}
		agent := NewAgent(captureProvider, func() ToolExecutor { return executor }, nil, NewDefaultConfig())

		err := agent.Chat(context.Background(), []Message{{Role: RoleUser, Content: "列出资产"}},
			func(e StreamEvent) {}, nil)
		assert.NoError(t, err)

		var assistantMsg *Message
		for i := range capturedMessages {
			if capturedMessages[i].Role == RoleAssistant {
				assistantMsg = &capturedMessages[i]
				break
			}
		}
		assert.NotNil(t, assistantMsg)
		assert.Equal(t, "先看一下有哪些资产", assistantMsg.Thinking)
		assert.Empty(t, assistantMsg.ReasoningContent, "非 DeepSeek-v4 不应写 ReasoningContent")
	})
}

func TestMessage_ReasoningContentJSON(t *testing.T) {
	convey.Convey("Message.ReasoningContent 序列化为 reasoning_content（DeepSeek/OpenAI 兼容字段）", t, func() {
		msg := Message{
			Role:             RoleAssistant,
			Content:          "",
			Thinking:         "thoughts",
			ReasoningContent: "thoughts",
		}
		data, err := json.Marshal(msg)
		assert.NoError(t, err)

		var raw map[string]any
		assert.NoError(t, json.Unmarshal(data, &raw))
		assert.Equal(t, "thoughts", raw["thinking"])
		assert.Equal(t, "thoughts", raw["reasoning_content"])
	})

	convey.Convey("ReasoningContent 为空时通过 omitempty 不出现在 JSON 中", t, func() {
		msg := Message{Role: RoleAssistant, Content: "hello"}
		data, err := json.Marshal(msg)
		assert.NoError(t, err)

		var raw map[string]any
		assert.NoError(t, json.Unmarshal(data, &raw))
		_, exists := raw["reasoning_content"]
		assert.False(t, exists, "空字符串应被 omitempty 略掉")
	})
}

func TestAgent_ToolCallLoop(t *testing.T) {
	convey.Convey("Agent tool 调用循环", t, func() {
		provider := &mockProvider{
			responses: [][]StreamEvent{
				// 第一轮：LLM 返回 tool 调用
				{
					{Type: "tool_call", ToolCalls: []ToolCall{
						{ID: "call_1", Type: "function", Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{Name: "list_assets", Arguments: `{"asset_type":"ssh"}`}},
					}},
					{Type: "done"},
				},
				// 第二轮：LLM 返回最终回复
				{
					{Type: "content", Content: "找到了2台服务器"},
					{Type: "done"},
				},
			},
		}
		executor := &mockExecutor{
			results: map[string]string{
				"list_assets": `[{"ID":1,"Name":"web-01"},{"ID":2,"Name":"web-02"}]`,
			},
		}
		agent := NewAgent(provider, func() ToolExecutor { return executor }, nil, NewDefaultConfig())

		var events []StreamEvent
		err := agent.Chat(context.Background(), []Message{
			{Role: RoleUser, Content: "列出所有SSH服务器"},
		}, func(e StreamEvent) {
			events = append(events, e)
		}, nil)

		assert.NoError(t, err)
		// executor 应被调用一次
		assert.Len(t, executor.calls, 1)
		assert.Equal(t, "list_assets", executor.calls[0].Name)

		// 最终应有 content 事件
		hasContent := false
		for _, e := range events {
			if e.Type == "content" {
				hasContent = true
			}
		}
		assert.True(t, hasContent)
	})
}
