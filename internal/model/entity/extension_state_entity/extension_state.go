package extension_state_entity

type ExtensionState struct {
	ID         int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name       string `gorm:"column:name;uniqueIndex:idx_ext_state_name"`
	Enabled    bool   `gorm:"column:enabled;default:true"`
	Createtime int64  `gorm:"column:createtime"`
	Updatetime int64  `gorm:"column:updatetime"`
}

func (ExtensionState) TableName() string {
	return "extension_state"
}
