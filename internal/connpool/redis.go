package connpool

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/sshpool"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// DialRedis 创建 Redis 连接（直连或通过 SSH 隧道）
// password 为已解析的明文密码，由调用方负责解密
func DialRedis(ctx context.Context, asset *asset_entity.Asset, cfg *asset_entity.RedisConfig, password string, sshPool *sshpool.Pool) (*redis.Client, io.Closer, error) {
	opts, err := buildRedisOptions(cfg, password)
	if err != nil {
		return nil, nil, err
	}

	var tunnel *SSHTunnel
	tunnelID := asset.SSHTunnelID
	if tunnelID == 0 {
		tunnelID = cfg.SSHAssetID // backward compat
	}
	if tunnelID > 0 && sshPool != nil {
		tunnel = NewSSHTunnel(tunnelID, cfg.Host, cfg.Port, sshPool)
		opts.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return tunnel.Dial(ctx)
		}
	}

	client := redis.NewClient(opts)
	if pingErr := client.Ping(ctx).Err(); pingErr != nil {
		if err := client.Close(); err != nil {
			logger.Ctx(ctx).Warn("close redis client", zap.Error(err))
		}
		if tunnel != nil {
			if err := tunnel.Close(); err != nil {
				logger.Ctx(ctx).Warn("close ssh tunnel", zap.Error(err))
			}
		}
		return nil, nil, fmt.Errorf("redis 连接失败: %w", pingErr)
	}

	// 直连时 tunnel 为 *SSHTunnel 的 nil，直接返回会变成 typed-nil 接口，
	// 调用方 `if closer != nil` 会误判为真并在 Close() 里 nil deref panic。
	if tunnel == nil {
		return client, nil, nil
	}
	return client, tunnel, nil
}

func buildRedisOptions(cfg *asset_entity.RedisConfig, password string) (*redis.Options, error) {
	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Username: cfg.Username,
		Password: password,
		DB:       cfg.Database,
	}
	if cfg.CommandTimeoutSeconds > 0 {
		timeout := time.Duration(cfg.CommandTimeoutSeconds) * time.Second
		opts.ReadTimeout = timeout
		opts.WriteTimeout = timeout
	}
	if cfg.TLS {
		tlsConfig, err := buildRedisTLSConfig(cfg)
		if err != nil {
			return nil, err
		}
		opts.TLSConfig = tlsConfig
	}
	return opts, nil
}

func buildRedisTLSConfig(cfg *asset_entity.RedisConfig) (*tls.Config, error) {
	return BuildTLSConfig("Redis", TLSFields{
		ServerName: cfg.TLSServerName,
		Insecure:   cfg.TLSInsecure,
		CAFile:     cfg.TLSCAFile,
		CertFile:   cfg.TLSCertFile,
		KeyFile:    cfg.TLSKeyFile,
	})
}
