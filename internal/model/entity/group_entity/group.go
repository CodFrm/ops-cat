package group_entity

import (
	"errors"

	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/pkg/jsonfield"
)

// Group 资产分组实体
type Group struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string `gorm:"column:name;type:varchar(255);not null"`
	ParentID    int64  `gorm:"column:parent_id;index"`
	Icon        string `gorm:"column:icon;type:varchar(100)"`
	Description string `gorm:"column:description;type:text"`
	CmdPolicy   string `gorm:"column:command_policy;type:text"`
	QryPolicy   string `gorm:"column:query_policy;type:text"`
	RdsPolicy   string `gorm:"column:redis_policy;type:text"`
	MgoPolicy   string `gorm:"column:mongo_policy;type:text"`
	SortOrder   int    `gorm:"column:sort_order;default:0"`
	Createtime  int64  `gorm:"column:createtime"`
	Updatetime  int64  `gorm:"column:updatetime"`
}

// TableName GORM表名
func (Group) TableName() string {
	return "groups"
}

// Validate 校验分组
func (g *Group) Validate() error {
	if g.Name == "" {
		return errors.New("分组名称不能为空")
	}
	return nil
}

// IsRoot 是否为顶层分组
func (g *Group) IsRoot() bool {
	return g.ParentID == 0
}

// GetCommandPolicy 解析命令权限策略
func (g *Group) GetCommandPolicy() (*policy.CommandPolicy, error) {
	return jsonfield.UnmarshalOrDefault[policy.CommandPolicy](g.CmdPolicy, "命令权限策略")
}

// SetCommandPolicy 序列化命令权限策略
func (g *Group) SetCommandPolicy(p *policy.CommandPolicy) error {
	s, err := jsonfield.MarshalOrClear(p, func(v *policy.CommandPolicy) bool {
		return v.IsEmpty()
	}, "命令权限策略")
	if err != nil {
		return err
	}
	g.CmdPolicy = s
	return nil
}

// GetQueryPolicy 解析 SQL 权限策略
func (g *Group) GetQueryPolicy() (*policy.QueryPolicy, error) {
	return jsonfield.UnmarshalOrDefault[policy.QueryPolicy](g.QryPolicy, "SQL权限策略")
}

// SetQueryPolicy 序列化 SQL 权限策略
func (g *Group) SetQueryPolicy(p *policy.QueryPolicy) error {
	s, err := jsonfield.MarshalOrClear(p, func(v *policy.QueryPolicy) bool {
		return v.IsEmpty()
	}, "SQL权限策略")
	if err != nil {
		return err
	}
	g.QryPolicy = s
	return nil
}

// GetRedisPolicy 解析 Redis 权限策略
func (g *Group) GetRedisPolicy() (*policy.RedisPolicy, error) {
	return jsonfield.UnmarshalOrDefault[policy.RedisPolicy](g.RdsPolicy, "Redis权限策略")
}

// SetRedisPolicy 序列化 Redis 权限策略
func (g *Group) SetRedisPolicy(p *policy.RedisPolicy) error {
	s, err := jsonfield.MarshalOrClear(p, func(v *policy.RedisPolicy) bool {
		return v.IsEmpty()
	}, "Redis权限策略")
	if err != nil {
		return err
	}
	g.RdsPolicy = s
	return nil
}

// GetMongoPolicy 解析 MongoDB 权限策略
func (g *Group) GetMongoPolicy() (*policy.MongoPolicy, error) {
	return jsonfield.UnmarshalOrDefault[policy.MongoPolicy](g.MgoPolicy, "MongoDB权限策略")
}

// SetMongoPolicy 序列化 MongoDB 权限策略
func (g *Group) SetMongoPolicy(p *policy.MongoPolicy) error {
	s, err := jsonfield.MarshalOrClear(p, func(v *policy.MongoPolicy) bool {
		return v.IsEmpty()
	}, "MongoDB权限策略")
	if err != nil {
		return err
	}
	g.MgoPolicy = s
	return nil
}
