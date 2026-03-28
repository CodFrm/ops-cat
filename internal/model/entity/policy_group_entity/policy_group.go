package policy_group_entity

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/opskat/opskat/internal/model/entity/policy"
)

// 策略类型常量
const (
	PolicyTypeCommand = "command"
	PolicyTypeQuery   = "query"
	PolicyTypeRedis   = "redis"
)

// 权限组来源常量
const (
	SourceBuiltin   = "builtin"
	SourceExtension = "extension"
	SourceUser      = "user"
)

// PolicyGroup 权限组实体
type PolicyGroup struct {
	ID            int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	StringID      string `gorm:"-" json:"-"` // 非 DB: 用于内存中的 builtin/extension 组
	Name          string `gorm:"column:name;type:varchar(255);not null" json:"name"`
	NameZh        string `gorm:"-" json:"-"` // 非 DB: 扩展组国际化
	Description   string `gorm:"column:description;type:text" json:"description"`
	DescriptionZh string `gorm:"-" json:"-"` // 非 DB: 扩展组国际化
	PolicyType    string `gorm:"column:policy_type;type:varchar(50);not null" json:"policyType"`
	Policy        string `gorm:"column:policy;type:text;not null" json:"policy"`
	Createtime    int64  `gorm:"column:createtime" json:"createtime"`
	Updatetime    int64  `gorm:"column:updatetime" json:"updatetime"`
}

// TableName GORM 表名
func (PolicyGroup) TableName() string {
	return "policy_groups"
}

// GetStringID 返回字符串 ID：builtin/extension 使用 StringID，用户组使用数字字符串
func (pg *PolicyGroup) GetStringID() string {
	if pg.StringID != "" {
		return pg.StringID
	}
	return policy.FormatUserID(pg.ID)
}

// Validate 校验
func (pg *PolicyGroup) Validate() error {
	if pg.Name == "" {
		return errors.New("权限组名称不能为空")
	}
	if !IsValidPolicyType(pg.PolicyType) {
		return errors.New("无效的策略类型")
	}
	return nil
}

// PolicyGroupItem 返回给前端的权限组项
type PolicyGroupItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	NameZh        string `json:"name_zh,omitempty"`
	Description   string `json:"description"`
	DescriptionZh string `json:"description_zh,omitempty"`
	PolicyType    string `json:"policyType"`
	Policy        string `json:"policy"`
	Source        string `json:"source"` // "builtin", "extension", "user"
	Createtime    int64  `json:"createtime"`
	Updatetime    int64  `json:"updatetime"`
}

// ToItem 转为 PolicyGroupItem
func (pg *PolicyGroup) ToItem(source string) *PolicyGroupItem {
	return &PolicyGroupItem{
		ID:            pg.GetStringID(),
		Name:          pg.Name,
		NameZh:        pg.NameZh,
		Description:   pg.Description,
		DescriptionZh: pg.DescriptionZh,
		PolicyType:    pg.PolicyType,
		Policy:        pg.Policy,
		Source:        source,
		Createtime:    pg.Createtime,
		Updatetime:    pg.Updatetime,
	}
}

// --- 内置权限组 ---

