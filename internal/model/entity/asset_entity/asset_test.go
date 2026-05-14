package asset_entity

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestAsset_Validate(t *testing.T) {
	convey.Convey("资产校验", t, func() {
		convey.Convey("名称为空时应返回错误", func() {
			a := &Asset{Type: AssetTypeSSH}
			err := a.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "名称")
		})

		convey.Convey("类型为空时应返回错误", func() {
			a := &Asset{Name: "test"}
			err := a.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "类型")
		})

		convey.Convey("扩展资产类型校验应通过", func() {
			a := &Asset{Name: "test", Type: "oss"}
			err := a.Validate()
			assert.NoError(t, err)
		})

		convey.Convey("SSH类型缺少Config应返回错误", func() {
			a := &Asset{Name: "test", Type: AssetTypeSSH}
			err := a.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "SSH")
		})

		convey.Convey("SSH类型配置完整时校验通过", func() {
			a := &Asset{Name: "test", Type: AssetTypeSSH}
			err := a.SetSSHConfig(&SSHConfig{
				Host:     "192.168.1.1",
				Port:     22,
				Username: "root",
				AuthType: AuthTypePassword,
			})
			assert.NoError(t, err)
			err = a.Validate()
			assert.NoError(t, err)
		})

		convey.Convey("SSH配置缺少host应返回错误", func() {
			a := &Asset{Name: "test", Type: AssetTypeSSH}
			_ = a.SetSSHConfig(&SSHConfig{
				Port:     22,
				Username: "root",
				AuthType: AuthTypePassword,
			})
			err := a.Validate()
			assert.Error(t, err)
		})
	})
}

func TestAsset_SSHConfig(t *testing.T) {
	convey.Convey("SSH配置序列化与反序列化", t, func() {
		convey.Convey("SetSSHConfig后GetSSHConfig应返回相同内容", func() {
			a := &Asset{Name: "test", Type: AssetTypeSSH}
			cfg := &SSHConfig{
				Host:     "10.0.0.1",
				Port:     2222,
				Username: "admin",
				AuthType: AuthTypeKey,
			}
			err := a.SetSSHConfig(cfg)
			assert.NoError(t, err)

			got, err := a.GetSSHConfig()
			assert.NoError(t, err)
			assert.Equal(t, cfg.Host, got.Host)
			assert.Equal(t, cfg.Port, got.Port)
			assert.Equal(t, cfg.Username, got.Username)
			assert.Equal(t, cfg.AuthType, got.AuthType)
		})

		convey.Convey("非SSH类型调用GetSSHConfig应返回错误", func() {
			a := &Asset{Name: "test", Type: "db"}
			_, err := a.GetSSHConfig()
			assert.Error(t, err)
		})

		convey.Convey("Config为空时GetSSHConfig应返回错误", func() {
			a := &Asset{Name: "test", Type: AssetTypeSSH}
			_, err := a.GetSSHConfig()
			assert.Error(t, err)
		})
	})
}

func TestAsset_IsSSH(t *testing.T) {
	convey.Convey("IsSSH判断", t, func() {
		convey.Convey("SSH类型返回true", func() {
			a := &Asset{Type: AssetTypeSSH}
			assert.True(t, a.IsSSH())
		})
		convey.Convey("其他类型返回false", func() {
			a := &Asset{Type: "db"}
			assert.False(t, a.IsSSH())
		})
	})
}

func TestAsset_CanConnect(t *testing.T) {
	convey.Convey("CanConnect判断", t, func() {
		convey.Convey("活跃的SSH资产且配置完整可连接", func() {
			a := &Asset{Name: "test", Type: AssetTypeSSH, Status: StatusActive}
			_ = a.SetSSHConfig(&SSHConfig{
				Host: "10.0.0.1", Port: 22, Username: "root", AuthType: AuthTypePassword,
			})
			assert.True(t, a.CanConnect())
		})

		convey.Convey("非活跃资产不可连接", func() {
			a := &Asset{Name: "test", Type: AssetTypeSSH, Status: StatusDeleted}
			_ = a.SetSSHConfig(&SSHConfig{
				Host: "10.0.0.1", Port: 22, Username: "root", AuthType: AuthTypePassword,
			})
			assert.False(t, a.CanConnect())
		})

		convey.Convey("非SSH类型不可连接", func() {
			a := &Asset{Name: "test", Type: "db", Status: StatusActive}
			assert.False(t, a.CanConnect())
		})
	})
}

