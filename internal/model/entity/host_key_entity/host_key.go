package host_key_entity

// HostKey SSH 主机密钥实体
type HostKey struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Host        string `gorm:"column:host;type:varchar(255);uniqueIndex:idx_host_port" json:"host"`
	Port        int    `gorm:"column:port;uniqueIndex:idx_host_port" json:"port"`
	KeyType     string `gorm:"column:key_type;type:varchar(50);not null" json:"keyType"`
	PublicKey   string `gorm:"column:public_key;type:text;not null" json:"-"`
	Fingerprint string `gorm:"column:fingerprint;type:varchar(255);not null" json:"fingerprint"`
	FirstSeen   int64  `gorm:"column:first_seen" json:"firstSeen"`
	LastSeen    int64  `gorm:"column:last_seen" json:"lastSeen"`
}

// TableName GORM 表名
func (HostKey) TableName() string {
	return "host_keys"
}
