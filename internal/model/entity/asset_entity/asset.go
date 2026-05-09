package asset_entity

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/pkg/jsonfield"
)

// 资产类型常量
const (
	AssetTypeSSH      = "ssh"
	AssetTypeDatabase = "database"
	AssetTypeRedis    = "redis"
	AssetTypeMongoDB  = "mongodb"
	AssetTypeKafka    = "kafka"
	AssetTypeK8s      = "k8s"
	AssetTypeSerial   = "serial"
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
	ID            int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name          string `gorm:"column:name;type:varchar(255);not null"`
	Type          string `gorm:"column:type;type:varchar(50);not null;index"`
	GroupID       int64  `gorm:"column:group_id;index"`
	Icon          string `gorm:"column:icon;type:varchar(100)"`
	Tags          string `gorm:"column:tags;type:text"`
	Description   string `gorm:"column:description;type:text"`
	Config        string `gorm:"column:config;type:text"`
	CmdPolicy     string `gorm:"column:command_policy;type:text"`
	SortOrder     int    `gorm:"column:sort_order;default:0"`
	SSHTunnelID   int64  `gorm:"column:ssh_tunnel_id;default:0" json:"sshTunnelId"`
	ExtensionName string `gorm:"column:extension_name;type:varchar(64);index" json:"extensionName,omitempty"`
	Status        int    `gorm:"column:status;default:1"`
	Createtime    int64  `gorm:"column:createtime"`
	Updatetime    int64  `gorm:"column:updatetime"`
}

// TableName GORM表名
func (Asset) TableName() string {
	return "assets"
}

// SSHConfig SSH类型的特定配置
type SSHConfig struct {
	Host                 string       `json:"host"`
	Port                 int          `json:"port"`
	Username             string       `json:"username"`
	AuthType             string       `json:"auth_type"`
	Password             string       `json:"password,omitempty"`               // 加密后的密码（内联，向后兼容）
	CredentialID         int64        `json:"credential_id,omitempty"`          // 统一凭证 ID（密码或密钥）
	PrivateKeys          []string     `json:"private_keys,omitempty"`           // 本地密钥文件路径（向后兼容）
	PrivateKeyPassphrase string       `json:"private_key_passphrase,omitempty"` // 本地密钥密码（加密存储）
	JumpHostID           int64        `json:"jump_host_id,omitempty"`           // Deprecated: use Asset.SSHTunnelID
	Proxy                *ProxyConfig `json:"proxy,omitempty"`
}

// ProxyConfig 代理配置
type ProxyConfig struct {
	Type     string `json:"type"` // "socks5"
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
	SSHAssetID   int64          `json:"ssh_asset_id,omitempty"`  // Deprecated: use Asset.SSHTunnelID
}

// RedisConfig Redis类型的特定配置
type RedisConfig struct {
	Host                  string `json:"host"`
	Port                  int    `json:"port"`
	Username              string `json:"username,omitempty"`
	Password              string `json:"password,omitempty"`
	CredentialID          int64  `json:"credential_id,omitempty"`           // 统一凭证 ID（密码）
	Database              int    `json:"database,omitempty"`                // DB index
	TLS                   bool   `json:"tls,omitempty"`                     // 启用 TLS 加密连接
	TLSInsecure           bool   `json:"tls_insecure,omitempty"`            // 跳过 TLS 证书校验
	TLSServerName         string `json:"tls_server_name,omitempty"`         // TLS SNI / ServerName
	TLSCAFile             string `json:"tls_ca_file,omitempty"`             // CA 证书路径
	TLSCertFile           string `json:"tls_cert_file,omitempty"`           // 客户端证书路径
	TLSKeyFile            string `json:"tls_key_file,omitempty"`            // 客户端私钥路径
	CommandTimeoutSeconds int    `json:"command_timeout_seconds,omitempty"` // Redis 命令超时，0 使用默认值
	ScanPageSize          int    `json:"scan_page_size,omitempty"`          // Key 扫描分页大小，0 使用默认值
	KeySeparator          string `json:"key_separator,omitempty"`           // 树形视图 key 分隔符，默认 ":"
	SSHAssetID            int64  `json:"ssh_asset_id,omitempty"`            // Deprecated: use Asset.SSHTunnelID
}

