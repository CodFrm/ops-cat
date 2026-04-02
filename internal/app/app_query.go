package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/connpool"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/credential_resolver"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

// TestDatabaseConnection 测试数据库连接
// configJSON: DatabaseConfig JSON，plainPassword: 明文密码
func (a *App) TestDatabaseConnection(configJSON string, plainPassword string) error {
	var cfg asset_entity.DatabaseConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}
	password := plainPassword
	if password == "" {
		var err error
		password, err = credential_resolver.Default().ResolveDatabasePassword(a.langCtx(), &cfg)
		if err != nil {
			return fmt.Errorf("连接失败: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer cancel()

	// 测试连接场景没有持久化的 Asset，使用零值让 backward compat 生效
	testAsset := &asset_entity.Asset{}
	db, tunnel, err := connpool.DialDatabase(ctx, testAsset, &cfg, password, a.sshPool)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Default().Warn("close db failed", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close tunnel failed", zap.Error(err))
			}
		}
	}()
	return nil
}

// TestRedisConnection 测试 Redis 连接
// configJSON: RedisConfig JSON，plainPassword: 明文密码
func (a *App) TestRedisConnection(configJSON string, plainPassword string) error {
	var cfg asset_entity.RedisConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}
	password := plainPassword
	if password == "" {
		var err error
		password, err = credential_resolver.Default().ResolveRedisPassword(a.langCtx(), &cfg)
		if err != nil {
			return fmt.Errorf("连接失败: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer cancel()

	// 测试连接场景没有持久化的 Asset，使用零值让 backward compat 生效
	testAsset := &asset_entity.Asset{}
	client, tunnel, err := connpool.DialRedis(ctx, testAsset, &cfg, password, a.sshPool)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Close(); err != nil {
			logger.Default().Warn("close redis client failed", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close tunnel failed", zap.Error(err))
			}
		}
	}()
	return nil
}

// ExecuteSQL 在指定数据库资产上执行 SQL 查询
func (a *App) ExecuteSQL(assetID int64, sqlText string, database string) (string, error) {
	asset, err := asset_svc.Asset().Get(a.langCtx(), assetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsDatabase() {
		return "", fmt.Errorf("资产不是数据库类型")
	}
	cfg, err := asset.GetDatabaseConfig()
	if err != nil {
		return "", fmt.Errorf("获取数据库配置失败: %w", err)
	}
	if database != "" {
		cfg.Database = database
	}
	password, err := credential_resolver.Default().ResolveDatabasePassword(a.langCtx(), cfg)
	if err != nil {
		return "", fmt.Errorf("解析凭据失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 30*time.Second)
	defer cancel()

	db, tunnel, err := connpool.DialDatabase(ctx, asset, cfg, password, a.sshPool)
	if err != nil {
		return "", fmt.Errorf("连接数据库失败: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Default().Warn("close db failed", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close tunnel failed", zap.Error(err))
			}
		}
	}()

	return ai.ExecuteSQL(ctx, db, sqlText)
}

// ExecuteRedis 在指定 Redis 资产上执行命令
func (a *App) ExecuteRedis(assetID int64, command string, db int) (string, error) {
	asset, err := asset_svc.Asset().Get(a.langCtx(), assetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsRedis() {
		return "", fmt.Errorf("资产不是 Redis 类型")
	}
	cfg, err := asset.GetRedisConfig()
	if err != nil {
		return "", fmt.Errorf("获取 Redis 配置失败: %w", err)
	}
	cfg.Database = db
	password, err := credential_resolver.Default().ResolveRedisPassword(a.langCtx(), cfg)
	if err != nil {
		return "", fmt.Errorf("解析凭据失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 30*time.Second)
	defer cancel()

	client, tunnel, err := connpool.DialRedis(ctx, asset, cfg, password, a.sshPool)
	if err != nil {
		return "", fmt.Errorf("连接 Redis 失败: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			logger.Default().Warn("close redis client failed", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close tunnel failed", zap.Error(err))
			}
		}
	}()

	return ai.ExecuteRedis(ctx, client, command)
}

// ExecuteRedisArgs 使用预拆分的参数执行 Redis 命令（支持含空格的值）
func (a *App) ExecuteRedisArgs(assetID int64, args []string, db int) (string, error) {
	asset, err := asset_svc.Asset().Get(a.langCtx(), assetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsRedis() {
		return "", fmt.Errorf("资产不是 Redis 类型")
	}
	cfg, err := asset.GetRedisConfig()
	if err != nil {
		return "", fmt.Errorf("获取 Redis 配置失败: %w", err)
	}
	cfg.Database = db
	password, err := credential_resolver.Default().ResolveRedisPassword(a.langCtx(), cfg)
	if err != nil {
		return "", fmt.Errorf("解析凭据失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 30*time.Second)
	defer cancel()

	client, tunnel, err := connpool.DialRedis(ctx, asset, cfg, password, a.sshPool)
	if err != nil {
		return "", fmt.Errorf("连接 Redis 失败: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			logger.Default().Warn("close redis client failed", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close tunnel failed", zap.Error(err))
			}
		}
	}()

	return ai.ExecuteRedisRaw(ctx, client, args)
}
