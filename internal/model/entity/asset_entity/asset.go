package asset_entity

import (
	"errors"
	"fmt"
	"sync"

	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/pkg/jsonfield"
)

// 扩展资产类型注册表
var (
	extAssetTypesMu sync.RWMutex
	extAssetTypes   = make(map[string]struct{})
)

// RegisterExtensionAssetType 注册扩展提供的资产类型
func RegisterExtensionAssetType(assetType string) {
	extAssetTypesMu.Lock()
	defer extAssetTypesMu.Unlock()
	extAssetTypes[assetType] = struct{}{}
}

// UnregisterExtensionAssetType 注销扩展提供的资产类型
func UnregisterExtensionAssetType(assetType string) {
	extAssetTypesMu.Lock()
	defer extAssetTypesMu.Unlock()
	delete(extAssetTypes, assetType)
}

// IsExtensionAssetType 检查是否为已注册的扩展资产类型
func IsExtensionAssetType(assetType string) bool {
	extAssetTypesMu.RLock()
	defer extAssetTypesMu.RUnlock()
	_, ok := extAssetTypes[assetType]
	return ok
}

// 资产类型常量
const (
	AssetTypeSSH      = "ssh"
	AssetTypeDatabase = "database"
	AssetTypeRedis    = "redis"
)

// DatabaseDriver 数据库驱动类型
type DatabaseDriver string

const (
	DriverMySQL      DatabaseDriver = "mysql"
	DriverPostgreSQL DatabaseDriver = "postgresql"
)

// DefaultPort 返回驱动默认端口
func (d DatabaseDriver) DefaultPort() int {
	switch d {
	case DriverMySQL:
		return 3306
	case DriverPostgreSQL:
		return 5432
	default:
		return 0
	}
}

// 认证方式常量
const (
	AuthTypePassword = "password"
	AuthTypeKey      = "key"
)

// 状态常量
const (
	StatusActive  = 1
	StatusDeleted = 2
)

// CommandPolicy 命令权限策略（类型别名，定义在 policy 包）
type CommandPolicy = policy.CommandPolicy

// DefaultCommandPolicy 返回默认命令权限策略
var DefaultCommandPolicy = policy.DefaultCommandPolicy

// Asset 通用资产实体（充血模型）
type Asset struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string `gorm:"column:name;type:varchar(255);not null"`
	Type        string `gorm:"column:type;type:varchar(50);not null;index"`
	GroupID     int64  `gorm:"column:group_id;index"`
	Icon        string `gorm:"column:icon;type:varchar(100)"`
	Tags        string `gorm:"column:tags;type:text"`
	Description string `gorm:"column:description;type:text"`
	Config      string `gorm:"column:config;type:text"`
	CmdPolicy   string `gorm:"column:command_policy;type:text"`
	SortOrder   int    `gorm:"column:sort_order;default:0"`
	Status      int    `gorm:"column:status;default:1"`
	Createtime  int64  `gorm:"column:createtime"`
	Updatetime  int64  `gorm:"column:updatetime"`
}

// TableName GORM表名
func (Asset) TableName() string {
	return "assets"
}

// SSHConfig SSH类型的特定配置
type SSHConfig struct {
	Host         string       `json:"host"`
	Port         int          `json:"port"`
	Username     string       `json:"username"`
	AuthType     string       `json:"auth_type"`
	Password     string       `json:"password,omitempty"`      // 加密后的密码（内联，向后兼容）
	CredentialID int64        `json:"credential_id,omitempty"` // 统一凭证 ID（密码或密钥）
	PrivateKeys  []string     `json:"private_keys,omitempty"`  // 本地密钥文件路径（向后兼容）
	JumpHostID   int64        `json:"jump_host_id,omitempty"`
	Proxy        *ProxyConfig `json:"proxy,omitempty"`
}

