package extension_data_entity

// ExtensionData 扩展私有 KV 存储
type ExtensionData struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Extension string `gorm:"column:extension;type:varchar(100);not null;uniqueIndex:idx_ext_key"`
	Key       string `gorm:"column:key;type:varchar(255);not null;uniqueIndex:idx_ext_key"`
	Value     []byte `gorm:"column:value;type:blob"`
}

func (ExtensionData) TableName() string {
	return "extension_data"
}
