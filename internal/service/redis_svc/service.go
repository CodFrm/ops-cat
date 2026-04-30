package redis_svc

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/opskat/opskat/internal/connpool"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/credential_resolver"
	"github.com/opskat/opskat/internal/sshpool"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

const defaultRedisValuePageSize = int64(100)

type redisExecutor interface {
	Do(ctx context.Context, args ...any) (any, error)
}

type Service struct {
	sshPool *sshpool.Pool
	history *CommandHistory
}

func New(sshPool *sshpool.Pool) *Service {
	return &Service{
		sshPool: sshPool,
		history: NewCommandHistory(200),
	}
}

func (s *Service) ListDatabases(ctx context.Context, assetID int64) ([]RedisDatabase, error) {
	var out []RedisDatabase
	err := s.withClient(ctx, assetID, -1, func(ctx context.Context, exec redisExecutor) error {
		var err error
		out, err = listDatabases(ctx, exec)
		return err
	})
	return out, err
}

func (s *Service) ScanKeys(ctx context.Context, req RedisScanRequest) (RedisScanResponse, error) {
	var out RedisScanResponse
	err := s.withClient(ctx, req.AssetID, req.DB, func(ctx context.Context, exec redisExecutor) error {
		var err error
		out, err = scanKeys(ctx, exec, req)
		return err
	})
	return out, err
}

func (s *Service) GetKeyDetail(ctx context.Context, req RedisKeyRequest) (RedisKeyDetail, error) {
	var out RedisKeyDetail
	err := s.withClient(ctx, req.AssetID, req.DB, func(ctx context.Context, exec redisExecutor) error {
		var err error
		out, err = getKeyDetail(ctx, exec, req)
		return err
	})
	return out, err
}

func (s *Service) SetKeyTTL(ctx context.Context, assetID int64, db int, key string, seconds int64) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return setKeyTTL(ctx, exec, key, seconds)
	})
}

func (s *Service) PersistKey(ctx context.Context, assetID int64, db int, key string) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return persistKey(ctx, exec, key)
	})
}

func (s *Service) RenameKey(ctx context.Context, assetID int64, db int, oldKey, newKey string) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return renameKey(ctx, exec, oldKey, newKey)
	})
}

func (s *Service) DeleteKeys(ctx context.Context, assetID int64, db int, keys []string) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return deleteKeys(ctx, exec, keys)
	})
}

func (s *Service) SetStringValue(ctx context.Context, req RedisStringSetRequest) error {
	return s.withClient(ctx, req.AssetID, req.DB, func(ctx context.Context, exec redisExecutor) error {
		return setStringValue(ctx, exec, req)
	})
}

func (s *Service) HashSet(ctx context.Context, assetID int64, db int, key, field, value string) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return hashSet(ctx, exec, key, field, value)
	})
}

func (s *Service) HashDelete(ctx context.Context, assetID int64, db int, key, field string) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return hashDelete(ctx, exec, key, field)
	})
}

func (s *Service) ListPush(ctx context.Context, assetID int64, db int, key, value string) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return listPush(ctx, exec, key, value)
	})
}

func (s *Service) ListSet(ctx context.Context, assetID int64, db int, key string, index int64, value string) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return listSet(ctx, exec, key, index, value)
	})
}

func (s *Service) ListDelete(ctx context.Context, assetID int64, db int, key string, index int64) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return listDelete(ctx, exec, key, index, "")
	})
}

func (s *Service) SetAdd(ctx context.Context, assetID int64, db int, key, member string) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return setAdd(ctx, exec, key, member)
	})
}

func (s *Service) SetRemove(ctx context.Context, assetID int64, db int, key, member string) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return setRemove(ctx, exec, key, member)
	})
}

func (s *Service) ZSetAdd(ctx context.Context, assetID int64, db int, key, member string, score float64) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return zsetAdd(ctx, exec, key, member, score)
	})
}

func (s *Service) ZSetRemove(ctx context.Context, assetID int64, db int, key, member string) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return zsetRemove(ctx, exec, key, member)
	})
}

