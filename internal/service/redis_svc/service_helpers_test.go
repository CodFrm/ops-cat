package redis_svc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKeyspaceInfo(t *testing.T) {
	info := "# Server\r\nredis_version:7.2.0\r\n# Keyspace\r\ndb0:keys=12,expires=2,avg_ttl=3000\r\ndb2:keys=5,expires=0,avg_ttl=0\r\n"

	got := ParseKeyspaceInfo(info)

	require.Len(t, got, 2)
	assert.Equal(t, 0, got[0].DB)
	assert.Equal(t, int64(12), got[0].Keys)
	assert.Equal(t, int64(2), got[0].Expires)
	assert.Equal(t, int64(3000), got[0].AvgTTL)
	assert.Equal(t, 2, got[1].DB)
	assert.Equal(t, int64(5), got[1].Keys)
}

func TestNormalizeScanOptions(t *testing.T) {
	t.Run("fills safe defaults", func(t *testing.T) {
		got := NormalizeScanOptions(RedisScanRequest{})

		assert.Equal(t, "*", got.Match)
		assert.Equal(t, int64(200), got.Count)
		assert.Equal(t, "0", got.Cursor)
		assert.Empty(t, got.Type)
	})

	t.Run("clamps excessive page size", func(t *testing.T) {
		got := NormalizeScanOptions(RedisScanRequest{Count: 50000})

		assert.Equal(t, int64(2000), got.Count)
	})

	t.Run("normalizes type filter", func(t *testing.T) {
		got := NormalizeScanOptions(RedisScanRequest{Type: " HASH "})

		assert.Equal(t, "hash", got.Type)
	})
}

func TestValidatePatternDelete(t *testing.T) {
	t.Run("rejects empty and full database patterns", func(t *testing.T) {
		assert.Error(t, ValidatePatternDelete(""))
		assert.Error(t, ValidatePatternDelete("   "))
		assert.Error(t, ValidatePatternDelete("*"))
		assert.Error(t, ValidatePatternDelete("**"))
	})

	t.Run("rejects patterns without wildcard to avoid accidental single-key deletes", func(t *testing.T) {
		assert.Error(t, ValidatePatternDelete("user:1"))
	})

	t.Run("accepts scoped wildcard patterns", func(t *testing.T) {
		assert.NoError(t, ValidatePatternDelete("user:*"))
		assert.NoError(t, ValidatePatternDelete("cache:session:?"))
	})
}
