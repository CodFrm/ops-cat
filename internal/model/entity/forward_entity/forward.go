package forward_entity

import "errors"

// ForwardConfig 转发配置（一组转发规则）
type ForwardConfig struct {
	ID         int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name       string `gorm:"column:name;type:varchar(255);not null" json:"name"`
	AssetID    int64  `gorm:"column:asset_id;not null" json:"assetId"`
	Createtime int64  `gorm:"column:createtime" json:"createtime"`
	Updatetime int64  `gorm:"column:updatetime" json:"updatetime"`
}

func (ForwardConfig) TableName() string {
	return "forward_configs"
}

func (c *ForwardConfig) Validate() error {
	if c.Name == "" {
		return errors.New("名称不能为空")
	}
	if c.AssetID <= 0 {
		return errors.New("必须选择资产")
	}
	return nil
}

// ForwardRule 转发规则
type ForwardRule struct {
	ID         int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ConfigID   int64  `gorm:"column:config_id;not null;index" json:"configId"`
	Type       string `gorm:"column:type;type:varchar(20);not null" json:"type"` // "local" | "remote"
	LocalHost  string `gorm:"column:local_host;type:varchar(255);not null" json:"localHost"`
	LocalPort  int    `gorm:"column:local_port;not null" json:"localPort"`
	RemoteHost string `gorm:"column:remote_host;type:varchar(255);not null" json:"remoteHost"`
	RemotePort int    `gorm:"column:remote_port;not null" json:"remotePort"`
}

func (ForwardRule) TableName() string {
	return "forward_rules"
}