// ProxyConfig 代理配置
type ProxyConfig struct {
	Type     string `json:"type"` // "socks5" | "socks4" | "http"
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// DatabaseConfig 数据库类型的特定配置
type DatabaseConfig struct {
	Driver       DatabaseDriver `json:"driver"`
	Host         string         `json:"host"`
	Port         int            `json:"port"`
	Username     string         `json:"username"`
	Password     string         `json:"password,omitempty"`      // credential_svc 加密（内联，向后兼容）
	CredentialID int64          `json:"credential_id,omitempty"` // 统一凭证 ID（密码）
	Database     string         `json:"database,omitempty"`      // 默认数据库
	SSLMode      string         `json:"ssl_mode,omitempty"`      // postgresql: disable/require/verify-full
	TLS          bool           `json:"tls,omitempty"`           // mysql: 启用 TLS 加密连接
	Params       string         `json:"params,omitempty"`        // 额外连接参数
	ReadOnly     bool           `json:"read_only,omitempty"`     // 连接级只读
	SSHAssetID   int64          `json:"ssh_asset_id,omitempty"`  // 0=直连, >0=SSH隧道
}

// RedisConfig Redis类型的特定配置
type RedisConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	CredentialID int64  `json:"credential_id,omitempty"` // 统一凭证 ID（密码）
	Database     int    `json:"database,omitempty"`      // DB index
	TLS          bool   `json:"tls,omitempty"`
	SSHAssetID   int64  `json:"ssh_asset_id,omitempty"`
}

// QueryPolicy SQL 权限策略（类型别名，定义在 policy 包）
type QueryPolicy = policy.QueryPolicy

// DefaultQueryPolicy 返回默认 SQL 权限策略
var DefaultQueryPolicy = policy.DefaultQueryPolicy

// RedisPolicy Redis 权限策略（类型别名，定义在 policy 包）
type RedisPolicy = policy.RedisPolicy

// DefaultRedisPolicy 返回默认 Redis 权限策略
var DefaultRedisPolicy = policy.DefaultRedisPolicy

// --- 充血模型方法 ---

// IsSSH 判断是否SSH类型
func (a *Asset) IsSSH() bool {
	return a.Type == AssetTypeSSH
}

// IsDatabase 判断是否数据库类型
func (a *Asset) IsDatabase() bool {
	return a.Type == AssetTypeDatabase
}

// IsRedis 判断是否Redis类型
func (a *Asset) IsRedis() bool {
	return a.Type == AssetTypeRedis
}

// GetSSHConfig 解析SSH配置
func (a *Asset) GetSSHConfig() (*SSHConfig, error) {
	if !a.IsSSH() {
		return nil, errors.New("资产不是SSH类型")
	}
	return jsonfield.Unmarshal[SSHConfig](a.Config, "SSH配置")
}

// SetSSHConfig 序列化SSH配置到Config字段
func (a *Asset) SetSSHConfig(cfg *SSHConfig) error {
	s, err := jsonfield.Marshal(cfg, "SSH配置")
	if err != nil {
		return err
	}
	a.Config = s
	return nil
}

// GetDatabaseConfig 解析数据库配置
func (a *Asset) GetDatabaseConfig() (*DatabaseConfig, error) {
	if !a.IsDatabase() {
		return nil, errors.New("资产不是数据库类型")
	}
	return jsonfield.Unmarshal[DatabaseConfig](a.Config, "数据库配置")
}

// SetDatabaseConfig 序列化数据库配置到Config字段
func (a *Asset) SetDatabaseConfig(cfg *DatabaseConfig) error {
	s, err := jsonfield.Marshal(cfg, "数据库配置")
	if err != nil {
		return err
	}
	a.Config = s
	return nil
}

// GetRedisConfig 解析Redis配置
func (a *Asset) GetRedisConfig() (*RedisConfig, error) {
	if !a.IsRedis() {
		return nil, errors.New("资产不是Redis类型")
	}
	return jsonfield.Unmarshal[RedisConfig](a.Config, "Redis配置")
}

// SetRedisConfig 序列化Redis配置到Config字段
func (a *Asset) SetRedisConfig(cfg *RedisConfig) error {
	s, err := jsonfield.Marshal(cfg, "Redis配置")
	if err != nil {
		return err
	}
	a.Config = s
	return nil
}

// GetQueryPolicy 解析SQL权限策略（database类型）
func (a *Asset) GetQueryPolicy() (*QueryPolicy, error) {
	return jsonfield.UnmarshalOrDefault[QueryPolicy](a.CmdPolicy, "SQL权限策略")
}

// SetQueryPolicy 序列化SQL权限策略
func (a *Asset) SetQueryPolicy(p *QueryPolicy) error {
	s, err := jsonfield.MarshalOrClear(p, func(v *QueryPolicy) bool {
		return v.IsEmpty()
	}, "SQL权限策略")
	if err != nil {
		return err
	}
	a.CmdPolicy = s
	return nil
}

// GetRedisPolicy 解析Redis权限策略
func (a *Asset) GetRedisPolicy() (*RedisPolicy, error) {
	return jsonfield.UnmarshalOrDefault[RedisPolicy](a.CmdPolicy, "Redis权限策略")
}

