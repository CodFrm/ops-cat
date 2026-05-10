package audit_entity

// AuditLog 审计日志实体
type AuditLog struct {
	ID             int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Source         string `gorm:"column:source;type:varchar(20);not null;index"` // "ai" | "opsctl" | "desktop"
	ToolName       string `gorm:"column:tool_name;type:varchar(100);not null"`
	AssetID        int64  `gorm:"column:asset_id;default:0;index"`
	AssetName      string `gorm:"column:asset_name;type:varchar(255)"`
	Command        string `gorm:"column:command;type:text"`
	Request        string `gorm:"column:request;type:text"`
	Result         string `gorm:"column:result;type:text"`
	Error          string `gorm:"column:error;type:text"`
	Success        int    `gorm:"column:success;default:1"` // 1=成功, 0=失败
	ConversationID int64  `gorm:"column:conversation_id;default:0;index"`
	GrantSessionID string `gorm:"column:grant_session_id;type:varchar(36)"`
	SessionID      string `gorm:"column:session_id;type:varchar(64);index"` // opsctl/AI 会话 ID
	Decision       string `gorm:"column:decision;type:varchar(10)"`         // "allow" | "deny"
	DecisionSource string `gorm:"column:decision_source;type:varchar(30)"`  // 决策来源
	MatchedPattern string `gorm:"column:matched_pattern;type:varchar(500)"` // 匹配的命令模式
	Createtime     int64  `gorm:"column:createtime;not null;index"`
}

// TableName GORM 表名
func (AuditLog) TableName() string {
	return "audit_logs"
}
