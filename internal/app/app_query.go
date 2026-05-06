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
	"github.com/opskat/opskat/internal/service/query_svc"

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

	ctx, cancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer cancel()

	password := plainPassword
	if password == "" {
		var err error
		password, err = credential_resolver.Default().ResolveDatabasePassword(ctx, &cfg)
		if err != nil {
			return fmt.Errorf("连接失败: %w", err)
		}
	}

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

	ctx, cancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer cancel()

	password := plainPassword
	if password == "" {
		var err error
		password, err = credential_resolver.Default().ResolveRedisPassword(ctx, &cfg)
		if err != nil {
			return fmt.Errorf("连接失败: %w", err)
		}
	}

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

// ExecuteTableImport executes a prepared table import batch on one database session.
func (a *App) ExecuteTableImport(
	assetID int64,
	database string,
	request query_svc.TableImportBatchRequest,
) (*query_svc.TableImportBatchResult, error) {
	asset, err := asset_svc.Asset().Get(a.langCtx(), assetID)
	if err != nil {
		return nil, fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsDatabase() {
		return nil, fmt.Errorf("资产不是数据库类型")
	}
	cfg, err := asset.GetDatabaseConfig()
	if err != nil {
		return nil, fmt.Errorf("获取数据库配置失败: %w", err)
	}
	if database != "" {
		cfg.Database = database
	}
	password, err := credential_resolver.Default().ResolveDatabasePassword(a.langCtx(), cfg)
	if err != nil {
		return nil, fmt.Errorf("解析凭据失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 30*time.Minute)
	defer cancel()

	db, tunnel, err := connpool.DialDatabase(ctx, asset, cfg, password, a.sshPool)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
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

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("打开数据库会话失败: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Default().Warn("close db session failed", zap.Error(err))
		}
	}()

	return query_svc.RunTableImportBatch(ctx, query_svc.NewSQLSession(conn), cfg.Driver, request)
}

// ExecuteSQLPaged 在指定数据库资产上执行分页 SQL 查询（SELECT/WITH 子查询包装）
func (a *App) ExecuteSQLPaged(assetID int64, sqlText string, database string, page int, pageSize int) (string, error) {
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

	return ai.ExecuteSQLPaged(ctx, db, sqlText, page, pageSize)
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

// TestMongoDBConnection 测试 MongoDB 连接
// configJSON: MongoDBConfig JSON，plainPassword: 明文密码
func (a *App) TestMongoDBConnection(configJSON string, plainPassword string) error {
	var cfg asset_entity.MongoDBConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer cancel()

	password := plainPassword
	if password == "" {
		var err error
		password, err = credential_resolver.Default().ResolveMongoDBPassword(ctx, &cfg)
		if err != nil {
			return fmt.Errorf("连接失败: %w", err)
		}
	}

	// 测试连接场景没有持久化的 Asset，使用零值让 backward compat 生效
	testAsset := &asset_entity.Asset{}
	client, tunnel, err := connpool.DialMongoDB(ctx, testAsset, &cfg, password, a.sshPool)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			logger.Default().Warn("disconnect mongodb client failed", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close tunnel failed", zap.Error(err))
			}
		}
	}()
	return nil
}

// ExecuteMongo 在指定 MongoDB 资产上执行操作
func (a *App) ExecuteMongo(assetID int64, operation, database, collection, query string) (string, error) {
	asset, err := asset_svc.Asset().Get(a.langCtx(), assetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsMongoDB() {
		return "", fmt.Errorf("资产不是 MongoDB 类型")
	}
	cfg, err := asset.GetMongoDBConfig()
	if err != nil {
		return "", fmt.Errorf("获取 MongoDB 配置失败: %w", err)
	}
	password, err := credential_resolver.Default().ResolveMongoDBPassword(a.langCtx(), cfg)
	if err != nil {
		return "", fmt.Errorf("解析凭据失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 30*time.Second)
	defer cancel()

	client, tunnel, err := connpool.DialMongoDB(ctx, asset, cfg, password, a.sshPool)
	if err != nil {
		return "", fmt.Errorf("连接 MongoDB 失败: %w", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			logger.Default().Warn("disconnect mongodb client failed", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close tunnel failed", zap.Error(err))
			}
		}
	}()

	return ai.ExecuteMongoDB(ctx, client, database, collection, operation, query)
}

// ListMongoDatabases 列出指定 MongoDB 资产的所有数据库
func (a *App) ListMongoDatabases(assetID int64) (string, error) {
	asset, err := asset_svc.Asset().Get(a.langCtx(), assetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsMongoDB() {
		return "", fmt.Errorf("资产不是 MongoDB 类型")
	}
	cfg, err := asset.GetMongoDBConfig()
	if err != nil {
		return "", fmt.Errorf("获取 MongoDB 配置失败: %w", err)
	}
	password, err := credential_resolver.Default().ResolveMongoDBPassword(a.langCtx(), cfg)
	if err != nil {
		return "", fmt.Errorf("解析凭据失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer cancel()

	client, tunnel, err := connpool.DialMongoDB(ctx, asset, cfg, password, a.sshPool)
	if err != nil {
		return "", fmt.Errorf("连接 MongoDB 失败: %w", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			logger.Default().Warn("disconnect mongodb client failed", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close tunnel failed", zap.Error(err))
			}
		}
	}()

	names, err := ai.ListMongoDatabases(ctx, client)
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(names)
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %w", err)
	}
	return string(result), nil
}

// ListMongoCollections 列出指定 MongoDB 资产中某个数据库的所有集合
func (a *App) ListMongoCollections(assetID int64, database string) (string, error) {
	asset, err := asset_svc.Asset().Get(a.langCtx(), assetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsMongoDB() {
		return "", fmt.Errorf("资产不是 MongoDB 类型")
	}
	cfg, err := asset.GetMongoDBConfig()
	if err != nil {
		return "", fmt.Errorf("获取 MongoDB 配置失败: %w", err)
	}
	password, err := credential_resolver.Default().ResolveMongoDBPassword(a.langCtx(), cfg)
	if err != nil {
		return "", fmt.Errorf("解析凭据失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.langCtx(), 10*time.Second)
	defer cancel()

	client, tunnel, err := connpool.DialMongoDB(ctx, asset, cfg, password, a.sshPool)
	if err != nil {
		return "", fmt.Errorf("连接 MongoDB 失败: %w", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			logger.Default().Warn("disconnect mongodb client failed", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close tunnel failed", zap.Error(err))
			}
		}
	}()

	names, err := ai.ListMongoCollections(ctx, client, database)
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(names)
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %w", err)
	}
	return string(result), nil
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