func (s *Service) StreamAdd(ctx context.Context, assetID int64, db int, key, id string, fields []RedisStreamField) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return streamAdd(ctx, exec, key, id, fields)
	})
}

func (s *Service) StreamDelete(ctx context.Context, assetID int64, db int, key string, ids []string) error {
	return s.withClient(ctx, assetID, db, func(ctx context.Context, exec redisExecutor) error {
		return streamDelete(ctx, exec, key, ids)
	})
}

func (s *Service) withClient(ctx context.Context, assetID int64, db int, fn func(context.Context, redisExecutor) error) error {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsRedis() {
		return fmt.Errorf("资产不是 Redis 类型")
	}
	cfg, err := asset.GetRedisConfig()
	if err != nil {
		return fmt.Errorf("获取 Redis 配置失败: %w", err)
	}
	if db >= 0 {
		cfg.Database = db
	}
	password, err := credential_resolver.Default().ResolveRedisPassword(ctx, cfg)
	if err != nil {
		return fmt.Errorf("解析 Redis 凭据失败: %w", err)
	}
	var opCtx context.Context
	var cancel context.CancelFunc
	if cfg.CommandTimeoutSeconds > 0 {
		opCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.CommandTimeoutSeconds)*time.Second)
	} else {
		opCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
	}
	defer cancel()
	client, closer, err := connpool.DialRedis(opCtx, asset, cfg, password, s.sshPool)
	if err != nil {
		return fmt.Errorf("连接 Redis 失败: %w", err)
	}
	defer closeRedisClient(client, closer)
	return fn(opCtx, &goRedisExecutor{client: client, history: s.history, assetID: assetID, db: cfg.Database})
}

func closeRedisClient(client *redis.Client, closer io.Closer) {
	if client != nil {
		if err := client.Close(); err != nil {
			logger.Default().Warn("close redis client failed", zap.Error(err))
		}
	}
	if closer != nil {
		if err := closer.Close(); err != nil {
			logger.Default().Warn("close redis tunnel failed", zap.Error(err))
		}
	}
}

type goRedisExecutor struct {
	client  *redis.Client
	history *CommandHistory
	assetID int64
	db      int
}

