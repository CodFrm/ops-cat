package ai

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
	Type       string `json:"type"`                   // "content" | "tool_start" | "tool_result" | "approval_request" | "approval_result" | "agent_start" | "agent_end" | "queue_consumed" | "done" | "error" | "thinking" | "thinking_done" | "stopped" | "retry" | "usage" | "compacted"
	// type=retry 时 Content 携带 attempt 序号（字符串形式，从 1 开始），RetryDelayMs 是下一次重试前的等待毫秒（用于前端倒计时同步）。
	Content    string `json:"content,omitempty"`      // type=content/tool_result/approval_result/agent_end 时的文本
	QueueID    string `json:"queue_id,omitempty"`     // type=queue_consumed 时的前端队列项 ID
	ToolName   string `json:"tool_name,omitempty"`    // type=tool_start/tool_result 时的工具名
	ToolInput  string `json:"tool_input,omitempty"`   // type=tool_start 时的输入摘要
	ToolCallID string `json:"tool_call_id,omitempty"` // type=tool_start/tool_result 时的工具调用 ID，前端用于跨 turn 还原 tool_calls 历史
	ConfirmID  string `json:"confirm_id,omitempty"`   // type=approval_request/approval_result 时的确认请求 ID
	Error      string `json:"error,omitempty"`        // type=error 时的错误信息
	AgentRole  string `json:"agent_role,omitempty"`   // type=agent_start/approval_request 时的角色描述
	AgentTask  string `json:"agent_task,omitempty"`   // type=agent_start 时的任务描述
	// approval_request 专用字段
	Kind        string         `json:"kind,omitempty"`        // "single" | "batch" | "grant" | "local_tool"
	Items       []ApprovalItem `json:"items,omitempty"`       // 审批项列表
	Description string         `json:"description,omitempty"` // grant 描述
	SessionID   string         `json:"session_id,omitempty"`  // grant session ID
	Patterns    []string       `json:"patterns,omitempty"`    // local_tool 默认 pattern 列表（与 sub-commands 对齐），前端预填可编辑
	// type=usage 时的 token 统计（前端累加到当前 assistant 消息）
	Usage *Usage `json:"usage,omitempty"`
	// type=retry 时的等待毫秒；前端据此显示倒计时。0 表示无显式退避（立即重试）。
	RetryDelayMs int `json:"retryDelayMs,omitempty"`
}

// PermissionResponse 权限响应
type PermissionResponse struct {
	Behavior string `json:"behavior"` // "allow" | "deny"
	Message  string `json:"message"`  // deny 原因
}
