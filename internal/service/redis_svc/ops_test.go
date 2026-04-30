package redis_svc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpsHelpers(t *testing.T) {
	t.Run("loads client list as text", func(t *testing.T) {
		exec := &fakeRedisExecutor{results: []any{"id=1 addr=127.0.0.1:1234 name=ops\n"}}

		got, err := clientList(context.Background(), exec)

		require.NoError(t, err)
		assert.Equal(t, "id=1 addr=127.0.0.1:1234 name=ops\n", got)
		assert.Equal(t, []any{"CLIENT", "LIST"}, exec.calls[0])
	})

	t.Run("loads slowlog entries", func(t *testing.T) {
		exec := &fakeRedisExecutor{results: []any{[]any{
			[]any{int64(1), int64(1700000000), int64(2300), []any{"GET", "k"}, "127.0.0.1:1", "ops"},
		}}}

		got, err := slowLog(context.Background(), exec, 10)

		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, int64(1), got[0].ID)
		assert.Equal(t, int64(1700000000), got[0].Timestamp)
		assert.Equal(t, int64(2300), got[0].DurationMicros)
		assert.Equal(t, []string{"GET", "k"}, got[0].Command)
		assert.Equal(t, "127.0.0.1:1", got[0].Client)
		assert.Equal(t, "ops", got[0].ClientName)
		assert.Equal(t, []any{"SLOWLOG", "GET", int64(10)}, exec.calls[0])
	})
}