func TestAsset_SerialConfig(t *testing.T) {
	convey.Convey("串口配置序列化与反序列化", t, func() {
		a := &Asset{Name: "serial", Type: AssetTypeSerial}
		cfg := &SerialConfig{
			PortPath:    "COM3",
			BaudRate:    115200,
			DataBits:    8,
			StopBits:    "1",
			Parity:      "none",
			FlowControl: "hardware",
		}

		err := a.SetSerialConfig(cfg)
		assert.NoError(t, err)

		got, err := a.GetSerialConfig()
		assert.NoError(t, err)
		assert.Equal(t, cfg.PortPath, got.PortPath)
		assert.Equal(t, cfg.BaudRate, got.BaudRate)
		assert.Equal(t, cfg.DataBits, got.DataBits)
		assert.Equal(t, cfg.StopBits, got.StopBits)
		assert.Equal(t, cfg.Parity, got.Parity)
		assert.Equal(t, cfg.FlowControl, got.FlowControl)
		assert.True(t, a.IsSerial())
	})
}

func TestValidateSerial(t *testing.T) {
	convey.Convey("串口资产校验", t, func() {
		newAsset := func(mutate func(*SerialConfig)) *Asset {
			cfg := &SerialConfig{
				PortPath:    "COM3",
				BaudRate:    115200,
				DataBits:    8,
				StopBits:    "1",
				Parity:      "none",
				FlowControl: "none",
			}
			if mutate != nil {
				mutate(cfg)
			}
			a := &Asset{Name: "serial", Type: AssetTypeSerial}
			assert.NoError(t, a.SetSerialConfig(cfg))
			return a
		}

		convey.Convey("配置完整时校验通过", func() {
			assert.NoError(t, newAsset(nil).Validate())
		})

		convey.Convey("端口路径为空时返回错误", func() {
			assert.Error(t, newAsset(func(cfg *SerialConfig) { cfg.PortPath = "" }).Validate())
		})

		convey.Convey("波特率无效时返回错误", func() {
			assert.Error(t, newAsset(func(cfg *SerialConfig) { cfg.BaudRate = 0 }).Validate())
		})

		convey.Convey("数据位无效时返回错误", func() {
			assert.Error(t, newAsset(func(cfg *SerialConfig) { cfg.DataBits = 9 }).Validate())
		})

		convey.Convey("停止位无效时返回错误", func() {
			assert.Error(t, newAsset(func(cfg *SerialConfig) { cfg.StopBits = "3" }).Validate())
		})

		convey.Convey("校验位无效时返回错误", func() {
			assert.Error(t, newAsset(func(cfg *SerialConfig) { cfg.Parity = "invalid" }).Validate())
		})

		convey.Convey("流控模式无效时返回错误", func() {
			assert.Error(t, newAsset(func(cfg *SerialConfig) { cfg.FlowControl = "software" }).Validate())
		})
	})
}

func TestAsset_CanConnectSerial(t *testing.T) {
	convey.Convey("串口资产连接能力判断", t, func() {
		convey.Convey("活跃且端口路径存在时可连接", func() {
			a := &Asset{Name: "serial", Type: AssetTypeSerial, Status: StatusActive}
			assert.NoError(t, a.SetSerialConfig(&SerialConfig{PortPath: "COM3", BaudRate: 115200, DataBits: 8, StopBits: "1", Parity: "none"}))
			assert.True(t, a.CanConnect())
		})

		convey.Convey("端口路径为空时不可连接", func() {
			a := &Asset{Name: "serial", Type: AssetTypeSerial, Status: StatusActive}
			assert.NoError(t, a.SetSerialConfig(&SerialConfig{PortPath: "", BaudRate: 115200, DataBits: 8, StopBits: "1", Parity: "none"}))
			assert.False(t, a.CanConnect())
		})

		convey.Convey("非活跃串口资产不可连接", func() {
			a := &Asset{Name: "serial", Type: AssetTypeSerial, Status: StatusDeleted}
			assert.NoError(t, a.SetSerialConfig(&SerialConfig{PortPath: "COM3", BaudRate: 115200, DataBits: 8, StopBits: "1", Parity: "none"}))
			assert.False(t, a.CanConnect())
		})
	})
}

