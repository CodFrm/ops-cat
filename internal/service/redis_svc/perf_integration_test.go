//go:build redisperf

package redis_svc

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

const (
	perfRedisAddr    = "127.0.0.1:6379"
	perfRedisDB      = 15
	perfDefaultKeys  = 5000
	perfScanCount    = int64(200)
	perfKeyTTL       = int64(1800)
	perfTargetUUID   = "2fe43136-1b38-43c3-b4bf-82b19c66c7bf"
	perfValuePayload = "opskat redis perf value"
	perfRootMarker   = "root"
)

func TestRedisCRUDPerformance(t *testing.T) {
	totalKeys := perfKeyCount(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client := redis.NewClient(&redis.Options{
		Addr:         perfRedisAddr,
		DB:           perfRedisDB,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("local redis unavailable at %s db%d: %v", perfRedisAddr, perfRedisDB, err)
	}

	exec := &goRedisExecutor{
		client:  client,
		history: NewCommandHistory(200),
		assetID: 0,
		db:      perfRedisDB,
	}
	prefix := fmt.Sprintf("opskat:perf:%d", time.Now().UnixNano())
	uuidKey := prefix + ":common:event:" + perfTargetUUID
	t.Logf("redis=%s db=%d prefix=%s ttl=%ds", perfRedisAddr, perfRedisDB, prefix, perfKeyTTL)

	start := time.Now()
	groups := []string{"common", "dispatcher", "meetingtwin", "meetingtwinbackend"}
	for i := 0; i < totalKeys; i++ {
		group := groups[i%len(groups)]
		key := fmt.Sprintf("%s:%s:item:%05d", prefix, group, i)
		if i == totalKeys/2 {
			key = uuidKey
		}
		require.NoError(t, setStringValue(ctx, exec, RedisStringSetRequest{
			DB:     perfRedisDB,
			Key:    key,
			Value:  perfValuePayload,
			Format: RedisValueFormatRaw,
		}))
		require.NoError(t, setKeyTTL(ctx, exec, key, perfKeyTTL))
	}
	t.Logf("crud set+ttl keys=%d elapsed=%s", totalKeys, time.Since(start).Round(time.Millisecond))

	hashKey := prefix + ":hash:sample"
	require.NoError(t, hashSet(ctx, exec, hashKey, "field", "value"))
	require.NoError(t, setKeyTTL(ctx, exec, hashKey, perfKeyTTL))
	listKey := prefix + ":list:sample"
	require.NoError(t, listPush(ctx, exec, listKey, "value"))
	require.NoError(t, setKeyTTL(ctx, exec, listKey, perfKeyTTL))
	setKey := prefix + ":set:sample"
	require.NoError(t, setAdd(ctx, exec, setKey, "member"))
	require.NoError(t, setKeyTTL(ctx, exec, setKey, perfKeyTTL))
	zsetKey := prefix + ":zset:sample"
	require.NoError(t, zsetAdd(ctx, exec, zsetKey, "member", 1.25))
	require.NoError(t, setKeyTTL(ctx, exec, zsetKey, perfKeyTTL))
	streamKey := prefix + ":stream:sample"
	require.NoError(t, streamAdd(ctx, exec, streamKey, "*", []RedisStreamField{{Field: "field", Value: "value"}}))
	require.NoError(t, setKeyTTL(ctx, exec, streamKey, perfKeyTTL))
	rootKey := prefix + ":acl:" + perfRootMarker + ":sample"
	require.NoError(t, setStringValue(ctx, exec, RedisStringSetRequest{
		DB:     perfRedisDB,
		Key:    rootKey,
		Value:  perfValuePayload,
		Format: RedisValueFormatRaw,
	}))
	require.NoError(t, setKeyTTL(ctx, exec, rootKey, perfKeyTTL))
	t.Log("crud type samples=hash,list,set,zset,stream,string-root inserted")

	measureScan(t, ctx, exec, "prefix first page", RedisScanRequest{
		DB:    perfRedisDB,
		Match: prefix + ":*",
		Count: perfScanCount,
	})
	measureScan(t, ctx, exec, "folder common first page", RedisScanRequest{
		DB:    perfRedisDB,
		Match: prefix + ":common:*",
		Count: perfScanCount,
	})
	measureScan(t, ctx, exec, "sparse uuid contains", RedisScanRequest{
		DB:    perfRedisDB,
		Match: "*" + perfTargetUUID + "*",
		Count: perfScanCount,
	})
	measureScan(t, ctx, exec, "sparse root contains", RedisScanRequest{
		DB:    perfRedisDB,
		Match: "*" + perfRootMarker + "*",
		Count: perfScanCount,
	})
	measureScan(t, ctx, exec, "exact uuid key", RedisScanRequest{
		DB:    perfRedisDB,
		Match: uuidKey,
		Count: perfScanCount,
		Exact: true,
	})

	start = time.Now()
	detail, err := getKeyDetail(ctx, exec, RedisKeyRequest{
		DB:  perfRedisDB,
		Key: uuidKey,
	})
	require.NoError(t, err)
	require.Equal(t, "string", detail.Type)
	t.Logf("get detail key=%s type=%s ttl=%d size=%d elapsed=%s", shorten(uuidKey), detail.Type, detail.TTL, detail.Size, time.Since(start).Round(time.Millisecond))

	start = time.Now()
	total, pages := drainScan(t, ctx, exec, RedisScanRequest{
		DB:    perfRedisDB,
		Match: prefix + ":*",
		Count: perfScanCount,
	})
	elapsed := time.Since(start)
	rate := float64(total) / elapsed.Seconds()
	t.Logf("tree drain match=%s pages=%d keys=%d elapsed=%s rate=%.0f_keys_per_sec", prefix+":*", pages, total, elapsed.Round(time.Millisecond), rate)

	dbs, err := listDatabases(ctx, exec)
	require.NoError(t, err)
	for _, db := range dbs {
		if db.DB == perfRedisDB {
			t.Logf("keyspace db=%d keys=%d expires=%d avg_ttl=%d", db.DB, db.Keys, db.Expires, db.AvgTTL)
			break
		}
	}
	t.Log("test keys are retained with TTL for automatic expiration")
}

func perfKeyCount(t *testing.T) int {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("OPSKAT_REDIS_PERF_KEYS"))
	if raw == "" {
		return perfDefaultKeys
	}
	count, err := strconv.Atoi(raw)
	require.NoError(t, err)
	require.Greater(t, count, 0)
	return count
}

func measureScan(t *testing.T, ctx context.Context, exec redisExecutor, name string, req RedisScanRequest) {
	t.Helper()
	start := time.Now()
	out, err := scanKeys(ctx, exec, req)
	require.NoError(t, err)
	require.NotEmpty(t, out.Keys, name)
	t.Logf("scan %s match=%s keys=%d cursor=%s has_more=%t elapsed=%s", name, req.Match, len(out.Keys), out.Cursor, out.HasMore, time.Since(start).Round(time.Millisecond))
}

func drainScan(t *testing.T, ctx context.Context, exec redisExecutor, req RedisScanRequest) (int, int) {
	t.Helper()
	total := 0
	pages := 0
	cursor := "0"
	for {
		req.Cursor = cursor
		out, err := scanKeys(ctx, exec, req)
		require.NoError(t, err)
		total += len(out.Keys)
		pages++
		cursor = out.Cursor
		if !out.HasMore {
			break
		}
	}
	return total, pages
}

func shorten(value string) string {
	if len(value) <= 72 {
		return value
	}
	return value[:36] + "..." + strings.TrimPrefix(value[len(value)-32:], ":")
}
