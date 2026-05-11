package ai

import (
	"context"
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

	"github.com/redis/go-redis/v9"
)

// --- Redis 连接缓存 ---

type redisCacheKeyType struct{}

// RedisClientCache 在同一次 AI Chat 中复用 Redis 连接
type RedisClientCache = ConnCache[*redis.Client]


func getRedisCache(ctx context.Context) *RedisClientCache {
	if cache, ok := ctx.Value(redisCacheKeyType{}).(*RedisClientCache); ok {
		return cache
	}
	return nil
}

// --- Handler ---

func handleExecRedis(ctx context.Context, args map[string]any) (string, error) {
	assetID := argInt64(args, "asset_id")
	command := argString(args, "command")
	if assetID == 0 || command == "" {
		return "", fmt.Errorf("missing required parameters: asset_id, command")
	}

	// 权限/审批由 aiagent.policyHook 在 PreToolUse 阶段统一处理（走 gw.RequestSingle，
	// emitter 闭包绑定的是正确 convID）。这里再做一次 checker.Check 会触发 legacy
	// makeCommandConfirmFunc 弹第二张卡 —— 这条 in-handler 防御从 cago 迁移之后
	// 就只服务 AI 路径，opsctl 不注 PolicyChecker，删掉零回归。

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("asset not found: %w", err)
	}
	if !asset.IsRedis() {
		return "", fmt.Errorf("asset is not Redis type")
	}
	cfg, err := asset.GetRedisConfig()
	if err != nil {
		return "", fmt.Errorf("failed to get Redis config: %w", err)
	}

	// 覆盖默认数据库
	if _, ok := args["db"]; ok {
		cfg.Database = int(argInt64(args, "db"))
	}

	client, closer, err := getOrDialRedis(ctx, asset, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Redis: %w", err)
	}
	if getRedisCache(ctx) == nil {
		if client != nil {
			defer func() {
				if err := client.Close(); err != nil {
					logger.Default().Warn("close Redis connection", zap.Error(err))
				}
			}()
		}
		if closer != nil {
			defer func() {
				if err := closer.Close(); err != nil {
					logger.Default().Warn("close Redis tunnel", zap.Error(err))
				}
			}()
		}
	}

	return ExecuteRedis(ctx, client, command)
}

func getOrDialRedis(ctx context.Context, asset *asset_entity.Asset, cfg *asset_entity.RedisConfig) (*redis.Client, io.Closer, error) {
	dialFn := func() (*redis.Client, io.Closer, error) {
		password, err := credential_resolver.Default().ResolveRedisPassword(ctx, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to resolve credentials: %w", err)
		}
		return connpool.DialRedis(ctx, asset, cfg, password, getSSHPool(ctx))
	}
	if cache := getRedisCache(ctx); cache != nil {
		return cache.GetOrDial(asset.ID, dialFn)
	}
	return dialFn()
}

// ExecuteRedis 执行 Redis 命令并返回 JSON 结果
func ExecuteRedis(ctx context.Context, client *redis.Client, command string) (string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", fmt.Errorf("redis command is empty")
	}

	// SELECT 命令在连接池模式下无效，必须通过 db 参数指定数据库
	if strings.EqualFold(parts[0], "SELECT") {
		return "", fmt.Errorf("SELECT command is not supported due to connection pooling. Use the 'db' parameter to specify the database number")
	}

	redisArgs := make([]any, len(parts))
	for i, p := range parts {
		redisArgs[i] = p
	}

	result, err := client.Do(ctx, redisArgs...).Result()
	if err != nil {
		if err == redis.Nil {
			return `{"type":"nil","value":null}`, nil
		}
		return "", fmt.Errorf("redis command failed: %w", err)
	}

	return formatRedisResult(result)
}

// ExecuteRedisRaw 使用预拆分的参数执行 Redis 命令（支持含空格的值）
func ExecuteRedisRaw(ctx context.Context, client *redis.Client, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("redis command is empty")
	}

	// SELECT 命令在连接池模式下无效，必须通过 db 参数指定数据库
	if strings.EqualFold(args[0], "SELECT") {
		return "", fmt.Errorf("SELECT command is not supported due to connection pooling. Use the 'db' parameter to specify the database number")
	}

	redisArgs := make([]any, len(args))
	for i, p := range args {
		redisArgs[i] = p
	}

	result, err := client.Do(ctx, redisArgs...).Result()
	if err != nil {
		if err == redis.Nil {
			return `{"type":"nil","value":null}`, nil
		}
		return "", fmt.Errorf("redis command failed: %w", err)
	}

	return formatRedisResult(result)
}

func formatRedisResult(result any) (string, error) {
	var out map[string]any
	switch v := result.(type) {
	case string:
		out = map[string]any{"type": "string", "value": v}
	case int64:
		out = map[string]any{"type": "integer", "value": v}
	case []any:
		out = map[string]any{"type": "list", "value": v}
	case map[any]any:
		// Redis hash result
		m := make(map[string]any, len(v))
		for k, val := range v {
			m[fmt.Sprint(k)] = val
		}
		out = map[string]any{"type": "hash", "value": m}
	case nil:
		out = map[string]any{"type": "nil", "value": nil}
	default:
		out = map[string]any{"type": fmt.Sprintf("%T", v), "value": fmt.Sprint(v)}
	}
	data, err := json.Marshal(out)
	if err != nil {
		logger.Default().Error("marshal redis result", zap.Error(err))
		return "", fmt.Errorf("failed to marshal Redis result: %w", err)
	}
	return string(data), nil
}
