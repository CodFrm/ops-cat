package policy

import (
	"strconv"
	"strings"
)

// CommandPolicy 命令权限策略
type CommandPolicy struct {
	AllowList []string `json:"allow_list"`       // 直接执行的命令规则
	DenyList  []string `json:"deny_list"`        // 始终拒绝的命令规则
	Groups    []string `json:"groups,omitempty"` // 引用的权限组 ID
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

// ExtensionPolicy 扩展策略（扩展类型资产使用）
type ExtensionPolicy struct {
	AllowList []string `json:"allow_list"`       // 允许的 action
	DenyList  []string `json:"deny_list"`        // 拒绝的 action
	Groups    []string `json:"groups,omitempty"` // 引用的权限组 ID
}

// IsEmpty 检查策略是否为空
func (p *ExtensionPolicy) IsEmpty() bool {
	return len(p.AllowList) == 0 && len(p.DenyList) == 0 && len(p.Groups) == 0
}

// Holder 策略持有者接口，Asset 和 Group 均实现此接口
type Holder interface {
	GetCommandPolicy() (*CommandPolicy, error)
	GetQueryPolicy() (*QueryPolicy, error)
	GetRedisPolicy() (*RedisPolicy, error)
}

// DefaultRedisPolicy 返回默认 Redis 权限策略（引用内置权限组）
func DefaultRedisPolicy() *RedisPolicy {
	return &RedisPolicy{
		Groups: []string{BuiltinRedisReadOnly, BuiltinRedisDangerousDeny},
	}
}

// --- 内置权限组 ID 常量 ---

const (
	BuiltinLinuxReadOnly      = "builtin.linux_readonly"
	BuiltinK8sReadOnly        = "builtin.k8s_readonly"
	BuiltinDockerReadOnly     = "builtin.docker_readonly"
	BuiltinDangerousDeny      = "builtin.dangerous_deny"
	BuiltinSQLReadOnly        = "builtin.sql_readonly"
	BuiltinSQLDangerousDeny   = "builtin.sql_dangerous_deny"
	BuiltinRedisReadOnly      = "builtin.redis_readonly"
	BuiltinRedisDangerousDeny = "builtin.redis_dangerous_deny"
)

// IsBuiltinID 检查 ID 是否为内置权限组
func IsBuiltinID(id string) bool {
	return strings.HasPrefix(id, "builtin.")
}

// IsExtensionID 检查 ID 是否为扩展权限组
func IsExtensionID(id string) bool {
	return strings.HasPrefix(id, "ext.")
}

// IsUserID 检查 ID 是否为用户自定义权限组（数字字符串）
func IsUserID(id string) bool {
	_, err := strconv.ParseInt(id, 10, 64)
	return err == nil
}

// ParseUserID 将用户权限组字符串 ID 解析为 DB int64 ID
func ParseUserID(id string) (int64, bool) {
	n, err := strconv.ParseInt(id, 10, 64)
	return n, err == nil
}

// FormatUserID 将 DB int64 ID 格式化为字符串 ID
func FormatUserID(id int64) string {
	return strconv.FormatInt(id, 10)
}
