package redis_svc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRedisExecutor struct {
	calls   [][]any
	results []any
	errs    []error
}

func (f *fakeRedisExecutor) Do(_ context.Context, args ...any) (any, error) {
	f.calls = append(f.calls, args)
	idx := len(f.calls) - 1
	if idx < len(f.errs) && f.errs[idx] != nil {
		return nil, f.errs[idx]
	}
	if idx < len(f.results) {
		return f.results[idx], nil
	}
	return nil, nil
}

func TestListDatabases(t *testing.T) {
	exec := &fakeRedisExecutor{results: []any{"# Keyspace\r\ndb0:keys=2,expires=1,avg_ttl=5\r\ndb3:keys=9,expires=0,avg_ttl=0\r\n"}}

	got, err := listDatabases(context.Background(), exec)

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, []any{"INFO", "keyspace"}, exec.calls[0])
	assert.Equal(t, 0, got[0].DB)
	assert.Equal(t, int64(2), got[0].Keys)
	assert.Equal(t, 3, got[1].DB)
}

func TestScanKeys(t *testing.T) {
	t.Run("builds scan command with match count and type", func(t *testing.T) {
		exec := &fakeRedisExecutor{results: []any{[]any{"17", []any{"a", "b"}}}}

		got, err := scanKeys(context.Background(), exec, RedisScanRequest{
			Cursor: "0",
			Match:  "user:*",
			Count:  100,
			Type:   "hash",
		})

		require.NoError(t, err)
		assert.Equal(t, []any{"SCAN", "0", "MATCH", "user:*", "COUNT", int64(100), "TYPE", "hash"}, exec.calls[0])
		assert.Equal(t, "17", got.Cursor)
		assert.True(t, got.HasMore)
		assert.Equal(t, []string{"a", "b"}, got.Keys)
	})

	t.Run("exact lookup uses exists and type", func(t *testing.T) {
		exec := &fakeRedisExecutor{results: []any{int64(1), "string"}}

		got, err := scanKeys(context.Background(), exec, RedisScanRequest{
			Match: "session:1",
			Type:  "string",
			Exact: true,
		})

		require.NoError(t, err)
		assert.Equal(t, []any{"EXISTS", "session:1"}, exec.calls[0])
		assert.Equal(t, []any{"TYPE", "session:1"}, exec.calls[1])
		assert.Equal(t, []string{"session:1"}, got.Keys)
		assert.False(t, got.HasMore)
	})

	t.Run("continues sparse match until it returns keys", func(t *testing.T) {
		exec := &fakeRedisExecutor{results: []any{
			[]any{"42", []any{}},
			[]any{"0", []any{"common:event:2fe43136-1b38-43c3-b4bf-82b19c66c7bf"}},
		}}

		got, err := scanKeys(context.Background(), exec, RedisScanRequest{
			Cursor: "0",
			Match:  "*2fe43136-1b38-43c3-b4bf-82b19c66c7bf*",
			Count:  100,
		})

		require.NoError(t, err)
		require.Len(t, exec.calls, 2)
		assert.Equal(t, []any{"SCAN", "0", "MATCH", "*2fe43136-1b38-43c3-b4bf-82b19c66c7bf*", "COUNT", int64(100)}, exec.calls[0])
		assert.Equal(t, []any{"SCAN", "42", "MATCH", "*2fe43136-1b38-43c3-b4bf-82b19c66c7bf*", "COUNT", int64(100)}, exec.calls[1])
		assert.Equal(t, "0", got.Cursor)
		assert.Equal(t, []string{"common:event:2fe43136-1b38-43c3-b4bf-82b19c66c7bf"}, got.Keys)
		assert.False(t, got.HasMore)
	})
}

func TestGetKeyDetail(t *testing.T) {
	t.Run("loads hash page", func(t *testing.T) {
		exec := &fakeRedisExecutor{results: []any{
			"hash",
			int64(120),
			int64(42),
			int64(2),
			[]any{"0", []any{"field", "value"}},
		}}

		got, err := getKeyDetail(context.Background(), exec, RedisKeyRequest{Key: "user:1"})

		require.NoError(t, err)
		assert.Equal(t, "hash", got.Type)
		assert.Equal(t, int64(120), got.TTL)
		assert.Equal(t, int64(42), got.Size)
		assert.Equal(t, int64(2), got.Total)
		assert.Equal(t, []RedisHashEntry{{Field: "field", Value: "value"}}, got.Value)
		assert.Equal(t, []any{"TYPE", "user:1"}, exec.calls[0])
		assert.Equal(t, []any{"TTL", "user:1"}, exec.calls[1])
		assert.Equal(t, []any{"MEMORY", "USAGE", "user:1"}, exec.calls[2])
		assert.Equal(t, []any{"HLEN", "user:1"}, exec.calls[3])
		assert.Equal(t, []any{"HSCAN", "user:1", "0", "COUNT", int64(100)}, exec.calls[4])
	})

	t.Run("loads next stream page without duplicating cursor entry", func(t *testing.T) {
		exec := &fakeRedisExecutor{results: []any{
			"stream",
			int64(-1),
			int64(80),
			int64(4),
			[]any{
				[]any{"1-0", []any{"name", "old"}},
				[]any{"2-0", []any{"name", "Ada"}},
				[]any{"3-0", []any{"name", "Lin"}},
			},
		}}

		got, err := getKeyDetail(context.Background(), exec, RedisKeyRequest{
			Key:    "events",
			Cursor: "1-0",
			Offset: 2,
			Count:  2,
		})

		require.NoError(t, err)
		assert.Equal(t, []RedisStreamEntry{
			{ID: "2-0", Fields: map[string]string{"name": "Ada"}},
			{ID: "3-0", Fields: map[string]string{"name": "Lin"}},
		}, got.Value)
		assert.Equal(t, "3-0", got.ValueCursor)
		assert.Equal(t, int64(4), got.ValueOffset)
		assert.False(t, got.HasMoreValues)
		assert.Equal(t, []any{"XRANGE", "events", "1-0", "+", "COUNT", int64(3)}, exec.calls[4])
	})
}