func mustMarshal(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// BuiltinGroups 返回所有内置权限组
func BuiltinGroups() []*PolicyGroup {
	return []*PolicyGroup{
		// SSH command 类型
		{
			StringID:    policy.BuiltinLinuxReadOnly,
			Name:        "Linux 常用只读",
			Description: "常用 Linux 只读命令",
			PolicyType:  PolicyTypeCommand,
			Policy: mustMarshal(&policy.CommandPolicy{
				AllowList: []string{
					"ls *", "cat *", "head *", "tail *",
					"grep *", "find *", "pwd", "wc *",
					"whoami", "hostname", "uname *", "id", "date",
					"env", "printenv *", "which *", "file *", "stat *",
					"df *", "du *", "free *", "uptime",
					"ps *", "top -b -n 1 *",
					"netstat *", "ss *", "ip *", "ifconfig *",
					"mount", "lsblk *", "blkid *",
					"lsof *", "vmstat *", "iostat *",
					"systemctl status *", "journalctl *",
				},
			}),
		},
		{
			StringID:    policy.BuiltinK8sReadOnly,
			Name:        "Kubernetes 只读",
			Description: "Kubernetes 只读操作命令",
			PolicyType:  PolicyTypeCommand,
			Policy: mustMarshal(&policy.CommandPolicy{
				AllowList: []string{
					"kubectl get *", "kubectl describe *", "kubectl logs *",
					"kubectl top *", "kubectl explain *",
					"kubectl api-resources *", "kubectl api-versions",
					"kubectl cluster-info *", "kubectl config view *",
					"kubectl config get-contexts *", "kubectl version *",
					"kubectl auth can-i *",
				},
			}),
		},
		{
			StringID:    policy.BuiltinDockerReadOnly,
			Name:        "Docker 只读",
			Description: "Docker 只读操作命令",
			PolicyType:  PolicyTypeCommand,
			Policy: mustMarshal(&policy.CommandPolicy{
				AllowList: []string{
					"docker ps *", "docker images *", "docker logs *",
					"docker inspect *", "docker stats *", "docker top *",
					"docker port *", "docker diff *", "docker history *",
					"docker info", "docker version",
					"docker network ls *", "docker network inspect *",
					"docker volume ls *", "docker volume inspect *",
					"docker compose ps *", "docker compose logs *",
				},
			}),
		},
		{
			StringID:    policy.BuiltinDangerousDeny,
			Name:        "高危命令拒绝",
			Description: "拒绝执行高危系统命令",
			PolicyType:  PolicyTypeCommand,
			Policy: mustMarshal(&policy.CommandPolicy{
				DenyList: []string{
					"rm -rf /*",
					"mkfs *",
					"dd *",
					"shutdown *",
					"reboot *",
					"poweroff *",
					"halt *",
				},
			}),
		},
		// Database query 类型
		{
			StringID:    policy.BuiltinSQLReadOnly,
			Name:        "SQL 只读",
			Description: "只允许查询类 SQL 语句",
			PolicyType:  PolicyTypeQuery,
			Policy: mustMarshal(&policy.QueryPolicy{
				AllowTypes: []string{
					"SELECT", "SHOW", "DESCRIBE", "EXPLAIN", "USE",
				},
			}),
		},
		{
			StringID:    policy.BuiltinSQLDangerousDeny,
			Name:        "SQL 高危拒绝",
			Description: "拒绝高危 SQL 操作",
			PolicyType:  PolicyTypeQuery,
			Policy: mustMarshal(&policy.QueryPolicy{
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
			}),
		},
		// Redis 类型
		{
			StringID:    policy.BuiltinRedisReadOnly,
			Name:        "Redis 只读",
			Description: "只允许 Redis 只读命令",
			PolicyType:  PolicyTypeRedis,
			Policy: mustMarshal(&policy.RedisPolicy{
				AllowList: []string{
					"GET", "MGET", "STRLEN",
					"HGET", "HGETALL", "HKEYS", "HVALS", "HLEN", "HMGET", "HEXISTS",
					"LRANGE", "LLEN", "LINDEX",
					"SMEMBERS", "SCARD", "SISMEMBER",
					"ZRANGE", "ZCARD", "ZSCORE", "ZRANK", "ZCOUNT",
					"TYPE", "TTL", "PTTL", "EXISTS", "DBSIZE", "KEYS", "SCAN",
					"INFO", "PING",
				},
			}),
		},
		{
			StringID:    policy.BuiltinRedisDangerousDeny,
			Name:        "Redis 高危拒绝",
			Description: "拒绝 Redis 高危命令",
			PolicyType:  PolicyTypeRedis,
			Policy: mustMarshal(&policy.RedisPolicy{
				DenyList: []string{
					"FLUSHDB", "FLUSHALL",
					"CONFIG SET *", "CONFIG RESETSTAT",
					"DEBUG *", "SHUTDOWN *",
					"SLAVEOF *", "REPLICAOF *",
					"ACL DELUSER *", "ACL SETUSER *",
					"SCRIPT FLUSH", "CLUSTER RESET *",
				},
			}),
		},
	}
}

// builtinMap 内置组缓存
var builtinMap map[string]*PolicyGroup

func init() {
	builtinMap = make(map[string]*PolicyGroup)
	for _, pg := range BuiltinGroups() {
		builtinMap[pg.StringID] = pg
	}
}

// FindBuiltin 按字符串 ID 查找内置权限组
func FindBuiltin(id string) *PolicyGroup {
	return builtinMap[id]
}

// --- 扩展权限组 ---

var (
	extensionGroupsMu sync.RWMutex
	extensionGroups   = make(map[string]*PolicyGroup)
)

// RegisterExtensionGroups 注册扩展权限组（扩展加载时调用）
func RegisterExtensionGroups(extName string, groups []*PolicyGroup) {
	extensionGroupsMu.Lock()
	defer extensionGroupsMu.Unlock()
	for _, g := range groups {
		extensionGroups[g.StringID] = g
	}
}

// UnregisterExtensionGroups 注销指定扩展的所有权限组
func UnregisterExtensionGroups(extName string) {
	extensionGroupsMu.Lock()
	defer extensionGroupsMu.Unlock()
	prefix := "ext." + extName + "."
	for k := range extensionGroups {
		if strings.HasPrefix(k, prefix) {
			delete(extensionGroups, k)
		}
	}
}

// FindExtensionGroup 按字符串 ID 查找扩展权限组
func FindExtensionGroup(id string) *PolicyGroup {
	extensionGroupsMu.RLock()
	defer extensionGroupsMu.RUnlock()
	return extensionGroups[id]
}

// ExtensionGroups 返回所有已注册的扩展权限组
func ExtensionGroups() []*PolicyGroup {
	extensionGroupsMu.RLock()
	defer extensionGroupsMu.RUnlock()
	result := make([]*PolicyGroup, 0, len(extensionGroups))
	for _, pg := range extensionGroups {
		result = append(result, pg)
	}
	return result
}

// ExtensionGroupsByType 返回指定扩展名的所有权限组字符串 ID
func ExtensionGroupsByType(extName string) []string {
	extensionGroupsMu.RLock()
	defer extensionGroupsMu.RUnlock()
	prefix := "ext." + extName + "."
	var ids []string
	for k := range extensionGroups {
		if strings.HasPrefix(k, prefix) {
			ids = append(ids, k)
		}
	}
	return ids
}

// --- 扩展策略类型注册 ---

var (
	extPolicyTypesMu sync.RWMutex
	extPolicyTypes   = make(map[string]struct{})
)

// RegisterExtensionPolicyType 注册扩展策略类型
func RegisterExtensionPolicyType(policyType string) {
	extPolicyTypesMu.Lock()
	defer extPolicyTypesMu.Unlock()
	extPolicyTypes[policyType] = struct{}{}
}

// UnregisterExtensionPolicyType 注销扩展策略类型
func UnregisterExtensionPolicyType(policyType string) {
	extPolicyTypesMu.Lock()
	defer extPolicyTypesMu.Unlock()
	delete(extPolicyTypes, policyType)
}

// IsExtensionPolicyType 检查是否为扩展策略类型
func IsExtensionPolicyType(policyType string) bool {
	extPolicyTypesMu.RLock()
	defer extPolicyTypesMu.RUnlock()
	_, ok := extPolicyTypes[policyType]
	return ok
}

// IsValidPolicyType 检查策略类型是否有效（内置 + 扩展）
func IsValidPolicyType(policyType string) bool {
	switch policyType {
	case PolicyTypeCommand, PolicyTypeQuery, PolicyTypeRedis:
		return true
	default:
		return IsExtensionPolicyType(policyType)
	}
}

// ParseGroupID 解析权限组 ID，返回 DB int64 ID（仅用户自定义组有效）
func ParseGroupID(id string) (int64, bool) {
	return policy.ParseUserID(id)
}
