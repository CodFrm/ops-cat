package connpool

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"

	"ops-cat/internal/model/entity/asset_entity"
	"ops-cat/internal/service/credential_svc"
	"ops-cat/internal/sshpool"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// DialRedis 创建 Redis 连接（直连或通过 SSH 隧道）
func DialRedis(ctx context.Context, cfg *asset_entity.RedisConfig, sshPool *sshpool.Pool) (*redis.Client, io.Closer, error) {
	password, err := credential_svc.Default().Decrypt(cfg.Password)
	if err != nil {
		password = cfg.Password
	}

	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Username: cfg.Username,
		Password: password,
	}
	if cfg.TLS {
		opts.TLSConfig = &tls.Config{}
	}

	var tunnel *SSHTunnel
	if cfg.SSHAssetID > 0 && sshPool != nil {
		tunnel = NewSSHTunnel(cfg.SSHAssetID, cfg.Host, cfg.Port, sshPool)
		opts.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return tunnel.Dial(ctx)
		}
	}

	client := redis.NewClient(opts)
	if pingErr := client.Ping(ctx).Err(); pingErr != nil {
		if err := client.Close(); err != nil {
			logger.Default().Warn("close redis client", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Default().Warn("close ssh tunnel", zap.Error(err))
			}
		}
		return nil, nil, fmt.Errorf("redis 连接失败: %w", pingErr)
	}

	return client, tunnel, nil
}
