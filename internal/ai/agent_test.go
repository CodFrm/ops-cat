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
}

func (m *mockProvider) Name() string { return "mock" }

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
		})

		assert.NoError(t, err)
		assert.Len(t, executor.calls, 0) // 没有 tool 调用

		// 应有 content + done 事件
		contentEvents := 0
		doneEvents := 0
		for _, e := range events {
			if e.Type == "content" {
				contentEvents++
			}
			if e.Type == "done" {
				doneEvents++
			}
		}
		assert.Equal(t, 2, contentEvents)
		assert.Equal(t, 1, doneEvents)
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
		}, func(e StreamEvent) {})

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

func (c *captureMockProvider) Chat(ctx context.Context, msgs []Message, tools []Tool) (<-chan StreamEvent, error) {
	// 第二轮调用时捕获完整 messages
	if c.inner.round > 0 {
		*c.captured = append(*c.captured, msgs...)
	}
	return c.inner.Chat(ctx, msgs, tools)
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
		})

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
