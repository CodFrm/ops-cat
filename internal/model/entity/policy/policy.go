package policy

// CommandPolicy 命令权限策略
type CommandPolicy struct {
	AllowList []string `json:"allow_list"`       // 直接执行的命令规则
	DenyList  []string `json:"deny_list"`        // 始终拒绝的命令规则
	Groups    []string `json:"groups,omitempty"` // 引用的权限组 ID（内置组: "builtin:xxx", 用户组: "123"）
}

// IsEmpty 检查策略是否为空（无规则且无引用组）
func (p *CommandPolicy) IsEmpty() bool {
	return len(p.AllowList) == 0 && len(p.DenyList) == 0 && len(p.Groups) == 0
}

// DefaultCommandPolicy 返回默认命令权限策略（引用内置权限组）
func DefaultCommandPolicy() *CommandPolicy {
	return &CommandPolicy{
		Groups: []string{BuiltinLinuxReadOnly, BuiltinDangerousDeny},
	}
}

// QueryPolicy SQL 权限策略（database 类型资产使用）
type QueryPolicy struct {
	AllowTypes []string `json:"allow_types"`      // 允许的语句类型: SELECT, SHOW, DESCRIBE, EXPLAIN
	DenyTypes  []string `json:"deny_types"`       // 拒绝的语句类型: DROP TABLE, TRUNCATE, ...
	DenyFlags  []string `json:"deny_flags"`       // 拒绝的特征: no_where_delete, prepare, call
	Groups     []string `json:"groups,omitempty"` // 引用的权限组 ID
}

// IsEmpty 检查策略是否为空
func (p *QueryPolicy) IsEmpty() bool {
	return len(p.AllowTypes) == 0 && len(p.DenyTypes) == 0 && len(p.DenyFlags) == 0 && len(p.Groups) == 0
}

// DefaultQueryPolicy 返回默认 SQL 权限策略（引用内置权限组）
func DefaultQueryPolicy() *QueryPolicy {
	return &QueryPolicy{
		Groups: []string{BuiltinSQLReadOnly, BuiltinSQLDangerousDeny},
	}
}

// RedisPolicy Redis 权限策略
type RedisPolicy struct {
	AllowList []string `json:"allow_list"`       // 允许的命令模式
	DenyList  []string `json:"deny_list"`        // 拒绝的命令模式
	Groups    []string `json:"groups,omitempty"` // 引用的权限组 ID
}

// IsEmpty 检查策略是否为空
func (p *RedisPolicy) IsEmpty() bool {
	return len(p.AllowList) == 0 && len(p.DenyList) == 0 && len(p.Groups) == 0
}

// MongoPolicy MongoDB 权限策略
type MongoPolicy struct {
	AllowTypes []string `json:"allow_types,omitempty"` // 允许的操作类型: find, findOne, aggregate, ...
	DenyTypes  []string `json:"deny_types,omitempty"`  // 拒绝的操作类型: dropDatabase, dropCollection, ...
	Groups     []string `json:"groups,omitempty"`      // 引用的权限组 ID
}

// IsEmpty 检查策略是否为空
func (p *MongoPolicy) IsEmpty() bool {
	return len(p.AllowTypes) == 0 && len(p.DenyTypes) == 0 && len(p.Groups) == 0
}

// DefaultMongoPolicy 返回默认 MongoDB 权限策略（引用内置权限组）
func DefaultMongoPolicy() *MongoPolicy {
	return &MongoPolicy{Groups: []string{BuiltinMongoReadOnly}}
}

// Holder 策略持有者接口，Asset 和 Group 均实现此接口
type Holder interface {
	GetCommandPolicy() (*CommandPolicy, error)
	GetQueryPolicy() (*QueryPolicy, error)
	GetRedisPolicy() (*RedisPolicy, error)
	GetMongoPolicy() (*MongoPolicy, error)
}

// DefaultRedisPolicy 返回默认 Redis 权限策略（引用内置权限组）
func DefaultRedisPolicy() *RedisPolicy {
	return &RedisPolicy{
		Groups: []string{BuiltinRedisReadOnly, BuiltinRedisDangerousDeny},
	}
}

// --- 内置权限组 ID 常量 ---

const (
	BuiltinLinuxReadOnly      = "builtin:linux-readonly"
	BuiltinK8sReadOnly        = "builtin:k8s-readonly"
	BuiltinDockerReadOnly     = "builtin:docker-readonly"
	BuiltinDangerousDeny      = "builtin:dangerous-deny"
	BuiltinSQLReadOnly        = "builtin:sql-readonly"
	BuiltinSQLDangerousDeny   = "builtin:sql-dangerous-deny"
	BuiltinRedisReadOnly      = "builtin:redis-readonly"
	BuiltinRedisDangerousDeny = "builtin:redis-dangerous-deny"
	BuiltinMongoReadOnly      = "builtin:mongo-readonly"
	BuiltinMongoReadWrite     = "builtin:mongo-readwrite"
	BuiltinMongoDangerousDeny = "builtin:mongo-dangerous-deny"

	// BuiltinPrefix 内置权限组 ID 前缀
	BuiltinPrefix = "builtin:"
)
