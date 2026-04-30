package redis_svc

import (
	"context"
	"fmt"
)

func (s *Service) ClientList(ctx context.Context, assetID int64) (string, error) {
	var out string
	err := s.withClient(ctx, assetID, -1, func(ctx context.Context, exec redisExecutor) error {
		var err error
		out, err = clientList(ctx, exec)
		return err
	})
	return out, err
}

func (s *Service) SlowLog(ctx context.Context, assetID int64, limit int64) ([]RedisSlowLogEntry, error) {
	var out []RedisSlowLogEntry
	err := s.withClient(ctx, assetID, -1, func(ctx context.Context, exec redisExecutor) error {
		var err error
		out, err = slowLog(ctx, exec, limit)
		return err
	})
	return out, err
}

func (s *Service) CommandHistory(assetID int64, limit int) []CommandHistoryEntry {
	if s == nil || s.history == nil {
		return nil
	}
	return s.history.List(assetID, limit)
}

func clientList(ctx context.Context, exec redisExecutor) (string, error) {
	result, err := exec.Do(ctx, "CLIENT", "LIST")
	if err != nil {
		return "", fmt.Errorf("load Redis client list: %w", err)
	}
	return fmt.Sprint(result), nil
}

func slowLog(ctx context.Context, exec redisExecutor, limit int64) ([]RedisSlowLogEntry, error) {
	if limit <= 0 {
		limit = 128
	}
	result, err := exec.Do(ctx, "SLOWLOG", "GET", limit)
	if err != nil {
		return nil, fmt.Errorf("load Redis slowlog: %w", err)
	}
	rawEntries, ok := result.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected Redis SLOWLOG result: %T", result)
	}
	entries := make([]RedisSlowLogEntry, 0, len(rawEntries))
	for _, rawEntry := range rawEntries {
		items, ok := rawEntry.([]any)
		if !ok || len(items) < 4 {
			continue
		}
		entry := RedisSlowLogEntry{
			ID:             toInt64(items[0]),
			Timestamp:      toInt64(items[1]),
			DurationMicros: toInt64(items[2]),
			Command:        toStringSlice(items[3]),
		}
		if len(items) > 4 {
			entry.Client = fmt.Sprint(items[4])
		}
		if len(items) > 5 {
			entry.ClientName = fmt.Sprint(items[5])
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
