package plan_entity

// 计划状态
const (
	PlanStatusPending  = 1
	PlanStatusApproved = 2
	PlanStatusRejected = 3
)

// PlanSession 批量审批计划
type PlanSession struct {
	ID          string `gorm:"column:id;primaryKey;type:varchar(36)"` // UUIDv4
	Description string `gorm:"column:description;type:text"`
	Status      int    `gorm:"column:status;not null;default:1"`
	Createtime  int64  `gorm:"column:createtime;not null"`
	Updatetime  int64  `gorm:"column:updatetime"`
}

// TableName GORM 表名
func (PlanSession) TableName() string {
	return "plan_sessions"
}

// PlanItem 计划中的单条操作
type PlanItem struct {
	ID            int64  `gorm:"column:id;primaryKey;autoIncrement"`
	PlanSessionID string `gorm:"column:plan_session_id;type:varchar(36);index;not null"`
	ItemIndex     int    `gorm:"column:item_index;not null"`
	ToolName      string `gorm:"column:tool_name;type:varchar(100);not null"` // "exec", "cp", "create", "update"
	AssetID       int64  `gorm:"column:asset_id;default:0"`
	AssetName     string `gorm:"column:asset_name;type:varchar(255)"`
	GroupID       int64  `gorm:"column:group_id;default:0"`
	GroupName     string `gorm:"column:group_name;type:varchar(255)"`
	Command       string `gorm:"column:command;type:text"`
	Detail        string `gorm:"column:detail;type:text"`
	Consumed      int    `gorm:"column:consumed;default:0"` // 0=未消费, 1=已消费
	AuditLogID    int64  `gorm:"column:audit_log_id;default:0"`
	ConsumedAt    int64  `gorm:"column:consumed_at;default:0"`
}

// TableName GORM 表名
func (PlanItem) TableName() string {
	return "plan_items"
}
