package policy

// CommandPolicy 命令权限策略
type CommandPolicy struct {
	AllowList []string `json:"allow_list"` // 直接执行的命令规则
	DenyList  []string `json:"deny_list"`  // 始终拒绝的命令规则
}

// DefaultCommandPolicy 返回默认命令权限策略（包含高危命令拒绝列表）
func DefaultCommandPolicy() *CommandPolicy {
	return &CommandPolicy{
		DenyList: []string{
			"rm -rf / *",  // 删除根目录
			"rm -rf /* *", // 删除根目录下所有内容
			"mkfs *",      // 格式化磁盘
			"dd *",        // 磁盘级写入
			"shutdown *",  // 关机
			"reboot *",    // 重启
			"poweroff *",  // 关机
			"halt *",      // 停机
		},
	}
}

// QueryPolicy SQL 权限策略（database 类型资产使用）
type QueryPolicy struct {
	AllowTypes []string `json:"allow_types"` // 允许的语句类型: SELECT, SHOW, DESCRIBE, EXPLAIN
	DenyTypes  []string `json:"deny_types"`  // 拒绝的语句类型: DROP TABLE, TRUNCATE, ...
	DenyFlags  []string `json:"deny_flags"`  // 拒绝的特征: no_where_delete, prepare, call
}

// DefaultQueryPolicy 返回默认 SQL 权限策略
func DefaultQueryPolicy() *QueryPolicy {
	return &QueryPolicy{
		DenyTypes: []string{
			"DROP TABLE", "DROP DATABASE", "TRUNCATE",
			"GRANT", "REVOKE",
			"CREATE USER", "DROP USER", "ALTER USER",
		},
		DenyFlags: []string{
			"no_where_delete",
			"no_where_update",
			"prepare",
		},
	}
}

// RedisPolicy Redis 权限策略
type RedisPolicy struct {
	AllowList []string `json:"allow_list"` // 允许的命令模式
	DenyList  []string `json:"deny_list"`  // 拒绝的命令模式
}

// DefaultRedisPolicy 返回默认 Redis 权限策略
func DefaultRedisPolicy() *RedisPolicy {
	return &RedisPolicy{
		DenyList: []string{
			"FLUSHDB", "FLUSHALL",
			"CONFIG SET *", "CONFIG RESETSTAT",
			"DEBUG *", "SHUTDOWN *",
			"SLAVEOF *", "REPLICAOF *",
			"ACL DELUSER *", "ACL SETUSER *",
			"SCRIPT FLUSH", "CLUSTER RESET *",
		},
	}
}
