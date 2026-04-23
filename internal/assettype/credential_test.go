package assettype

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/credential_svc"
	"github.com/smartystreets/goconvey/convey"
)

// 验证 4 种 handler 的密码加密链路：明文经过 ApplyCreateArgs/ApplyUpdateArgs 写入后
// 不应等于明文，且能用 credential_svc 解密回来。
func TestHandlersEncryptInlinePassword(t *testing.T) {
	ctx := context.Background()

	convey.Convey("SSH ApplyCreateArgs 加密 password", t, func() {
		h := &sshHandler{}
		a := &asset_entity.Asset{Type: "ssh", Name: "ssh-1"}
		err := h.ApplyCreateArgs(ctx, a, map[string]any{
			"host": "10.0.0.1", "port": float64(22), "username": "root",
			"password": "p@ss-word",
		})
		convey.So(err, convey.ShouldBeNil)

		cfg, _ := a.GetSSHConfig()
		convey.So(cfg.Password, convey.ShouldNotBeEmpty)
		convey.So(cfg.Password, convey.ShouldNotEqual, "p@ss-word")
		convey.So(cfg.AuthType, convey.ShouldEqual, "password")

		decrypted, err := credential_svc.Default().Decrypt(cfg.Password)
		convey.So(err, convey.ShouldBeNil)
		convey.So(decrypted, convey.ShouldEqual, "p@ss-word")
	})

	convey.Convey("Database ApplyCreateArgs 加密 password", t, func() {
		h := &databaseHandler{}
		a := &asset_entity.Asset{Type: "database"}
		err := h.ApplyCreateArgs(ctx, a, map[string]any{
			"driver": "mysql", "host": "10.0.0.1", "port": float64(3306),
			"username": "admin", "password": "db-secret",
		})
		convey.So(err, convey.ShouldBeNil)

		cfg, _ := a.GetDatabaseConfig()
		convey.So(cfg.Password, convey.ShouldNotEqual, "db-secret")
		decrypted, _ := credential_svc.Default().Decrypt(cfg.Password)
		convey.So(decrypted, convey.ShouldEqual, "db-secret")
	})

	convey.Convey("Redis ApplyCreateArgs 加密 password 并支持 redis_db", t, func() {
		h := &redisHandler{}
		a := &asset_entity.Asset{Type: "redis"}
		err := h.ApplyCreateArgs(ctx, a, map[string]any{
			"host": "10.0.0.1", "port": float64(6379),
			"password": "redis-secret", "redis_db": float64(5),
		})
		convey.So(err, convey.ShouldBeNil)

		cfg, _ := a.GetRedisConfig()
		convey.So(cfg.Database, convey.ShouldEqual, 5)
		decrypted, _ := credential_svc.Default().Decrypt(cfg.Password)
		convey.So(decrypted, convey.ShouldEqual, "redis-secret")
	})

	convey.Convey("MongoDB ApplyCreateArgs 加密 password", t, func() {
		h := &mongodbHandler{}
		a := &asset_entity.Asset{Type: "mongodb"}
		err := h.ApplyCreateArgs(ctx, a, map[string]any{
			"host": "10.0.0.1", "port": float64(27017),
			"username": "admin", "password": "mongo-secret",
		})
		convey.So(err, convey.ShouldBeNil)

		cfg, _ := a.GetMongoDBConfig()
		decrypted, _ := credential_svc.Default().Decrypt(cfg.Password)
		convey.So(decrypted, convey.ShouldEqual, "mongo-secret")
	})
}

