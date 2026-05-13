package conversation_entity

import (
	"encoding/json"
	"fmt"
)

// 状态常量
const (
	StatusActive  = 1
	StatusDeleted = 2
)

// Conversation 会话实体
type Conversation struct {
	ID           int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Title        string `gorm:"column:title;type:varchar(255)"`
	ProviderType string `gorm:"column:provider_type;type:varchar(50);not null"`
	Model        string `gorm:"column:model;type:varchar(100)"`
	ProviderID   int64  `gorm:"column:provider_id"`
	SessionData  string `gorm:"column:session_data;type:text"`
	WorkDir      string `gorm:"column:work_dir;type:varchar(500)"`
	Status       int    `gorm:"column:status;default:1"`
	Createtime   int64  `gorm:"column:createtime"`
	Updatetime   int64  `gorm:"column:updatetime"`
}

// TableName GORM表名
func (Conversation) TableName() string {
	return "conversations"
}

// SessionInfo 会话数据（JSON）
type SessionInfo struct {
	SessionID string `json:"session_id,omitempty"` // Claude CLI session ID
}

// GetSessionInfo 获取会话数据
func (c *Conversation) GetSessionInfo() (*SessionInfo, error) {
	if c.SessionData == "" {
		return &SessionInfo{}, nil
	}
	var info SessionInfo
	if err := json.Unmarshal([]byte(c.SessionData), &info); err != nil {
		return nil, fmt.Errorf("解析会话数据失败: %w", err)
	}
	return &info, nil
}

// SetSessionInfo 设置会话数据
func (c *Conversation) SetSessionInfo(info *SessionInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("序列化会话数据失败: %w", err)
	}
	c.SessionData = string(data)
	return nil
}

// IsLocalCLI 是否为本地 CLI 模式
func (c *Conversation) IsLocalCLI() bool {
	return c.ProviderType == "local_cli"
}

// Message 会话消息实体
type Message struct {
	ID             int64  `gorm:"column:id;primaryKey;autoIncrement"`
	ConversationID int64  `gorm:"column:conversation_id;index;not null"`
	Role           string `gorm:"column:role;type:varchar(20);not null"`
	Content        string `gorm:"column:content;type:text"`
	ToolCalls      string `gorm:"column:tool_calls;type:text"`
	ToolCallID     string `gorm:"column:tool_call_id;type:varchar(100)"`
	Blocks         string `gorm:"column:blocks;type:text"`
	TokenUsage     string `gorm:"column:token_usage;type:text"` // JSON: TokenUsage，仅 assistant 消息可能有
	SortOrder      int    `gorm:"column:sort_order;default:0"`
	Createtime     int64  `gorm:"column:createtime"`
}

// TableName GORM表名
func (Message) TableName() string {
	return "conversation_messages"
}

// ContentBlock 前端内容块（用于持久化显示状态）
//
// "error" 类型的 block 用于持久化对话级错误：
//   - EventError 命中时由前端推入（含分类标签 + 原始错误正文）
//   - 重试期间被关闭 tab/切会话/应用退出时，由前端 materializeRetryStatusAsError
//     把 retryStatus 物化成 kind=interrupted 的 ErrorBlock 落盘
//
// 历史回放（internal/ai/message_convert.go ToAgentMessages）必须跳过 type="error"，
// 否则错误正文会作为 assistant 历史发回给 LLM。
type ContentBlock struct {
	Type       string `json:"type"` // "text" | "tool" | "agent" | "approval" | "thinking" | "error"
	Content    string `json:"content"`
	ToolName   string `json:"toolName,omitempty"`
	ToolInput  string `json:"toolInput,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"` // 跨 turn 还原 tool_calls 历史；老数据无此字段，前端兜底为塌缩消息
	Status     string `json:"status,omitempty"`     // "running" | "completed" | "error" | "cancelled"
	// error 块字段：
	//   ErrorKind   — "rate_limit" | "server" | "network" | "auth" | "interrupted" | "unknown"
	//   ErrorDetail — 原始错误正文，UI 直接展示
	ErrorKind   string `json:"errorKind,omitempty"`
	ErrorDetail string `json:"errorDetail,omitempty"`
}

// GetBlocks 获取前端显示块
func (m *Message) GetBlocks() ([]ContentBlock, error) {
	if m.Blocks == "" {
		return nil, nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal([]byte(m.Blocks), &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

// SetBlocks 设置前端显示块
func (m *Message) SetBlocks(blocks []ContentBlock) error {
	if len(blocks) == 0 {
		m.Blocks = ""
		return nil
	}
	data, err := json.Marshal(blocks)
	if err != nil {
		return err
	}
	m.Blocks = string(data)
	return nil
}

// TokenUsage 一条 assistant 消息累计消耗的 token 数
type TokenUsage struct {
	InputTokens         int `json:"inputTokens,omitempty"`
	OutputTokens        int `json:"outputTokens,omitempty"`
	CacheCreationTokens int `json:"cacheCreationTokens,omitempty"`
	CacheReadTokens     int `json:"cacheReadTokens,omitempty"`
}

// GetTokenUsage 反序列化 token_usage 字段
func (m *Message) GetTokenUsage() (*TokenUsage, error) {
	if m.TokenUsage == "" {
		return nil, nil
	}
	var u TokenUsage
	if err := json.Unmarshal([]byte(m.TokenUsage), &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// SetTokenUsage 序列化 token_usage 字段；nil 或全零值视为清空
func (m *Message) SetTokenUsage(u *TokenUsage) error {
	if u == nil || (u.InputTokens == 0 && u.OutputTokens == 0 && u.CacheCreationTokens == 0 && u.CacheReadTokens == 0) {
		m.TokenUsage = ""
		return nil
	}
	data, err := json.Marshal(u)
	if err != nil {
		return err
	}
	m.TokenUsage = string(data)
	return nil
}
