package assettype

import (
	"context"
	"fmt"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/service/credential_resolver"
)

type redisHandler struct{}

func init() {
	Register(&redisHandler{})
	policy.RegisterDefaultPolicy("redis", func() any { return asset_entity.DefaultRedisPolicy() })
}

func (h *redisHandler) Type() string     { return asset_entity.AssetTypeRedis }
func (h *redisHandler) DefaultPort() int { return 6379 }

func (h *redisHandler) SafeView(a *asset_entity.Asset) map[string]any {
	cfg, err := a.GetRedisConfig()
	if err != nil || cfg == nil {
		return nil
	}
	return map[string]any{
		"host": cfg.Host, "port": cfg.Port,
		"username": cfg.Username, "redis_db": cfg.Database,
	}
}

func (h *redisHandler) ResolvePassword(ctx context.Context, a *asset_entity.Asset) (string, error) {
	cfg, err := a.GetRedisConfig()
	if err != nil {
		return "", fmt.Errorf("get Redis config failed: %w", err)
	}
	return credential_resolver.Default().ResolvePasswordGeneric(ctx, cfg)
}

func (h *redisHandler) DefaultPolicy() any { return asset_entity.DefaultRedisPolicy() }

func (h *redisHandler) ApplyCreateArgs(a *asset_entity.Asset, args map[string]any) error {
	return a.SetRedisConfig(&asset_entity.RedisConfig{
		Host:       ArgString(args, "host"),
		Port:       ArgInt(args, "port"),
		Username:   ArgString(args, "username"),
		SSHAssetID: ArgInt64(args, "ssh_asset_id"),
	})
}

func (h *redisHandler) ApplyUpdateArgs(a *asset_entity.Asset, args map[string]any) error {
	cfg, err := a.GetRedisConfig()
	if err != nil || cfg == nil {
		return err
	}
	if v := ArgString(args, "host"); v != "" {
		cfg.Host = v
	}
	if v := ArgInt(args, "port"); v > 0 {
		cfg.Port = v
	}
	if v := ArgString(args, "username"); v != "" {
		cfg.Username = v
	}
	return a.SetRedisConfig(cfg)
}
