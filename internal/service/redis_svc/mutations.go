package redis_svc

import (
	"context"
	"fmt"
	"time"
)

func setKeyTTL(ctx context.Context, exec redisExecutor, key string, seconds int64) error {
	if seconds <= 0 {
		return fmt.Errorf("ttl must be greater than zero")
	}
	_, err := exec.Do(ctx, "EXPIRE", key, seconds)
	if err != nil {
		return fmt.Errorf("set Redis key ttl: %w", err)
	}
	return nil
}

func persistKey(ctx context.Context, exec redisExecutor, key string) error {
	_, err := exec.Do(ctx, "PERSIST", key)
	if err != nil {
		return fmt.Errorf("persist Redis key: %w", err)
	}
	return nil
}

func renameKey(ctx context.Context, exec redisExecutor, oldKey, newKey string) error {
	result, err := exec.Do(ctx, "RENAMENX", oldKey, newKey)
	if err != nil {
		return fmt.Errorf("rename Redis key: %w", err)
	}
	if toInt64(result) == 0 {
		return fmt.Errorf("target key already exists")
	}
	return nil
}

func setStringValue(ctx context.Context, exec redisExecutor, req RedisStringSetRequest) error {
	value, err := EncodeValueForStorage(req.Value, req.Format)
	if err != nil {
		return err
	}
	ttlMillis := int64(-2)
	if ttl, ttlErr := exec.Do(ctx, "PTTL", req.Key); ttlErr == nil {
		ttlMillis = toInt64(ttl)
	}
	_, err = exec.Do(ctx, "SET", req.Key, value)
	if err != nil {
		return fmt.Errorf("set Redis string value: %w", err)
	}
	if ttlMillis > 0 {
		if _, err = exec.Do(ctx, "PEXPIRE", req.Key, ttlMillis); err != nil {
			return fmt.Errorf("restore Redis string ttl: %w", err)
		}
	}
	return nil
}

func deleteKeys(ctx context.Context, exec redisExecutor, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	args := make([]any, 0, len(keys)+1)
	args = append(args, "DEL")
	for _, key := range keys {
		args = append(args, key)
	}
	_, err := exec.Do(ctx, args...)
	if err != nil {
		return fmt.Errorf("delete Redis keys: %w", err)
	}
	return nil
}

func hashSet(ctx context.Context, exec redisExecutor, key, field, value string) error {
	_, err := exec.Do(ctx, "HSET", key, field, value)
	if err != nil {
		return fmt.Errorf("set Redis hash field: %w", err)
	}
	return nil
}

func hashDelete(ctx context.Context, exec redisExecutor, key, field string) error {
	_, err := exec.Do(ctx, "HDEL", key, field)
	if err != nil {
		return fmt.Errorf("delete Redis hash field: %w", err)
	}
	return nil
}

func listPush(ctx context.Context, exec redisExecutor, key, value string) error {
	_, err := exec.Do(ctx, "RPUSH", key, value)
	if err != nil {
		return fmt.Errorf("push Redis list value: %w", err)
	}
	return nil
}

func listSet(ctx context.Context, exec redisExecutor, key string, index int64, value string) error {
	_, err := exec.Do(ctx, "LSET", key, index, value)
	if err != nil {
		return fmt.Errorf("set Redis list value: %w", err)
	}
	return nil
}

func listDelete(ctx context.Context, exec redisExecutor, key string, index int64, sentinel string) error {
	if sentinel == "" {
		sentinel = fmt.Sprintf("__OPSKAT_LIST_DELETE_%d__", time.Now().UnixNano())
	}
	if _, err := exec.Do(ctx, "LSET", key, index, sentinel); err != nil {
		return fmt.Errorf("mark Redis list value for deletion: %w", err)
	}
	if _, err := exec.Do(ctx, "LREM", key, int64(1), sentinel); err != nil {
		return fmt.Errorf("delete Redis list value: %w", err)
	}
	return nil
}

func setAdd(ctx context.Context, exec redisExecutor, key, member string) error {
	_, err := exec.Do(ctx, "SADD", key, member)
	if err != nil {
		return fmt.Errorf("add Redis set member: %w", err)
	}
	return nil
}

func setRemove(ctx context.Context, exec redisExecutor, key, member string) error {
	_, err := exec.Do(ctx, "SREM", key, member)
	if err != nil {
		return fmt.Errorf("remove Redis set member: %w", err)
	}
	return nil
}

func zsetAdd(ctx context.Context, exec redisExecutor, key, member string, score float64) error {
	_, err := exec.Do(ctx, "ZADD", key, score, member)
	if err != nil {
		return fmt.Errorf("add Redis zset member: %w", err)
	}
	return nil
}

func zsetRemove(ctx context.Context, exec redisExecutor, key, member string) error {
	_, err := exec.Do(ctx, "ZREM", key, member)
	if err != nil {
		return fmt.Errorf("remove Redis zset member: %w", err)
	}
	return nil
}

func streamAdd(ctx context.Context, exec redisExecutor, key, id string, fields []RedisStreamField) error {
	if id == "" {
		id = "*"
	}
	args := make([]any, 0, 3+2*len(fields))
	args = append(args, "XADD", key, id)
	for _, field := range fields {
		args = append(args, field.Field, field.Value)
	}
	_, err := exec.Do(ctx, args...)
	if err != nil {
		return fmt.Errorf("add Redis stream entry: %w", err)
	}
	return nil
}

func streamDelete(ctx context.Context, exec redisExecutor, key string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, 0, len(ids)+2)
	args = append(args, "XDEL", key)
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := exec.Do(ctx, args...)
	if err != nil {
		return fmt.Errorf("delete Redis stream entries: %w", err)
	}
	return nil
}