func TestAsset_SSHAddress(t *testing.T) {
	convey.Convey("SSHAddress格式", t, func() {
		a := &Asset{Name: "test", Type: AssetTypeSSH}
		_ = a.SetSSHConfig(&SSHConfig{Host: "10.0.0.1", Port: 2222, Username: "root", AuthType: AuthTypePassword})

		addr, err := a.SSHAddress()
		assert.NoError(t, err)
		assert.Equal(t, "10.0.0.1:2222", addr)
	})
}

func TestRedisConfig(t *testing.T) {
	convey.Convey("Redis配置序列化与反序列化", t, func() {
		a := &Asset{Name: "cache", Type: AssetTypeRedis}
		cfg := &RedisConfig{
			Host:                  "redis.example.com",
			Port:                  6380,
			Username:              "default",
			Database:              2,
			TLS:                   true,
			TLSInsecure:           true,
			TLSServerName:         "redis.internal",
			TLSCAFile:             "/etc/redis/ca.pem",
			TLSCertFile:           "/etc/redis/client.pem",
			TLSKeyFile:            "/etc/redis/client-key.pem",
			CommandTimeoutSeconds: 7,
			ScanPageSize:          500,
			KeySeparator:          "/",
		}

		err := a.SetRedisConfig(cfg)
		assert.NoError(t, err)

		got, err := a.GetRedisConfig()
		assert.NoError(t, err)
		assert.Equal(t, cfg.Host, got.Host)
		assert.Equal(t, cfg.Port, got.Port)
		assert.Equal(t, cfg.Username, got.Username)
		assert.Equal(t, cfg.Database, got.Database)
		assert.Equal(t, cfg.TLS, got.TLS)
		assert.Equal(t, cfg.TLSInsecure, got.TLSInsecure)
		assert.Equal(t, cfg.TLSServerName, got.TLSServerName)
		assert.Equal(t, cfg.TLSCAFile, got.TLSCAFile)
		assert.Equal(t, cfg.TLSCertFile, got.TLSCertFile)
		assert.Equal(t, cfg.TLSKeyFile, got.TLSKeyFile)
		assert.Equal(t, cfg.CommandTimeoutSeconds, got.CommandTimeoutSeconds)
		assert.Equal(t, cfg.ScanPageSize, got.ScanPageSize)
		assert.Equal(t, cfg.KeySeparator, got.KeySeparator)
	})
}

func TestMongoDBConfig(t *testing.T) {
	convey.Convey("MongoDB配置序列化与反序列化", t, func() {
		convey.Convey("IsMongoDB类型判断", func() {
			a := &Asset{Type: AssetTypeMongoDB}
			assert.True(t, a.IsMongoDB())
			b := &Asset{Type: AssetTypeSSH}
			assert.False(t, b.IsMongoDB())
		})

		convey.Convey("SetMongoDBConfig后GetMongoDBConfig应返回相同内容", func() {
			a := &Asset{Name: "test", Type: AssetTypeMongoDB}
			cfg := &MongoDBConfig{
				Host:     "mongo.example.com",
				Port:     27017,
				Username: "admin",
				Password: "secret",
				Database: "mydb",
			}
			err := a.SetMongoDBConfig(cfg)
			assert.NoError(t, err)

			got, err := a.GetMongoDBConfig()
			assert.NoError(t, err)
			assert.Equal(t, cfg.Host, got.Host)
			assert.Equal(t, cfg.Port, got.Port)
			assert.Equal(t, cfg.Username, got.Username)
			assert.Equal(t, cfg.Database, got.Database)
		})

		convey.Convey("非MongoDB类型调用GetMongoDBConfig应返回错误", func() {
			a := &Asset{Name: "test", Type: AssetTypeSSH}
			_, err := a.GetMongoDBConfig()
			assert.Error(t, err)
		})

		convey.Convey("ConnectionURI模式配置序列化", func() {
			a := &Asset{Name: "test", Type: AssetTypeMongoDB}
			cfg := &MongoDBConfig{ //nolint:gosec // test data
				ConnectionURI: "mongodb://user:pass@host:27017/db",
			}
			err := a.SetMongoDBConfig(cfg)
			assert.NoError(t, err)

			got, err := a.GetMongoDBConfig()
			assert.NoError(t, err)
			assert.Equal(t, cfg.ConnectionURI, got.ConnectionURI)
		})
	})
}

