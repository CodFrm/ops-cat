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
	Role             Role       `json:"role"`
	Content          string     `json:"content"`
	Thinking         string     `json:"thinking,omitempty"`          // Anthropic 格式
	ReasoningContent string     `json:"reasoning_content,omitempty"` // DeepSeek/OpenAI 格式
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"` // role=tool 时标识调用
}

// ToolCall AI 发起的工具调用
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
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

// Usage 本轮 LLM 调用的 token 使用情况
// 语义统一：InputTokens 仅包含本次真正新增的输入；CacheReadTokens / CacheCreationTokens
// 分开统计缓存命中与缓存写入，便于前端展示和成本核算。
type Usage struct {
	InputTokens         int `json:"input_tokens,omitempty"`
	OutputTokens        int `json:"output_tokens,omitempty"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"` // Anthropic cache write
	CacheReadTokens     int `json:"cache_read_tokens,omitempty"`     // Anthropic cache read / OpenAI cached prompt tokens
}

// StreamEvent 流式响应事件
type StreamEvent struct {
	Type      string     `json:"type"`                 // "content" | "tool_start" | "tool_result" | "tool_call" | "approval_request" | "approval_result" | "agent_start" | "agent_end" | "done" | "error" | "thinking" | "thinking_done" | "stopped" | "retry" | "usage"
	Content   string     `json:"content,omitempty"`    // type=content/tool_result/approval_result/agent_end 时的文本
	ToolName  string     `json:"tool_name,omitempty"`  // type=tool_start/tool_result 时的工具名
	ToolInput string     `json:"tool_input,omitempty"` // type=tool_start 时的输入摘要
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // type=tool_call 时的工具调用 (OpenAI)
	ToolCallID string     `json:"tool_call_id,omitempty"` // type=tool_start/tool_result 时的工具调用 ID，前端用于跨 turn 还原 tool_calls 历史
	ConfirmID  string     `json:"confirm_id,omitempty"`   // type=approval_request/approval_result 时的确认请求 ID
	Error      string     `json:"error,omitempty"`        // type=error 时的错误信息
	AgentRole  string     `json:"agent_role,omitempty"`   // type=agent_start/approval_request 时的角色描述
	AgentTask  string     `json:"agent_task,omitempty"`   // type=agent_start 时的任务描述
	// approval_request 专用字段
	Kind        string         `json:"kind,omitempty"`        // "single" | "batch" | "grant"
	Items       []ApprovalItem `json:"items,omitempty"`       // 审批项列表
	Description string         `json:"description,omitempty"` // grant 描述
	SessionID   string         `json:"session_id,omitempty"`  // grant session ID
	// type=usage 时的 token 统计（前端累加到当前 assistant 消息）
	Usage *Usage `json:"usage,omitempty"`
}

// PermissionResponse 权限响应
type PermissionResponse struct {
	Behavior string `json:"behavior"` // "allow" | "deny"
	Message  string `json:"message"`  // deny 原因
}

// ProviderError wraps API errors with retry metadata
type ProviderError struct {
	Err        error
	RetryAfter string // from HTTP Retry-After header
	StatusCode int
}

func (e *ProviderError) Error() string {
	return e.Err.Error()
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// Provider AI 服务提供者接口
type Provider interface {
	// Chat 发送对话，返回流式事件 channel
	Chat(ctx context.Context, messages []Message, tools []Tool) (<-chan StreamEvent, error)
	// Name 返回 provider 名称
	Name() string
	// Model 返回当前使用的模型 ID（用于 agent 层根据模型类型决定是否需要回传 reasoning_content 等）
	Model() string
}
