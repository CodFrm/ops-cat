package ai

import "context"

// Role 消息角色
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message 对话消息
type Message struct {
	Role       Role        `json:"role"`
	Content    string      `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"` // role=tool 时标识调用
}

// ToolCall AI 发起的工具调用
type ToolCall struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON string
	} `json:"function"`
}

// Tool 工具定义（OpenAI function calling 格式）
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction 工具函数定义
type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"` // JSON Schema
}

// StreamEvent 流式响应事件
type StreamEvent struct {
	Type      string     `json:"type"`                  // "content" | "tool_start" | "tool_result" | "tool_call" | "done" | "error"
	Content   string     `json:"content,omitempty"`     // type=content/tool_result 时的文本
	ToolName  string     `json:"tool_name,omitempty"`   // type=tool_start/tool_result 时的工具名
	ToolInput string     `json:"tool_input,omitempty"`  // type=tool_start 时的输入摘要
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`  // type=tool_call 时的工具调用 (OpenAI)
	Error     string     `json:"error,omitempty"`       // type=error 时的错误信息
}

// Provider AI 服务提供者接口
type Provider interface {
	// Chat 发送对话，返回流式事件 channel
	Chat(ctx context.Context, messages []Message, tools []Tool) (<-chan StreamEvent, error)
	// Name 返回 provider 名称
	Name() string
}