func TestValidateMongoDB(t *testing.T) {
	convey.Convey("MongoDB资产校验", t, func() {
		convey.Convey("ConnectionURI模式校验通过", func() {
			a := &Asset{Name: "test", Type: AssetTypeMongoDB}
			_ = a.SetMongoDBConfig(&MongoDBConfig{
				ConnectionURI: "mongodb://localhost:27017",
			})
			err := a.Validate()
			assert.NoError(t, err)
		})

		convey.Convey("手动模式配置完整时校验通过", func() {
			a := &Asset{Name: "test", Type: AssetTypeMongoDB}
			_ = a.SetMongoDBConfig(&MongoDBConfig{
				Host: "localhost",
				Port: 27017,
			})
			err := a.Validate()
			assert.NoError(t, err)
		})

		convey.Convey("手动模式缺少Host应返回错误", func() {
			a := &Asset{Name: "test", Type: AssetTypeMongoDB}
			_ = a.SetMongoDBConfig(&MongoDBConfig{
				Port: 27017,
			})
			err := a.Validate()
			assert.Error(t, err)
		})

		convey.Convey("空URI且无Host应返回错误", func() {
			a := &Asset{Name: "test", Type: AssetTypeMongoDB}
			_ = a.SetMongoDBConfig(&MongoDBConfig{
				ConnectionURI: "",
				Port:          27017,
			})
			err := a.Validate()
			assert.Error(t, err)
		})
	})
}

func TestKafkaConfig(t *testing.T) {
	convey.Convey("Kafka配置序列化与反序列化", t, func() {
		a := &Asset{Name: "kafka", Type: AssetTypeKafka, Status: StatusActive}
		cfg := &KafkaConfig{
			Brokers:               []string{"broker-1:9092", "broker-2:9092"},
			ClientID:              "opskat-test",
			SASLMechanism:         KafkaSASLSCRAMSHA256,
			Username:              "alice",
			Password:              "encrypted",
			TLS:                   true,
			TLSInsecure:           true,
			RequestTimeoutSeconds: 10,
			MessagePreviewBytes:   2048,
			MessageFetchLimit:     50,
		}
		err := a.SetKafkaConfig(cfg)
		assert.NoError(t, err)

		got, err := a.GetKafkaConfig()
		assert.NoError(t, err)
		assert.Equal(t, cfg.Brokers, got.Brokers)
		assert.Equal(t, cfg.ClientID, got.ClientID)
		assert.Equal(t, cfg.SASLMechanism, got.SASLMechanism)
		assert.Equal(t, cfg.Username, got.Username)
		assert.Equal(t, cfg.TLS, got.TLS)
		assert.True(t, a.IsKafka())
		assert.True(t, a.CanConnect())
	})
}

func TestValidateKafka(t *testing.T) {
	convey.Convey("Kafka资产校验", t, func() {
		convey.Convey("明文连接配置完整时校验通过", func() {
			a := &Asset{Name: "kafka", Type: AssetTypeKafka}
			_ = a.SetKafkaConfig(&KafkaConfig{Brokers: []string{"localhost:9092"}})
			assert.NoError(t, a.Validate())
		})

		convey.Convey("broker缺少端口应返回错误", func() {
			a := &Asset{Name: "kafka", Type: AssetTypeKafka}
			_ = a.SetKafkaConfig(&KafkaConfig{Brokers: []string{"localhost"}})
			assert.Error(t, a.Validate())
		})

		convey.Convey("SASL启用时需要用户名和密码来源", func() {
			a := &Asset{Name: "kafka", Type: AssetTypeKafka}
			_ = a.SetKafkaConfig(&KafkaConfig{
				Brokers:       []string{"localhost:9092"},
				SASLMechanism: KafkaSASLPlain,
			})
			assert.Error(t, a.Validate())
		})

		convey.Convey("TLS证书和私钥必须成对配置", func() {
			a := &Asset{Name: "kafka", Type: AssetTypeKafka}
			_ = a.SetKafkaConfig(&KafkaConfig{
				Brokers:     []string{"localhost:9092"},
				TLSCertFile: "/tmp/client.pem",
			})
			assert.Error(t, a.Validate())
		})
	})
}
