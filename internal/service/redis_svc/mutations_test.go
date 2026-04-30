package redis_svc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisMutations(t *testing.T) {
	ctx := context.Background()

	t.Run("sets ttl and persists key", func(t *testing.T) {
		exec := &fakeRedisExecutor{}

		require.NoError(t, setKeyTTL(ctx, exec, "session:1", 60))
		require.NoError(t, persistKey(ctx, exec, "session:1"))

		assert.Equal(t, []any{"EXPIRE", "session:1", int64(60)}, exec.calls[0])
		assert.Equal(t, []any{"PERSIST", "session:1"}, exec.calls[1])
	})

	t.Run("renames with NX semantics", func(t *testing.T) {
		exec := &fakeRedisExecutor{results: []any{int64(1)}}

		err := renameKey(ctx, exec, "old", "new")

		require.NoError(t, err)
		assert.Equal(t, []any{"RENAMENX", "old", "new"}, exec.calls[0])
	})

	t.Run("sets string without splitting spaces", func(t *testing.T) {
		exec := &fakeRedisExecutor{results: []any{int64(-1), nil}}

		err := setStringValue(ctx, exec, RedisStringSetRequest{Key: "k", Value: "hello world", Format: RedisValueFormatRaw})

		require.NoError(t, err)
		assert.Equal(t, []any{"PTTL", "k"}, exec.calls[0])
		assert.Equal(t, []any{"SET", "k", "hello world"}, exec.calls[1])
	})

	t.Run("decodes hex before setting string", func(t *testing.T) {
		exec := &fakeRedisExecutor{results: []any{int64(-1), nil}}

		err := setStringValue(ctx, exec, RedisStringSetRequest{Key: "k", Value: "6869", Format: RedisValueFormatHex})

		require.NoError(t, err)
		assert.Equal(t, []any{"PTTL", "k"}, exec.calls[0])
		assert.Equal(t, []any{"SET", "k", "hi"}, exec.calls[1])
	})

	t.Run("preserves existing string ttl when editing", func(t *testing.T) {
		exec := &fakeRedisExecutor{results: []any{int64(12500), nil, nil}}

		err := setStringValue(ctx, exec, RedisStringSetRequest{Key: "k", Value: "v", Format: RedisValueFormatRaw})

		require.NoError(t, err)
		assert.Equal(t, []any{"PTTL", "k"}, exec.calls[0])
		assert.Equal(t, []any{"SET", "k", "v"}, exec.calls[1])
		assert.Equal(t, []any{"PEXPIRE", "k", int64(12500)}, exec.calls[2])
	})

	t.Run("deletes multiple keys with one command", func(t *testing.T) {
		exec := &fakeRedisExecutor{}

		require.NoError(t, deleteKeys(ctx, exec, []string{"a", "b"}))

		assert.Equal(t, []any{"DEL", "a", "b"}, exec.calls[0])
	})

	t.Run("hash list set zset and stream operations use argument arrays", func(t *testing.T) {
		exec := &fakeRedisExecutor{}

		require.NoError(t, hashSet(ctx, exec, "h", "field name", "value with spaces"))
		require.NoError(t, hashDelete(ctx, exec, "h", "field name"))
		require.NoError(t, listPush(ctx, exec, "l", "value with spaces"))
		require.NoError(t, listSet(ctx, exec, "l", 3, "value with spaces"))
		require.NoError(t, listDelete(ctx, exec, "l", 3, "sentinel"))
		require.NoError(t, setAdd(ctx, exec, "s", "member with spaces"))
		require.NoError(t, setRemove(ctx, exec, "s", "member with spaces"))
		require.NoError(t, zsetAdd(ctx, exec, "z", "member with spaces", 1.5))
		require.NoError(t, zsetRemove(ctx, exec, "z", "member with spaces"))
		require.NoError(t, streamAdd(ctx, exec, "x", "*", []RedisStreamField{{Field: "name", Value: "alice bob"}}))
		require.NoError(t, streamDelete(ctx, exec, "x", []string{"1-0"}))

		assert.Equal(t, []any{"HSET", "h", "field name", "value with spaces"}, exec.calls[0])
		assert.Equal(t, []any{"HDEL", "h", "field name"}, exec.calls[1])
		assert.Equal(t, []any{"RPUSH", "l", "value with spaces"}, exec.calls[2])
		assert.Equal(t, []any{"LSET", "l", int64(3), "value with spaces"}, exec.calls[3])
		assert.Equal(t, []any{"LSET", "l", int64(3), "sentinel"}, exec.calls[4])
		assert.Equal(t, []any{"LREM", "l", int64(1), "sentinel"}, exec.calls[5])
		assert.Equal(t, []any{"SADD", "s", "member with spaces"}, exec.calls[6])
		assert.Equal(t, []any{"SREM", "s", "member with spaces"}, exec.calls[7])
		assert.Equal(t, []any{"ZADD", "z", 1.5, "member with spaces"}, exec.calls[8])
		assert.Equal(t, []any{"ZREM", "z", "member with spaces"}, exec.calls[9])
		assert.Equal(t, []any{"XADD", "x", "*", "name", "alice bob"}, exec.calls[10])
		assert.Equal(t, []any{"XDEL", "x", "1-0"}, exec.calls[11])
	})
}
