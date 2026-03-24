package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ops-cat/internal/connpool"
	"ops-cat/internal/model/entity/asset_entity"
	"ops-cat/internal/service/asset_svc"

	"github.com/redis/go-redis/v9"
)

// --- Redis 连接缓存 ---

type redisCacheKeyType struct{}

// RedisClientCache 在同一次 AI Chat 中复用 Redis 连接
type RedisClientCache struct {
	clients map[int64]*redis.Client
	closers map[int64]io.Closer
}

// NewRedisClientCache 创建 Redis 连接缓存
func NewRedisClientCache() *RedisClientCache {
	return &RedisClientCache{
		clients: make(map[int64]*redis.Client),
		closers: make(map[int64]io.Closer),
	}
}

// Close 关闭所有缓存的 Redis 连接
func (c *RedisClientCache) Close() error {
	for id, client := range c.clients {
		_ = client.Close()
		delete(c.clients, id)
	}
	for id, closer := range c.closers {
		if closer != nil {
			_ = closer.Close()
		}
		delete(c.closers, id)
	}
	return nil
}

// WithRedisCache 将 Redis 缓存注入 context
func WithRedisCache(ctx context.Context, cache *RedisClientCache) context.Context {
	return context.WithValue(ctx, redisCacheKeyType{}, cache)
}

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
		return "", fmt.Errorf("缺少必要参数 (asset_id, command)")
	}

	// 权限检查
	if checker := GetPolicyChecker(ctx); checker != nil {
		result := checker.CheckForAsset(ctx, assetID, asset_entity.AssetTypeRedis, command)
		if result.Decision != Allow {
			return result.Message, nil
		}
	}

	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsRedis() {
		return "", fmt.Errorf("资产不是Redis类型")
	}
	cfg, err := asset.GetRedisConfig()
	if err != nil {
		return "", fmt.Errorf("获取Redis配置失败: %w", err)
	}

	client, closer, err := getOrDialRedis(ctx, assetID, cfg)
	if err != nil {
		return "", fmt.Errorf("连接Redis失败: %w", err)
	}
	if getRedisCache(ctx) == nil {
		if client != nil {
			defer func() { _ = client.Close() }()
		}
		if closer != nil {
			defer func() { _ = closer.Close() }()
		}
	}

	return ExecuteRedis(ctx, client, command)
}

func getOrDialRedis(ctx context.Context, assetID int64, cfg *asset_entity.RedisConfig) (*redis.Client, io.Closer, error) {
	if cache := getRedisCache(ctx); cache != nil {
		if client, ok := cache.clients[assetID]; ok {
			return client, nil, nil
		}
		client, closer, err := connpool.DialRedis(ctx, cfg, getSSHPool(ctx))
		if err != nil {
			return nil, nil, err
		}
		cache.clients[assetID] = client
		cache.closers[assetID] = closer
		return client, nil, nil
	}
	return connpool.DialRedis(ctx, cfg, getSSHPool(ctx))
}

// ExecuteRedis 执行 Redis 命令并返回 JSON 结果
func ExecuteRedis(ctx context.Context, client *redis.Client, command string) (string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", fmt.Errorf("Redis 命令为空")
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
		return "", fmt.Errorf("Redis 命令执行失败: %w", err)
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
	data, _ := json.Marshal(out)
	return string(data), nil
}
