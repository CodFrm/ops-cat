package extension_data_entity

// ExtensionData 扩展数据存储实体
type ExtensionData struct {
	ID            int64  `gorm:"column:id;primaryKey;autoIncrement"`
	ExtensionName string `gorm:"column:extension_name;uniqueIndex:idx_ext_key"`
	Key           string `gorm:"column:key;uniqueIndex:idx_ext_key"`
	Value         []byte `gorm:"column:value;type:blob"`
	Updatetime    int64  `gorm:"column:updatetime"`
}

func (ExtensionData) TableName() string {
	return "extension_data"
}