// SSH 更新 password 时必须连带把 auth_type 强制切回 password 并清掉 CredentialID，
// 否则原本走 key/CredentialID 的资产会落入 auth_type=key + 无 key 的死状态。
func TestSSHUpdatePasswordResetsAuth(t *testing.T) {
	convey.Convey("SSH ApplyUpdateArgs 用 password 覆盖 key 认证", t, func() {
		h := &sshHandler{}
		a := &asset_entity.Asset{Type: "ssh", Name: "srv"}
		_ = a.SetSSHConfig(&asset_entity.SSHConfig{
			Host: "10.0.0.1", Port: 22, Username: "root",
			AuthType: "key", CredentialID: 99,
		})

		err := h.ApplyUpdateArgs(context.Background(), a, map[string]any{
			"password": "new-pass",
		})
		convey.So(err, convey.ShouldBeNil)

		cfg, _ := a.GetSSHConfig()
		convey.So(cfg.AuthType, convey.ShouldEqual, "password")
		convey.So(cfg.CredentialID, convey.ShouldEqual, 0)
		convey.So(cfg.Password, convey.ShouldNotBeEmpty)
		decrypted, _ := credential_svc.Default().Decrypt(cfg.Password)
		convey.So(decrypted, convey.ShouldEqual, "new-pass")
	})

	convey.Convey("显式 auth_type 在 password/private_key 处理之后生效（最终覆盖）", t, func() {
		h := &sshHandler{}
		a := &asset_entity.Asset{Type: "ssh", Name: "srv"}
		_ = a.SetSSHConfig(&asset_entity.SSHConfig{
			Host: "10.0.0.1", Port: 22, Username: "root", AuthType: "password",
		})
		err := h.ApplyUpdateArgs(context.Background(), a, map[string]any{
			"auth_type": "key",
		})
		convey.So(err, convey.ShouldBeNil)
		cfg, _ := a.GetSSHConfig()
		convey.So(cfg.AuthType, convey.ShouldEqual, "key")
	})
}

// update 字段补齐：database 现在能改 driver/database/read_only/ssh_asset_id/password，
// redis 能改 redis_db/ssh_asset_id，mongodb 能改 database/ssh_asset_id。
func TestUpdateExtendedFields(t *testing.T) {
	ctx := context.Background()

	convey.Convey("Database 更新 read_only/driver/ssh_asset_id", t, func() {
		h := &databaseHandler{}
		a := &asset_entity.Asset{Type: "database"}
		_ = a.SetDatabaseConfig(&asset_entity.DatabaseConfig{
			Driver: asset_entity.DriverMySQL, Host: "10.0.0.1", Port: 3306,
			Username: "admin", Database: "mydb", ReadOnly: false,
		})
		err := h.ApplyUpdateArgs(ctx, a, map[string]any{
			"driver": "postgresql", "read_only": "true", "ssh_asset_id": float64(7),
		})
		convey.So(err, convey.ShouldBeNil)
		cfg, _ := a.GetDatabaseConfig()
		convey.So(string(cfg.Driver), convey.ShouldEqual, "postgresql")
		convey.So(cfg.ReadOnly, convey.ShouldBeTrue)
		convey.So(cfg.SSHAssetID, convey.ShouldEqual, 7)
	})

	convey.Convey("Database 更新 database 字段允许清空", t, func() {
		h := &databaseHandler{}
		a := &asset_entity.Asset{Type: "database"}
		_ = a.SetDatabaseConfig(&asset_entity.DatabaseConfig{
			Driver: asset_entity.DriverMySQL, Database: "old",
		})
		err := h.ApplyUpdateArgs(ctx, a, map[string]any{"database": ""})
		convey.So(err, convey.ShouldBeNil)
		cfg, _ := a.GetDatabaseConfig()
		convey.So(cfg.Database, convey.ShouldEqual, "")
	})

	convey.Convey("Redis 更新 redis_db / ssh_asset_id", t, func() {
		h := &redisHandler{}
		a := &asset_entity.Asset{Type: "redis"}
		_ = a.SetRedisConfig(&asset_entity.RedisConfig{
			Host: "10.0.0.1", Port: 6379, Database: 0,
		})
		err := h.ApplyUpdateArgs(ctx, a, map[string]any{
			"redis_db": float64(3), "ssh_asset_id": float64(11),
		})
		convey.So(err, convey.ShouldBeNil)
		cfg, _ := a.GetRedisConfig()
		convey.So(cfg.Database, convey.ShouldEqual, 3)
		convey.So(cfg.SSHAssetID, convey.ShouldEqual, 11)
	})

	convey.Convey("MongoDB 更新 ssh_asset_id 落到 Asset.SSHTunnelID", t, func() {
		h := &mongodbHandler{}
		a := &asset_entity.Asset{Type: "mongodb"}
		_ = a.SetMongoDBConfig(&asset_entity.MongoDBConfig{
			Host: "10.0.0.1", Port: 27017, Database: "mydb",
		})
		err := h.ApplyUpdateArgs(ctx, a, map[string]any{
			"ssh_asset_id": float64(42),
		})
		convey.So(err, convey.ShouldBeNil)
		convey.So(a.SSHTunnelID, convey.ShouldEqual, 42)
	})
}
