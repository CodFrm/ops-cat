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
	// cago State 平铺字段（202605080010 迁移之后写入；gormStore 单源）
	ThreadID    string `gorm:"column:thread_id;type:varchar(255)"`
	StateValues string `gorm:"column:state_values;type:text"`
}

// TableName GORM表名
func (Conversation) TableName() string {
	return "conversations"
}

// GetStateValues 反序列化 cago Session State.Values。
func (c *Conversation) GetStateValues() (map[string]string, error) {
	if c.StateValues == "" {
		return nil, nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(c.StateValues), &m); err != nil {
		return nil, fmt.Errorf("解析 state_values 失败: %w", err)
	}
	return m, nil
}

// SetStateValues 序列化 cago Session State.Values；nil/空 map 视为清空。
func (c *Conversation) SetStateValues(v map[string]string) error {
	if len(v) == 0 {
		c.StateValues = ""
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("序列化 state_values 失败: %w", err)
	}
	c.StateValues = string(data)
	return nil
}

// Message 会话消息实体（cago agents v2 schema）
type Message struct {
	ID             int64  `gorm:"column:id;primaryKey;autoIncrement"`
	ConversationID int64  `gorm:"column:conversation_id;not null;uniqueIndex:idx_conv_msg_unique,priority:1"`
	Role           string `gorm:"column:role;type:varchar(20);not null"`
	Blocks         string `gorm:"column:blocks;type:text"`      // []ContentBlock JSON（含 MetadataBlock）
	Mentions       string `gorm:"column:mentions;type:text"`    // []ai.MentionedAsset JSON
	TokenUsage     string `gorm:"column:token_usage;type:text"` // *agent.Usage JSON，仅 assistant 可能有
	PartialReason  string `gorm:"column:partial_reason;type:varchar(16);not null;default:''"`
	SortOrder      int    `gorm:"column:sort_order;not null;default:0;uniqueIndex:idx_conv_msg_unique,priority:2"`
	Createtime     int64  `gorm:"column:createtime"`
}

// TableName GORM表名
func (Message) TableName() string {
	return "conversation_messages"
}

// ContentBlock 前端内容块（用于持久化显示状态）
type ContentBlock struct {
	Type       string `json:"type"` // "text" | "tool"
	Content    string `json:"content"`
	ToolName   string `json:"toolName,omitempty"`
	ToolInput  string `json:"toolInput,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"` // 跨 turn 还原 tool_calls 历史；老数据无此字段，前端兜底为塌缩消息
	Status     string `json:"status,omitempty"`     // "running" | "completed" | "error"
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

// MentionRef 用户消息中引用的资产（@ 提及）
type MentionRef struct {
	AssetID int64  `json:"assetId"`
	Name    string `json:"name"`  // 发送时刻的资产名快照
	Start   int    `json:"start"` // content 中字符起始索引（含 @ 符号）
	End     int    `json:"end"`   // 结束索引（不含）
}

// GetMentions 反序列化 mentions 字段
func (m *Message) GetMentions() ([]MentionRef, error) {
	if m.Mentions == "" {
		return nil, nil
	}
	var refs []MentionRef
	if err := json.Unmarshal([]byte(m.Mentions), &refs); err != nil {
		return nil, err
	}
	return refs, nil
}

// SetMentions 序列化 mentions 字段
func (m *Message) SetMentions(refs []MentionRef) error {
	if len(refs) == 0 {
		m.Mentions = ""
		return nil
	}
	data, err := json.Marshal(refs)
	if err != nil {
		return err
	}
	m.Mentions = string(data)
	return nil
}