// MongoDBConfig MongoDB类型的特定配置
type MongoDBConfig struct {
	ConnectionURI string `json:"connection_uri,omitempty"` // 完整连接 URI（优先于手动配置）
	Host          string `json:"host,omitempty"`
	Port          int    `json:"port,omitempty"`
	ReplicaSet    string `json:"replica_set,omitempty"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	CredentialID  int64  `json:"credential_id,omitempty"` // 统一凭证 ID（密码）
	Database      string `json:"database,omitempty"`      // 默认数据库
	AuthSource    string `json:"auth_source,omitempty"`   // 认证源数据库
	TLS           bool   `json:"tls,omitempty"`
	SSHAssetID    int64  `json:"ssh_asset_id,omitempty"` // Deprecated: use Asset.SSHTunnelID
}

// Kafka SASL 机制常量
const (
	KafkaSASLNone        = "none"
	KafkaSASLPlain       = "plain"
	KafkaSASLSCRAMSHA256 = "scram-sha-256"
	KafkaSASLSCRAMSHA512 = "scram-sha-512"
)

// KafkaConfig Kafka 类型的特定配置
type KafkaConfig struct {
	Brokers               []string                  `json:"brokers"`
	ClientID              string                    `json:"client_id,omitempty"`
	SASLMechanism         string                    `json:"sasl_mechanism,omitempty"`
	Username              string                    `json:"username,omitempty"`
	Password              string                    `json:"password,omitempty"`
	CredentialID          int64                     `json:"credential_id,omitempty"`
	TLS                   bool                      `json:"tls,omitempty"`
	TLSInsecure           bool                      `json:"tls_insecure,omitempty"`
	TLSServerName         string                    `json:"tls_server_name,omitempty"`
	TLSCAFile             string                    `json:"tls_ca_file,omitempty"`
	TLSCertFile           string                    `json:"tls_cert_file,omitempty"`
	TLSKeyFile            string                    `json:"tls_key_file,omitempty"`
	RequestTimeoutSeconds int                       `json:"request_timeout_seconds,omitempty"`
	MessagePreviewBytes   int                       `json:"message_preview_bytes,omitempty"`
	MessageFetchLimit     int                       `json:"message_fetch_limit,omitempty"`
	SSHAssetID            int64                     `json:"ssh_asset_id,omitempty"` // Deprecated: use Asset.SSHTunnelID
	SchemaRegistry        KafkaSchemaRegistryConfig `json:"schema_registry,omitempty"`
	Connect               KafkaConnectConfig        `json:"connect,omitempty"`
}

// KafkaSchemaRegistryConfig Schema Registry companion 配置
type KafkaSchemaRegistryConfig struct {
	Enabled       bool   `json:"enabled,omitempty"`
	URL           string `json:"url,omitempty"`
	AuthType      string `json:"auth_type,omitempty"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	CredentialID  int64  `json:"credential_id,omitempty"`
	TLSInsecure   bool   `json:"tls_insecure,omitempty"`
	TLSServerName string `json:"tls_server_name,omitempty"`
	TLSCAFile     string `json:"tls_ca_file,omitempty"`
	TLSCertFile   string `json:"tls_cert_file,omitempty"`
	TLSKeyFile    string `json:"tls_key_file,omitempty"`
}

// KafkaConnectConfig Kafka Connect companion 配置
type KafkaConnectConfig struct {
	Enabled  bool                        `json:"enabled,omitempty"`
	Clusters []KafkaConnectClusterConfig `json:"clusters,omitempty"`
}

