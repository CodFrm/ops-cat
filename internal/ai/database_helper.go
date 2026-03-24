package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/connpool"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/sshpool"
)

// --- Database 连接缓存 ---

type dbCacheKeyType struct{}

// DatabaseClientCache 在同一次 AI Chat 中复用数据库连接
type DatabaseClientCache struct {
	clients map[int64]*sql.DB
	closers map[int64]io.Closer
}

// NewDatabaseClientCache 创建数据库连接缓存
func NewDatabaseClientCache() *DatabaseClientCache {
	return &DatabaseClientCache{
		clients: make(map[int64]*sql.DB),
		closers: make(map[int64]io.Closer),
	}
}

// Close 关闭所有缓存的数据库连接
func (c *DatabaseClientCache) Close() error {
	for id, db := range c.clients {
		if err := db.Close(); err != nil {
			logger.Default().Warn("close cached database connection", zap.Int64("assetID", id), zap.Error(err))
		}
		delete(c.clients, id)
	}
	for id, closer := range c.closers {
		if closer != nil {
			if err := closer.Close(); err != nil {
				logger.Default().Warn("close database tunnel", zap.Int64("assetID", id), zap.Error(err))
			}
		}
		delete(c.closers, id)
	}
	return nil
}

// WithDatabaseCache 将数据库缓存注入 context
func WithDatabaseCache(ctx context.Context, cache *DatabaseClientCache) context.Context {
	return context.WithValue(ctx, dbCacheKeyType{}, cache)
}

func getDatabaseCache(ctx context.Context) *DatabaseClientCache {
	if cache, ok := ctx.Value(dbCacheKeyType{}).(*DatabaseClientCache); ok {
		return cache
	}
	return nil
}

// --- SSH Pool context ---

type sshPoolKeyType struct{}

// WithSSHPool 将 SSH 连接池注入 context（供 connpool 隧道使用）
func WithSSHPool(ctx context.Context, pool *sshpool.Pool) context.Context {
	return context.WithValue(ctx, sshPoolKeyType{}, pool)
}

func getSSHPool(ctx context.Context) *sshpool.Pool {
	if pool, ok := ctx.Value(sshPoolKeyType{}).(*sshpool.Pool); ok {
		return pool
	}
	return nil
}

// --- Handler ---

func handleExecSQL(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	sqlText := argString(args, "sql")
	if assetID == 0 || sqlText == "" {
		return "", fmt.Errorf("缺少必要参数 (asset_id, sql)")
	}

	// 权限检查
	if checker := GetPolicyChecker(ctx); checker != nil {
		result := checker.CheckForAsset(ctx, assetID, asset_entity.AssetTypeDatabase, sqlText)
		setCheckResult(ctx, result)
		if result.Decision != Allow {
			return result.Message, nil
		}
	}

	asset, err := asset_svc.Asset().Get(ctx, assetID)
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

	// 覆盖默认数据库
	if dbOverride := argString(args, "database"); dbOverride != "" {
		cfg.Database = dbOverride
	}

	db, closer, err := getOrDialDatabase(ctx, assetID, cfg)
	if err != nil {
		return "", fmt.Errorf("连接数据库失败: %w", err)
	}
	// 如果不是缓存连接，使用后关闭
	if getDatabaseCache(ctx) == nil {
		if db != nil {
			defer func() {
				if err := db.Close(); err != nil {
					logger.Default().Warn("close database connection", zap.Error(err))
				}
			}()
		}
		if closer != nil {
			defer func() {
				if err := closer.Close(); err != nil {
					logger.Default().Warn("close database tunnel", zap.Error(err))
				}
			}()
		}
	}

	return ExecuteSQL(ctx, db, sqlText)
}

func getOrDialDatabase(ctx context.Context, assetID int64, cfg *asset_entity.DatabaseConfig) (*sql.DB, io.Closer, error) {
	if cache := getDatabaseCache(ctx); cache != nil {
		if db, ok := cache.clients[assetID]; ok {
			return db, nil, nil
		}
		db, closer, err := connpool.DialDatabase(ctx, cfg, getSSHPool(ctx))
		if err != nil {
			return nil, nil, err
		}
		cache.clients[assetID] = db
		cache.closers[assetID] = closer
		return db, nil, nil
	}
	return connpool.DialDatabase(ctx, cfg, getSSHPool(ctx))
}

// ExecuteSQL 执行 SQL 并返回 JSON 结果
func ExecuteSQL(ctx context.Context, db *sql.DB, sqlText string) (string, error) {
	trimmed := strings.TrimSpace(strings.ToUpper(sqlText))
	if isQueryStatement(trimmed) {
		rows, err := db.QueryContext(ctx, sqlText)
		if err != nil {
			return "", fmt.Errorf("SQL 查询失败: %w", err)
		}
		defer func() {
			if err := rows.Close(); err != nil {
				logger.Default().Warn("close SQL rows", zap.Error(err))
			}
		}()
		return formatRowsJSON(rows)
	}

	result, err := db.ExecContext(ctx, sqlText)
	if err != nil {
		return "", fmt.Errorf("SQL 执行失败: %w", err)
	}
	affected, _ := result.RowsAffected()
	return fmt.Sprintf(`{"affected_rows":%d}`, affected), nil
}

func isQueryStatement(upper string) bool {
	return strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "DESCRIBE") ||
		strings.HasPrefix(upper, "DESC ") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "WITH") // CTE
}

func formatRowsJSON(rows *sql.Rows) (string, error) {
	columns, err := rows.Columns()
	if err != nil {
		return "", err
	}

	var resultRows []map[string]any
	for rows.Next() {
		values := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		row := make(map[string]any, len(columns))
		for i, col := range columns {
			val := values[i]
			// 将 []byte 转为 string
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			row[col] = val
		}
		resultRows = append(resultRows, row)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	data, err := json.Marshal(map[string]any{
		"columns": columns,
		"rows":    resultRows,
		"count":   len(resultRows),
	})
	if err != nil {
		logger.Default().Error("marshal query result", zap.Error(err))
		return "", fmt.Errorf("序列化查询结果失败: %w", err)
	}
	return string(data), nil
}
