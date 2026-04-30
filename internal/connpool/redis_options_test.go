package connpool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

func TestBuildRedisOptions(t *testing.T) {
	t.Run("maps basic redis config", func(t *testing.T) {
		opts, err := buildRedisOptions(&asset_entity.RedisConfig{
			Host:     "redis.internal",
			Port:     6380,
			Username: "default",
			Database: 2,
		}, "secret")

		require.NoError(t, err)
		assert.Equal(t, "redis.internal:6380", opts.Addr)
		assert.Equal(t, "default", opts.Username)
		assert.Equal(t, "secret", opts.Password)
		assert.Equal(t, 2, opts.DB)
		assert.Nil(t, opts.TLSConfig)
	})

	t.Run("maps tls and command timeout config", func(t *testing.T) {
		opts, err := buildRedisOptions(&asset_entity.RedisConfig{
			Host:                  "redis.internal",
			Port:                  6380,
			TLS:                   true,
			TLSInsecure:           true,
			TLSServerName:         "cache.example.com",
			CommandTimeoutSeconds: 7,
		}, "")

		require.NoError(t, err)
		require.NotNil(t, opts.TLSConfig)
		assert.True(t, opts.TLSConfig.InsecureSkipVerify)
		assert.Equal(t, "cache.example.com", opts.TLSConfig.ServerName)
		assert.Equal(t, 7*time.Second, opts.ReadTimeout)
		assert.Equal(t, 7*time.Second, opts.WriteTimeout)
	})
}