func (e *goRedisExecutor) Do(ctx context.Context, args ...any) (any, error) {
	start := time.Now()
	result, err := e.client.Do(ctx, args...).Result()
	if err == redis.Nil {
		err = nil
	}
	if e.history != nil {
		e.history.Add(CommandHistoryEntry{
			AssetID:    e.assetID,
			DB:         e.db,
			Command:    formatCommandForHistory(args),
			CostMillis: time.Since(start).Milliseconds(),
			Error:      errorString(err),
			Timestamp:  time.Now().UnixMilli(),
		})
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func listDatabases(ctx context.Context, exec redisExecutor) ([]RedisDatabase, error) {
	result, err := exec.Do(ctx, "INFO", "keyspace")
	if err != nil {
		return nil, fmt.Errorf("load Redis keyspace info: %w", err)
	}
	return ParseKeyspaceInfo(fmt.Sprint(result)), nil
}

func scanKeys(ctx context.Context, exec redisExecutor, req RedisScanRequest) (RedisScanResponse, error) {
	req = NormalizeScanOptions(req)
	if req.Exact && req.Match != "*" {
		return scanExactKey(ctx, exec, req)
	}

	cursor := req.Cursor
	keys := make([]string, 0)
	scanCount := req.Count
	filteredScan := req.Match != "*" || req.Type != ""
	for {
		args := []any{"SCAN", cursor, "MATCH", req.Match, "COUNT", scanCount}
		if req.Type != "" {
			args = append(args, "TYPE", req.Type)
		}
		result, err := exec.Do(ctx, args...)
		if err != nil {
			return RedisScanResponse{}, fmt.Errorf("scan Redis keys: %w", err)
		}
		nextCursor, nextKeys, err := parseScanResult(result)
		if err != nil {
			return RedisScanResponse{}, err
		}
		cursor = nextCursor
		keys = append(keys, nextKeys...)
		if !filteredScan || cursor == "0" || int64(len(keys)) >= req.Count {
			break
		}
	}
	return RedisScanResponse{Cursor: cursor, Keys: keys, HasMore: cursor != "0"}, nil
}

func scanExactKey(ctx context.Context, exec redisExecutor, req RedisScanRequest) (RedisScanResponse, error) {
	exists, err := exec.Do(ctx, "EXISTS", req.Match)
	if err != nil {
		return RedisScanResponse{}, fmt.Errorf("check Redis key existence: %w", err)
	}
	if toInt64(exists) == 0 {
		return RedisScanResponse{Cursor: "0"}, nil
	}
	if req.Type != "" {
		keyType, err := exec.Do(ctx, "TYPE", req.Match)
		if err != nil {
			return RedisScanResponse{}, fmt.Errorf("check Redis key type: %w", err)
		}
		if strings.ToLower(fmt.Sprint(keyType)) != req.Type {
			return RedisScanResponse{Cursor: "0"}, nil
		}
	}
	return RedisScanResponse{Cursor: "0", Keys: []string{req.Match}}, nil
}

func parseScanResult(result any) (string, []string, error) {
	items, ok := result.([]any)
	if !ok || len(items) < 2 {
		return "", nil, fmt.Errorf("unexpected Redis SCAN result: %T", result)
	}
	cursor := fmt.Sprint(items[0])
	keys := toStringSlice(items[1])
	return cursor, keys, nil
}

func getKeyDetail(ctx context.Context, exec redisExecutor, req RedisKeyRequest) (RedisKeyDetail, error) {
	count := req.Count
	if count <= 0 {
		count = defaultRedisValuePageSize
	}
	cursor := strings.TrimSpace(req.Cursor)
	if cursor == "" {
		cursor = "0"
	}
	keyType, err := exec.Do(ctx, "TYPE", req.Key)
	if err != nil {
		return RedisKeyDetail{}, fmt.Errorf("load Redis key type: %w", err)
	}
	detail := RedisKeyDetail{Key: req.Key, Type: strings.ToLower(fmt.Sprint(keyType)), TTL: -1, Total: -1}
	if detail.Type == "none" {
		return detail, nil
	}
	if ttl, err := exec.Do(ctx, "TTL", req.Key); err == nil {
		detail.TTL = toInt64(ttl)
	}
	if size, err := exec.Do(ctx, "MEMORY", "USAGE", req.Key); err == nil {
		detail.Size = toInt64(size)
	}
	switch detail.Type {
	case "string":
		value, err := exec.Do(ctx, "GET", req.Key)
		if err != nil {
			return RedisKeyDetail{}, fmt.Errorf("load Redis string value: %w", err)
		}
		detail.Value = fmt.Sprint(value)
	case "hash":
		total, err := exec.Do(ctx, "HLEN", req.Key)
		if err != nil {
			return RedisKeyDetail{}, fmt.Errorf("load Redis hash length: %w", err)
		}
		detail.Total = toInt64(total)
		value, err := exec.Do(ctx, "HSCAN", req.Key, cursor, "COUNT", count)
		if err != nil {
			return RedisKeyDetail{}, fmt.Errorf("load Redis hash values: %w", err)
		}
		nextCursor, flat, err := parseScanResult(value)
		if err != nil {
			return RedisKeyDetail{}, err
		}
		detail.ValueCursor = nextCursor
		detail.HasMoreValues = nextCursor != "0"
		detail.Value = toHashEntries(flat)
	case "list":
		total, err := exec.Do(ctx, "LLEN", req.Key)
		if err != nil {
			return RedisKeyDetail{}, fmt.Errorf("load Redis list length: %w", err)
		}
		detail.Total = toInt64(total)
		value, err := exec.Do(ctx, "LRANGE", req.Key, req.Offset, req.Offset+count-1)
		if err != nil {
			return RedisKeyDetail{}, fmt.Errorf("load Redis list values: %w", err)
		}
		items := toStringSlice(value)
		detail.Value = items
		detail.ValueOffset = req.Offset + int64(len(items))
		detail.HasMoreValues = detail.ValueOffset < detail.Total
	case "set":
		total, err := exec.Do(ctx, "SCARD", req.Key)
		if err != nil {
			return RedisKeyDetail{}, fmt.Errorf("load Redis set size: %w", err)
		}
		detail.Total = toInt64(total)
		value, err := exec.Do(ctx, "SSCAN", req.Key, cursor, "COUNT", count)
		if err != nil {
			return RedisKeyDetail{}, fmt.Errorf("load Redis set values: %w", err)
		}
		nextCursor, items, err := parseScanResult(value)
		if err != nil {
			return RedisKeyDetail{}, err
		}
		detail.ValueCursor = nextCursor
		detail.HasMoreValues = nextCursor != "0"
		detail.Value = items
	case "zset":
		total, err := exec.Do(ctx, "ZCARD", req.Key)
		if err != nil {
			return RedisKeyDetail{}, fmt.Errorf("load Redis zset size: %w", err)
		}
		detail.Total = toInt64(total)
		value, err := exec.Do(ctx, "ZRANGE", req.Key, req.Offset, req.Offset+count-1, "WITHSCORES")
		if err != nil {
			return RedisKeyDetail{}, fmt.Errorf("load Redis zset values: %w", err)
		}
		entries := toZSetEntries(toStringSlice(value))
		detail.Value = entries
		detail.ValueOffset = req.Offset + int64(len(entries))
		detail.HasMoreValues = detail.ValueOffset < detail.Total
	case "stream":
		total, err := exec.Do(ctx, "XLEN", req.Key)
		if err != nil {
			return RedisKeyDetail{}, fmt.Errorf("load Redis stream length: %w", err)
		}
		detail.Total = toInt64(total)
		start := "-"
		queryCount := count
		if cursor != "0" {
			start = cursor
			queryCount = count + 1
		}
		value, err := exec.Do(ctx, "XRANGE", req.Key, start, "+", "COUNT", queryCount)
		if err != nil {
			return RedisKeyDetail{}, fmt.Errorf("load Redis stream values: %w", err)
		}
		entries := toStreamEntries(value)
		if cursor != "0" && len(entries) > 0 && entries[0].ID == cursor {
			entries = entries[1:]
		}
		detail.Value = entries
		detail.ValueOffset = req.Offset + int64(len(entries))
		detail.HasMoreValues = detail.ValueOffset < detail.Total
		if len(entries) > 0 {
			detail.ValueCursor = entries[len(entries)-1].ID
		}
	default:
		value, err := exec.Do(ctx, "GET", req.Key)
		if err == nil {
			detail.Value = fmt.Sprint(value)
		}
	}
	return detail, nil
}

func toHashEntries(flat []string) []RedisHashEntry {
	entries := make([]RedisHashEntry, 0, len(flat)/2)
	for i := 0; i < len(flat); i += 2 {
		value := ""
		if i+1 < len(flat) {
			value = flat[i+1]
		}
		entries = append(entries, RedisHashEntry{Field: flat[i], Value: value})
	}
	return entries
}

func toZSetEntries(flat []string) []RedisZSetEntry {
	entries := make([]RedisZSetEntry, 0, len(flat)/2)
	for i := 0; i < len(flat); i += 2 {
		score := 0.0
		if i+1 < len(flat) {
			score, _ = strconv.ParseFloat(flat[i+1], 64)
		}
		entries = append(entries, RedisZSetEntry{Member: flat[i], Score: score})
	}
	return entries
}

func toStreamEntries(raw any) []RedisStreamEntry {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	entries := make([]RedisStreamEntry, 0, len(items))
	for _, item := range items {
		pair, ok := item.([]any)
		if !ok || len(pair) < 2 {
			continue
		}
		fields := map[string]string{}
		flat := toStringSlice(pair[1])
		for i := 0; i < len(flat); i += 2 {
			value := ""
			if i+1 < len(flat) {
				value = flat[i+1]
			}
			fields[flat[i]] = value
		}
		entries = append(entries, RedisStreamEntry{ID: fmt.Sprint(pair[0]), Fields: fields})
	}
	return entries
}

func toStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return nil
	}
}

func toInt64(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case uint64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	default:
		return 0
	}
}