// SetRedisPolicy 序列化Redis权限策略
func (a *Asset) SetRedisPolicy(p *RedisPolicy) error {
	s, err := jsonfield.MarshalOrClear(p, func(v *RedisPolicy) bool {
		return v.IsEmpty()
	}, "Redis权限策略")
	if err != nil {
		return err
	}
	a.CmdPolicy = s
	return nil
}

// Validate 校验资产必填字段和类型配置的完整性
func (a *Asset) Validate() error {
	if a.Name == "" {
		return errors.New("资产名称不能为空")
	}
	if a.Type == "" {
		return errors.New("资产类型不能为空")
	}

	// 校验类型是否合法
	switch a.Type {
	case AssetTypeSSH:
		return a.validateSSH()
	case AssetTypeDatabase:
		return a.validateDatabase()
	case AssetTypeRedis:
		return a.validateRedis()
	default:
		// 扩展提供的资产类型，配置校验由扩展自行处理
		if IsExtensionAssetType(a.Type) {
			return nil
		}
		return fmt.Errorf("无效的资产类型: %s", a.Type)
	}
}

// validateSSH 校验SSH类型特定配置
func (a *Asset) validateSSH() error {
	cfg, err := a.GetSSHConfig()
	if err != nil {
		return fmt.Errorf("SSH配置无效: %w", err)
	}
	if cfg.Host == "" {
		return errors.New("SSH主机地址不能为空")
	}
	if cfg.Port <= 0 {
		return errors.New("SSH端口无效")
	}
	if cfg.Username == "" {
		return errors.New("SSH用户名不能为空")
	}
	if cfg.AuthType == "" {
		return errors.New("SSH认证方式不能为空")
	}
	return nil
}

// validateDatabase 校验数据库类型特定配置
func (a *Asset) validateDatabase() error {
	cfg, err := a.GetDatabaseConfig()
	if err != nil {
		return fmt.Errorf("数据库配置无效: %w", err)
	}
	if cfg.Driver == "" {
		return errors.New("数据库驱动不能为空")
	}
	switch cfg.Driver {
	case DriverMySQL, DriverPostgreSQL:
	default:
		return fmt.Errorf("不支持的数据库驱动: %s", cfg.Driver)
	}
	if cfg.Host == "" {
		return errors.New("数据库主机地址不能为空")
	}
	if cfg.Port <= 0 {
		return errors.New("数据库端口无效")
	}
	if cfg.Username == "" {
		return errors.New("数据库用户名不能为空")
	}
	return nil
}

// validateRedis 校验Redis类型特定配置
func (a *Asset) validateRedis() error {
	cfg, err := a.GetRedisConfig()
	if err != nil {
		return fmt.Errorf("redis配置无效: %w", err)
	}
	if cfg.Host == "" {
		return errors.New("Redis主机地址不能为空")
	}
	if cfg.Port <= 0 {
		return errors.New("Redis端口无效")
	}
	return nil
}

// CanConnect 判断资产是否处于可连接状态
func (a *Asset) CanConnect() bool {
	if a.Status != StatusActive {
		return false
	}
	switch a.Type {
	case AssetTypeSSH:
		cfg, err := a.GetSSHConfig()
		if err != nil {
			return false
		}
		return cfg.Host != "" && cfg.Port > 0
	case AssetTypeDatabase:
		cfg, err := a.GetDatabaseConfig()
		if err != nil {
			return false
		}
		return cfg.Host != "" && cfg.Port > 0
	case AssetTypeRedis:
		cfg, err := a.GetRedisConfig()
		if err != nil {
			return false
		}
		return cfg.Host != "" && cfg.Port > 0
	}
	return false
}

// SSHAddress 返回 host:port 格式地址
func (a *Asset) SSHAddress() (string, error) {
	cfg, err := a.GetSSHConfig()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), nil
}

// GetCommandPolicy 解析命令权限策略
func (a *Asset) GetCommandPolicy() (*CommandPolicy, error) {
	return jsonfield.UnmarshalOrDefault[CommandPolicy](a.CmdPolicy, "命令权限策略")
}

// SetCommandPolicy 序列化命令权限策略
func (a *Asset) SetCommandPolicy(p *CommandPolicy) error {
	s, err := jsonfield.MarshalOrClear(p, func(v *CommandPolicy) bool {
		return v.IsEmpty()
	}, "命令权限策略")
	if err != nil {
		return err
	}
	a.CmdPolicy = s
	return nil
}
