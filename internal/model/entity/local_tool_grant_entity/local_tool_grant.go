package local_tool_grant_entity

// LocalToolGrant 记录某条会话对 cago built-in 本地工具（write / edit）的"始终放行"。
// bash 不在表中——bash 的策略是每次都要确认，绝不写入这里。
//
// 与 grant_entity.GrantSession/GrantItem 的区别：那一组是资产维度的命令级 grant；
// 这一组是工具维度，没有资产/命令通配，仅用 (session_id, tool_name) 命中即放行。
type LocalToolGrant struct {
	ID         int64  `gorm:"column:id;primaryKey;autoIncrement"`
	SessionID  string `gorm:"column:session_id;type:varchar(64);not null;uniqueIndex:uq_local_tool_grant_session_tool,priority:1"`
	ToolName   string `gorm:"column:tool_name;type:varchar(64);not null;uniqueIndex:uq_local_tool_grant_session_tool,priority:2"`
	Createtime int64  `gorm:"column:createtime;not null"`
}

// TableName GORM 表名。
func (LocalToolGrant) TableName() string {
	return "ai_local_tool_grants"
}
