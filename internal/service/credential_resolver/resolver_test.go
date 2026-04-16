package credential_resolver

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/credential_svc"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func setupCredentialSvc(t *testing.T) *credential_svc.CredentialSvc {
	svc := credential_svc.New("test-master-key-1234567890abcdef", []byte("test-salt-16byte"))
	credential_svc.SetDefault(svc)
	t.Cleanup(func() { credential_svc.SetDefault(nil) })
	return svc
}

func TestResolveDatabasePassword(t *testing.T) {
	convey.Convey("解析数据库密码", t, func() {
		svc := setupCredentialSvc(t)
		r := Default()

		convey.Convey("空密码返回空字符串", func() {
			cfg := &asset_entity.DatabaseConfig{Password: ""}
			password, err := r.ResolveDatabasePassword(context.Background(), cfg)
			assert.NoError(t, err)
			assert.Equal(t, "", password)
		})

		convey.Convey("正确解密已加密的密码", func() {
			encrypted, err := svc.Encrypt("db-secret-123")
			assert.NoError(t, err)

			cfg := &asset_entity.DatabaseConfig{Password: encrypted}
			password, err := r.ResolveDatabasePassword(context.Background(), cfg)
			assert.NoError(t, err)
			assert.Equal(t, "db-secret-123", password)
		})

		convey.Convey("无效密文返回错误", func() {
			cfg := &asset_entity.DatabaseConfig{Password: "plain-text-not-encrypted"} //nolint:gosec
			_, err := r.ResolveDatabasePassword(context.Background(), cfg)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "解密数据库密码失败")
		})
	})
}

func TestResolveRedisPassword(t *testing.T) {
	convey.Convey("解析 Redis 密码", t, func() {
		svc := setupCredentialSvc(t)
		r := Default()

		convey.Convey("空密码返回空字符串", func() {
			cfg := &asset_entity.RedisConfig{Password: ""}
			password, err := r.ResolveRedisPassword(context.Background(), cfg)
			assert.NoError(t, err)
			assert.Equal(t, "", password)
		})

		convey.Convey("正确解密已加密的密码", func() {
			encrypted, err := svc.Encrypt("redis-secret-456")
			assert.NoError(t, err)

			cfg := &asset_entity.RedisConfig{Password: encrypted}
			password, err := r.ResolveRedisPassword(context.Background(), cfg)
			assert.NoError(t, err)
			assert.Equal(t, "redis-secret-456", password)
		})

		convey.Convey("无效密文返回错误", func() {
			cfg := &asset_entity.RedisConfig{Password: "plain-text-not-encrypted"} //nolint:gosec
			_, err := r.ResolveRedisPassword(context.Background(), cfg)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "解密 Redis 密码失败")
		})
	})
}

func TestResolveMongoDBPassword(t *testing.T) {
	convey.Convey("解析 MongoDB 密码", t, func() {
		svc := setupCredentialSvc(t)
		r := Default()

		convey.Convey("空密码返回空字符串", func() {
			cfg := &asset_entity.MongoDBConfig{Password: ""}
			password, err := r.ResolveMongoDBPassword(context.Background(), cfg)
			assert.NoError(t, err)
			assert.Equal(t, "", password)
		})

		convey.Convey("正确解密已加密的密码", func() {
			encrypted, err := svc.Encrypt("mongodb-secret-789")
			assert.NoError(t, err)

			cfg := &asset_entity.MongoDBConfig{Password: encrypted}
			password, err := r.ResolveMongoDBPassword(context.Background(), cfg)
			assert.NoError(t, err)
			assert.Equal(t, "mongodb-secret-789", password)
		})

		convey.Convey("无效密文返回错误", func() {
			cfg := &asset_entity.MongoDBConfig{Password: "plain-text-not-encrypted"} //nolint:gosec
			_, err := r.ResolveMongoDBPassword(context.Background(), cfg)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "解密 MongoDB 密码失败")
		})
	})
}
