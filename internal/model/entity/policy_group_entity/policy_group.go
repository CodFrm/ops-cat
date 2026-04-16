package policy_group_entity

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/opskat/opskat/internal/model/entity/policy"
)

// 策略类型常量
const (
	PolicyTypeCommand = "command"
	PolicyTypeQuery   = "query"
	PolicyTypeRedis   = "redis"
	PolicyTypeMongo   = "mongo"
)

// PolicyGroup 权限组实体（数据库）
type PolicyGroup struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name        string `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Description string `gorm:"column:description;type:text" json:"description"`
	PolicyType  string `gorm:"column:policy_type;type:varchar(50);not null" json:"policyType"`
	Policy      string `gorm:"column:policy;type:text;not null" json:"policy"`
	Createtime  int64  `gorm:"column:createtime" json:"createtime"`
	Updatetime  int64  `gorm:"column:updatetime" json:"updatetime"`

	// 非数据库字段，仅内置组/扩展组使用
	BuiltinID     string `gorm:"-" json:"-"`
	ExtensionName string `gorm:"-" json:"-"` // 扩展名称（如 "oss"）
}

// TableName GORM 表名
func (PolicyGroup) TableName() string {
	return "policy_groups"
}

// Validate 校验
func (pg *PolicyGroup) Validate() error {
	if pg.Name == "" {
		return errors.New("权限组名称不能为空")
	}
	switch pg.PolicyType {
	case PolicyTypeCommand, PolicyTypeQuery, PolicyTypeRedis, PolicyTypeMongo:
	default:
		if !hasExtensionPolicyType(pg.PolicyType) {
			return errors.New("无效的策略类型")
		}
	}
	return nil
}

// PolicyGroupItem 返回给前端的权限组项
type PolicyGroupItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	PolicyType    string `json:"policyType"`
	Policy        string `json:"policy"`
	Builtin       bool   `json:"builtin"`
	ExtensionName string `json:"extensionName,omitempty"`
	Createtime    int64  `json:"createtime"`
	Updatetime    int64  `json:"updatetime"`
}

// ToItem 转为 PolicyGroupItem
func (pg *PolicyGroup) ToItem() *PolicyGroupItem {
	item := &PolicyGroupItem{
		Name:          pg.Name,
		Description:   pg.Description,
		PolicyType:    pg.PolicyType,
		Policy:        pg.Policy,
		ExtensionName: pg.ExtensionName,
		Createtime:    pg.Createtime,
		Updatetime:    pg.Updatetime,
	}
	if pg.BuiltinID != "" {
		item.ID = pg.BuiltinID
		item.Builtin = true
	} else {
		item.ID = strconv.FormatInt(pg.ID, 10)
	}
	return item
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
			BuiltinID:   policy.BuiltinLinuxReadOnly,
			Name:        "Linux Read-Only",
			Description: "Common Linux read-only commands",
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
			BuiltinID:   policy.BuiltinK8sReadOnly,
			Name:        "Kubernetes Read-Only",
			Description: "Kubernetes read-only commands",
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
			BuiltinID:   policy.BuiltinDockerReadOnly,
			Name:        "Docker Read-Only",
			Description: "Docker read-only commands",
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
			BuiltinID:   policy.BuiltinDangerousDeny,
			Name:        "Dangerous Command Deny",
			Description: "Deny dangerous system commands",
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
			BuiltinID:   policy.BuiltinSQLReadOnly,
			Name:        "SQL Read-Only",
			Description: "Allow query-only SQL statements",
			PolicyType:  PolicyTypeQuery,
			Policy: mustMarshal(&policy.QueryPolicy{
				AllowTypes: []string{
					"SELECT", "SHOW", "DESCRIBE", "EXPLAIN", "USE",
				},
			}),
		},
		{
			BuiltinID:   policy.BuiltinSQLDangerousDeny,
			Name:        "SQL Dangerous Deny",
			Description: "Deny dangerous SQL operations",
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
			BuiltinID:   policy.BuiltinRedisReadOnly,
			Name:        "Redis Read-Only",
			Description: "Allow Redis read-only commands",
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
			BuiltinID:   policy.BuiltinRedisDangerousDeny,
			Name:        "Redis Dangerous Deny",
			Description: "Deny dangerous Redis commands",
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
		// MongoDB 类型
		{
			BuiltinID:   policy.BuiltinMongoReadOnly,
			Name:        "MongoDB Read-Only",
			Description: "Allow MongoDB read-only operations",
			PolicyType:  PolicyTypeMongo,
			Policy: mustMarshal(&policy.MongoPolicy{
				AllowTypes: []string{
					"find", "findOne", "aggregate", "countDocuments",
				},
			}),
		},
		{
			BuiltinID:   policy.BuiltinMongoReadWrite,
			Name:        "MongoDB Read-Write",
			Description: "Allow MongoDB CRUD operations",
			PolicyType:  PolicyTypeMongo,
			Policy: mustMarshal(&policy.MongoPolicy{
				AllowTypes: []string{
					"find", "findOne", "aggregate", "countDocuments",
					"insertOne", "insertMany",
					"updateOne", "updateMany",
					"deleteOne", "deleteMany",
				},
			}),
		},
		{
			BuiltinID:   policy.BuiltinMongoDangerousDeny,
			Name:        "MongoDB Dangerous Deny",
			Description: "Deny dangerous MongoDB operations",
			PolicyType:  PolicyTypeMongo,
			Policy: mustMarshal(&policy.MongoPolicy{
				DenyTypes: []string{
					"dropDatabase", "dropCollection",
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
		builtinMap[pg.BuiltinID] = pg
	}
}

// FindBuiltin 按 ID 查找内置权限组
func FindBuiltin(id string) *PolicyGroup {
	return builtinMap[id]
}

// IsBuiltinID 检查 ID 是否为内置权限组
func IsBuiltinID(id string) bool {
	return strings.HasPrefix(id, policy.BuiltinPrefix)
}

const ExtensionPrefix = "ext:"

var (
	extensionGroupMu  sync.RWMutex
	extensionGroupMap = make(map[string]*PolicyGroup)
)

// IsExtensionID returns true if the ID has the ext: prefix.
func IsExtensionID(id string) bool {
	return strings.HasPrefix(id, ExtensionPrefix)
}

// RegisterExtensionGroup registers an extension-provided policy group.
func RegisterExtensionGroup(pg *PolicyGroup) {
	extensionGroupMu.Lock()
	defer extensionGroupMu.Unlock()
	extensionGroupMap[pg.BuiltinID] = pg
}

// FindExtensionGroup looks up an extension policy group by ID.
func FindExtensionGroup(id string) *PolicyGroup {
	extensionGroupMu.RLock()
	defer extensionGroupMu.RUnlock()
	return extensionGroupMap[id]
}

// hasExtensionPolicyType checks if any extension group uses the given policy type.
func hasExtensionPolicyType(policyType string) bool {
	extensionGroupMu.RLock()
	defer extensionGroupMu.RUnlock()
	for _, pg := range extensionGroupMap {
		if pg.PolicyType == policyType {
			return true
		}
	}
	return false
}

// ExtensionGroups returns all registered extension policy groups.
func ExtensionGroups() []*PolicyGroup {
	extensionGroupMu.RLock()
	defer extensionGroupMu.RUnlock()
	groups := make([]*PolicyGroup, 0, len(extensionGroupMap))
	for _, pg := range extensionGroupMap {
		groups = append(groups, pg)
	}
	return groups
}

// UnregisterExtensionGroups removes all extension groups for a given policy type.
func UnregisterExtensionGroups(policyType string) {
	extensionGroupMu.Lock()
	defer extensionGroupMu.Unlock()
	for id, pg := range extensionGroupMap {
		if pg.PolicyType == policyType {
			delete(extensionGroupMap, id)
		}
	}
}

// UnregisterExtensionGroupsByExtension removes all extension groups for a given extension name.
func UnregisterExtensionGroupsByExtension(extName string) {
	extensionGroupMu.Lock()
	defer extensionGroupMu.Unlock()
	for id, pg := range extensionGroupMap {
		if pg.ExtensionName == extName {
			delete(extensionGroupMap, id)
		}
	}
}
