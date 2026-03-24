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

	"github.com/redis/go-redis/v9"
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
		DB:       cfg.Database,
	}
	if cfg.TLS {
		opts.TLSConfig = &tls.Config{} //nolint:gosec // TLS config uses defaults
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
		_ = client.Close()
		if tunnel != nil {
			_ = tunnel.Close()
		}
		return nil, nil, fmt.Errorf("Redis 连接失败: %w", pingErr)
	}

	return client, tunnel, nil
}