// KafkaConnectClusterConfig 单个 Kafka Connect 集群配置
type KafkaConnectClusterConfig struct {
	Name          string `json:"name,omitempty"`
	URL           string `json:"url,omitempty"`
	AuthType      string `json:"auth_type,omitempty"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	CredentialID  int64  `json:"credential_id,omitempty"`
	TLSInsecure   bool   `json:"tls_insecure,omitempty"`
	TLSServerName string `json:"tls_server_name,omitempty"`
	TLSCAFile     string `json:"tls_ca_file,omitempty"`
	TLSCertFile   string `json:"tls_cert_file,omitempty"`
	TLSKeyFile    string `json:"tls_key_file,omitempty"`
}

// K8sConfig K8S集群类型的特定配置
type K8sConfig struct {
	Kubeconfig string `json:"kubeconfig,omitempty"` // kubeconfig YAML 内容
	Namespace  string `json:"namespace,omitempty"`  // 默认命名空间
	Context    string `json:"context,omitempty"`    // kubeconfig context 名称
}

// K8sConfig PasswordSource implementation.
// Kubeconfig 在落库时由 assettype/k8s.go 加密；返回密文走通用解密路径
// （credential_resolver.ResolvePasswordGeneric）。
func (c *K8sConfig) GetCredentialID() int64 { return 0 }
func (c *K8sConfig) GetPassword() string    { return c.Kubeconfig }

// SerialConfig 串口（COM/TTY）类型的特定配置
type SerialConfig struct {
	PortPath    string `json:"port_path"`              // 串口路径，如 COM3, /dev/ttyUSB0
	BaudRate    int    `json:"baud_rate"`              // 波特率，如 9600, 115200
	DataBits    int    `json:"data_bits"`              // 数据位: 5, 6, 7, 8
	StopBits    string `json:"stop_bits"`              // 停止位: "1", "1.5", "2"
	Parity      string `json:"parity"`                 // 校验位: "none", "odd", "even", "mark", "space"
	FlowControl string `json:"flow_control,omitempty"` // 流控制: "none", "hardware", "software"
}

// DatabaseConfig PasswordSource implementation
func (c *DatabaseConfig) GetCredentialID() int64 { return c.CredentialID }
func (c *DatabaseConfig) GetPassword() string    { return c.Password }

// RedisConfig PasswordSource implementation
func (c *RedisConfig) GetCredentialID() int64 { return c.CredentialID }
func (c *RedisConfig) GetPassword() string    { return c.Password }

// MongoDBConfig PasswordSource implementation
func (c *MongoDBConfig) GetCredentialID() int64 { return c.CredentialID }
func (c *MongoDBConfig) GetPassword() string    { return c.Password }

// KafkaConfig PasswordSource implementation
func (c *KafkaConfig) GetCredentialID() int64 { return c.CredentialID }
func (c *KafkaConfig) GetPassword() string    { return c.Password }

// KafkaSchemaRegistryConfig PasswordSource implementation
func (c *KafkaSchemaRegistryConfig) GetCredentialID() int64 { return c.CredentialID }
func (c *KafkaSchemaRegistryConfig) GetPassword() string    { return c.Password }

// KafkaConnectClusterConfig PasswordSource implementation
func (c *KafkaConnectClusterConfig) GetCredentialID() int64 { return c.CredentialID }
func (c *KafkaConnectClusterConfig) GetPassword() string    { return c.Password }

// QueryPolicy SQL 权限策略（类型别名，定义在 policy 包）
type QueryPolicy = policy.QueryPolicy

// DefaultQueryPolicy 返回默认 SQL 权限策略
var DefaultQueryPolicy = policy.DefaultQueryPolicy

// RedisPolicy Redis 权限策略（类型别名，定义在 policy 包）
type RedisPolicy = policy.RedisPolicy

// DefaultRedisPolicy 返回默认 Redis 权限策略
var DefaultRedisPolicy = policy.DefaultRedisPolicy

// MongoPolicy MongoDB 权限策略（类型别名，定义在 policy 包）
type MongoPolicy = policy.MongoPolicy

// DefaultMongoPolicy 返回默认 MongoDB 权限策略
var DefaultMongoPolicy = policy.DefaultMongoPolicy

// KafkaPolicy Kafka 权限策略（类型别名，定义在 policy 包）
type KafkaPolicy = policy.KafkaPolicy

// DefaultKafkaPolicy 返回默认 Kafka 权限策略
var DefaultKafkaPolicy = policy.DefaultKafkaPolicy

// K8sPolicy K8S 权限策略（类型别名，定义在 policy 包）
type K8sPolicy = policy.K8sPolicy

// DefaultK8sPolicy 返回默认 K8S 权限策略
var DefaultK8sPolicy = policy.DefaultK8sPolicy

// SerialConfig PasswordSource implementation（串口无密码，返回空）
func (c *SerialConfig) GetCredentialID() int64 { return 0 }
func (c *SerialConfig) GetPassword() string    { return "" }

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

// IsMongoDB 判断是否MongoDB类型
func (a *Asset) IsMongoDB() bool {
	return a.Type == AssetTypeMongoDB
}

// IsKafka 判断是否 Kafka 类型
func (a *Asset) IsKafka() bool {
	return a.Type == AssetTypeKafka
}

// IsK8s 判断是否K8S集群类型
func (a *Asset) IsK8s() bool {
	return a.Type == AssetTypeK8s
}

// IsSerial 判断是否串口类型
func (a *Asset) IsSerial() bool {
	return a.Type == AssetTypeSerial
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

// GetMongoDBConfig 解析MongoDB配置
func (a *Asset) GetMongoDBConfig() (*MongoDBConfig, error) {
	if !a.IsMongoDB() {
		return nil, errors.New("资产不是MongoDB类型")
	}
	return jsonfield.Unmarshal[MongoDBConfig](a.Config, "MongoDB配置")
}

// SetMongoDBConfig 序列化MongoDB配置到Config字段
func (a *Asset) SetMongoDBConfig(cfg *MongoDBConfig) error {
	s, err := jsonfield.Marshal(cfg, "MongoDB配置")
	if err != nil {
		return err
	}
	a.Config = s
	return nil
}

// GetKafkaConfig 解析 Kafka 配置
func (a *Asset) GetKafkaConfig() (*KafkaConfig, error) {
	if !a.IsKafka() {
		return nil, errors.New("资产不是Kafka类型")
	}
	return jsonfield.Unmarshal[KafkaConfig](a.Config, "Kafka配置")
}

// SetKafkaConfig 序列化 Kafka 配置到 Config 字段
func (a *Asset) SetKafkaConfig(cfg *KafkaConfig) error {
	s, err := jsonfield.Marshal(cfg, "Kafka配置")
	if err != nil {
		return err
	}
	a.Config = s
	return nil
}

// GetK8sConfig 解析K8S配置
func (a *Asset) GetK8sConfig() (*K8sConfig, error) {
	if !a.IsK8s() {
		return nil, errors.New("资产不是K8S集群类型")
	}
	return jsonfield.Unmarshal[K8sConfig](a.Config, "K8S配置")
}

// SetK8sConfig 序列化K8S配置到Config字段
func (a *Asset) SetK8sConfig(cfg *K8sConfig) error {
	s, err := jsonfield.Marshal(cfg, "K8S配置")
	if err != nil {
		return err
	}
	a.Config = s
	return nil
}

// GetSerialConfig 解析串口配置
func (a *Asset) GetSerialConfig() (*SerialConfig, error) {
	if !a.IsSerial() {
		return nil, errors.New("资产不是串口类型")
	}
	return jsonfield.Unmarshal[SerialConfig](a.Config, "串口配置")
}

// SetSerialConfig 序列化串口配置到Config字段
func (a *Asset) SetSerialConfig(cfg *SerialConfig) error {
	s, err := jsonfield.Marshal(cfg, "串口配置")
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

// GetMongoPolicy 解析MongoDB权限策略
func (a *Asset) GetMongoPolicy() (*MongoPolicy, error) {
	return jsonfield.UnmarshalOrDefault[MongoPolicy](a.CmdPolicy, "MongoDB权限策略")
}

// SetMongoPolicy 序列化MongoDB权限策略
func (a *Asset) SetMongoPolicy(p *MongoPolicy) error {
	s, err := jsonfield.MarshalOrClear(p, func(v *MongoPolicy) bool {
		return v.IsEmpty()
	}, "MongoDB权限策略")
	if err != nil {
		return err
	}
	a.CmdPolicy = s
	return nil
}

// GetKafkaPolicy 解析 Kafka 权限策略
func (a *Asset) GetKafkaPolicy() (*KafkaPolicy, error) {
	return jsonfield.UnmarshalOrDefault[KafkaPolicy](a.CmdPolicy, "Kafka权限策略")
}

// SetKafkaPolicy 序列化 Kafka 权限策略
func (a *Asset) SetKafkaPolicy(p *KafkaPolicy) error {
	s, err := jsonfield.MarshalOrClear(p, func(v *KafkaPolicy) bool {
		return v.IsEmpty()
	}, "Kafka权限策略")
	if err != nil {
		return err
	}
	a.CmdPolicy = s
	return nil
}

// GetK8sPolicy 解析K8S权限策略
func (a *Asset) GetK8sPolicy() (*K8sPolicy, error) {
	return jsonfield.UnmarshalOrDefault[K8sPolicy](a.CmdPolicy, "K8S权限策略")
}

// SetK8sPolicy 序列化K8S权限策略
func (a *Asset) SetK8sPolicy(p *K8sPolicy) error {
	s, err := jsonfield.MarshalOrClear(p, func(v *K8sPolicy) bool {
		return v.IsEmpty()
	}, "K8S权限策略")
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
	case AssetTypeMongoDB:
		return a.validateMongoDB()
	case AssetTypeKafka:
		return a.validateKafka()
	case AssetTypeK8s:
		return a.validateK8s()
	case AssetTypeSerial:
		return a.validateSerial()
	default:
		// 扩展资产类型由扩展自行校验
		return nil
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

// validateMongoDB 校验MongoDB类型特定配置
func (a *Asset) validateMongoDB() error {
	cfg, err := a.GetMongoDBConfig()
	if err != nil {
		return fmt.Errorf("MongoDB配置无效: %w", err)
	}
	// URI 模式：直接通过
	if cfg.ConnectionURI != "" {
		return nil
	}
	// 手动配置模式：host 和 port 必填
	if cfg.Host == "" {
		return errors.New("MongoDB主机地址不能为空")
	}
	if cfg.Port <= 0 {
		return errors.New("MongoDB端口无效")
	}
	return nil
}

// validateKafka 校验 Kafka 类型特定配置
func (a *Asset) validateKafka() error {
	cfg, err := a.GetKafkaConfig()
	if err != nil {
		return fmt.Errorf("kafka配置无效: %w", err)
	}
	if len(cfg.Brokers) == 0 {
		return errors.New("kafka broker不能为空")
	}
	for _, broker := range cfg.Brokers {
		if err := validateKafkaBroker(broker); err != nil {
			return err
		}
	}
	switch normalizeKafkaSASLMechanism(cfg.SASLMechanism) {
	case KafkaSASLNone:
	case KafkaSASLPlain, KafkaSASLSCRAMSHA256, KafkaSASLSCRAMSHA512:
		if strings.TrimSpace(cfg.Username) == "" {
			return errors.New("kafka SASL用户名不能为空")
		}
		if cfg.CredentialID == 0 && strings.TrimSpace(cfg.Password) == "" {
			return errors.New("kafka SASL密码不能为空")
		}
	default:
		return fmt.Errorf("不支持的Kafka SASL机制: %s", cfg.SASLMechanism)
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return errors.New("kafka TLS客户端证书和私钥必须同时配置")
	}
	if cfg.RequestTimeoutSeconds < 0 || cfg.RequestTimeoutSeconds > 300 {
		return errors.New("kafka 请求超时时间无效")
	}
	if cfg.MessagePreviewBytes < 0 || cfg.MessagePreviewBytes > 1024*1024 {
		return errors.New("kafka 消息预览大小无效")
	}
	if cfg.MessageFetchLimit < 0 || cfg.MessageFetchLimit > 1000 {
		return errors.New("kafka 消息读取数量无效")
	}
	return nil
}

// validateK8s 校验K8S集群类型特定配置
func (a *Asset) validateK8s() error {
	cfg, err := a.GetK8sConfig()
	if err != nil {
		return fmt.Errorf("K8S配置无效: %w", err)
	}
	if cfg.Kubeconfig == "" {
		return errors.New("K8S集群kubeconfig不能为空")
	}
	return nil
}

// validateSerial 校验串口类型特定配置
func (a *Asset) validateSerial() error {
	cfg, err := a.GetSerialConfig()
	if err != nil {
		return fmt.Errorf("串口配置无效: %w", err)
	}
	if cfg.PortPath == "" {
		return errors.New("串口路径不能为空")
	}
	if cfg.BaudRate <= 0 {
		return errors.New("串口波特率无效")
	}
	if cfg.DataBits < 5 || cfg.DataBits > 8 {
		return errors.New("串口数据位必须在5-8之间")
	}
	switch cfg.StopBits {
	case "1", "1.5", "2":
	default:
		return fmt.Errorf("不支持的串口停止位: %s", cfg.StopBits)
	}
	switch cfg.Parity {
	case "none", "odd", "even", "mark", "space":
	default:
		return fmt.Errorf("不支持的串口校验位: %s", cfg.Parity)
	}
	switch cfg.FlowControl {
	case "", "none", "hardware":
	default:
		return fmt.Errorf("不支持的串口流控模式: %s", cfg.FlowControl)
	}
	return nil
}

func validateKafkaBroker(broker string) error {
	broker = strings.TrimSpace(broker)
	if broker == "" {
		return errors.New("kafka broker不能为空")
	}
	host, portText, err := net.SplitHostPort(broker)
	if err != nil || strings.TrimSpace(host) == "" {
		return fmt.Errorf("kafka broker必须为host:port格式: %s", broker)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("kafka broker端口无效: %s", broker)
	}
	return nil
}

func normalizeKafkaSASLMechanism(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return KafkaSASLNone
	}
	return v
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
	case AssetTypeMongoDB:
		cfg, err := a.GetMongoDBConfig()
		if err != nil {
			return false
		}
		if cfg.ConnectionURI != "" {
			return true
		}
		return cfg.Host != "" && cfg.Port > 0
	case AssetTypeKafka:
		cfg, err := a.GetKafkaConfig()
		if err != nil {
			return false
		}
		return len(cfg.Brokers) > 0
	case AssetTypeK8s:
		cfg, err := a.GetK8sConfig()
		if err != nil {
			return false
		}
		return cfg.Kubeconfig != ""
	case AssetTypeSerial:
		cfg, err := a.GetSerialConfig()
		if err != nil {
			return false
		}
		return cfg.PortPath != ""
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
