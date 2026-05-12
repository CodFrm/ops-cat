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
	"github.com/opskat/opskat/internal/service/credential_resolver"
	"github.com/opskat/opskat/internal/sshpool"
)

// --- Database 连接缓存 ---

type dbCacheKeyType struct{}

// DatabaseClientCache 在同一次 AI Chat 中复用数据库连接
type DatabaseClientCache = ConnCache[*sql.DB]

// NewDatabaseClientCache 创建数据库连接缓存
func NewDatabaseClientCache() *DatabaseClientCache {
	return NewConnCache[*sql.DB]("database")
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
		return "", fmt.Errorf("missing required parameters: asset_id, sql")
	}

	// 权限检查
	if checker := GetPolicyChecker(ctx); checker != nil {
		result := checker.CheckForAsset(ctx, assetID, asset_entity.AssetTypeDatabase, sqlText)
		RecordDecision(ctx, result)
		if result.Decision != Allow {
			return result.Message, nil
		}
	}

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("asset not found: %w", err)
	}
	if !asset.IsDatabase() {
		return "", fmt.Errorf("asset is not database type")
	}
	cfg, err := asset.GetDatabaseConfig()
	if err != nil {
		return "", fmt.Errorf("failed to get database config: %w", err)
	}

	// 覆盖默认数据库
	if dbOverride := argString(args, "database"); dbOverride != "" {
		cfg.Database = dbOverride
	}

	db, closer, err := getOrDialDatabase(ctx, asset, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to connect to database: %w", err)
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

func getOrDialDatabase(ctx context.Context, asset *asset_entity.Asset, cfg *asset_entity.DatabaseConfig) (*sql.DB, io.Closer, error) {
	dialFn := func() (*sql.DB, io.Closer, error) {
		password, err := credential_resolver.Default().ResolveDatabasePassword(ctx, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to resolve credentials: %w", err)
		}
		return connpool.DialDatabase(ctx, asset, cfg, password, getSSHPool(ctx))
	}
	if cache := getDatabaseCache(ctx); cache != nil {
		return cache.GetOrDial(asset.ID, dialFn)
	}
	return dialFn()
}

// ExecuteSQL 执行 SQL 并返回 JSON 结果
func ExecuteSQL(ctx context.Context, db *sql.DB, sqlText string) (string, error) {
	trimmed := strings.TrimSpace(strings.ToUpper(sqlText))
	if isQueryStatement(trimmed) {
		rows, err := db.QueryContext(ctx, sqlText)
		if err != nil {
			return "", fmt.Errorf("SQL query failed: %w", err)
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
		return "", fmt.Errorf("SQL execution failed: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Default().Warn("get rows affected", zap.Error(err))
	}
	return fmt.Sprintf(`{"affected_rows":%d}`, affected), nil
}

// isPageableQuery 判断是否可以用子查询包装分页（仅 SELECT / WITH）
func isPageableQuery(upper string) bool {
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
}

// stripTrailingSemicolon 去掉末尾分号，避免子查询包装出错
func stripTrailingSemicolon(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), ";")
}

// ExecuteSQLPaged 对 SELECT/WITH 语句进行子查询包装分页，其他语句走 ExecuteSQL
func ExecuteSQLPaged(ctx context.Context, db *sql.DB, sqlText string, page, pageSize int) (string, error) {
	trimmed := strings.TrimSpace(strings.ToUpper(sqlText))

	// 非查询语句或不可分页的查询（SHOW/DESCRIBE/EXPLAIN）走原逻辑
	if !isQueryStatement(trimmed) || !isPageableQuery(trimmed) {
		return ExecuteSQL(ctx, db, sqlText)
	}

	cleanSQL := stripTrailingSemicolon(sqlText)
	offset := page * pageSize

	// 1. 获取总行数
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS _t", cleanSQL) //nolint:gosec // SQL is user-provided and intentionally executed
	var totalCount int
	if err := db.QueryRowContext(ctx, countSQL).Scan(&totalCount); err != nil {
		return "", fmt.Errorf("count query failed: %w", err)
	}

	// 2. 分页查询
	pagedSQL := fmt.Sprintf("SELECT * FROM (%s) AS _t LIMIT %d OFFSET %d", cleanSQL, pageSize, offset) //nolint:gosec // SQL is user-provided and intentionally executed
	rows, err := db.QueryContext(ctx, pagedSQL)
	if err != nil {
		return "", fmt.Errorf("SQL query failed: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			logger.Default().Warn("close SQL rows", zap.Error(err))
		}
	}()

	return formatRowsPagedJSON(rows, totalCount)
}

func formatRowsPagedJSON(rows *sql.Rows, totalCount int) (string, error) {
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
		"columns":     columns,
		"rows":        resultRows,
		"count":       len(resultRows),
		"total_count": totalCount,
	})
	if err != nil {
		logger.Default().Error("marshal query result", zap.Error(err))
		return "", fmt.Errorf("failed to marshal query result: %w", err)
	}
	return string(data), nil
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
		return "", fmt.Errorf("failed to marshal query result: %w", err)
	}
	return string(data), nil
}
