package assettype

import (
	"context"
	"fmt"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/service/credential_resolver"
)

type mongodbHandler struct{}

func init() {
	Register(&mongodbHandler{})
	policy.RegisterDefaultPolicy("mongodb", func() any { return asset_entity.DefaultMongoPolicy() })
}

func (h *mongodbHandler) Type() string     { return asset_entity.AssetTypeMongoDB }
func (h *mongodbHandler) DefaultPort() int { return 27017 }

func (h *mongodbHandler) SafeView(a *asset_entity.Asset) map[string]any {
	cfg, err := a.GetMongoDBConfig()
	if err != nil || cfg == nil {
		return nil
	}
	return map[string]any{
		"host": cfg.Host, "port": cfg.Port,
		"username": cfg.Username, "database": cfg.Database,
	}
}

func (h *mongodbHandler) ResolvePassword(ctx context.Context, a *asset_entity.Asset) (string, error) {
	cfg, err := a.GetMongoDBConfig()
	if err != nil {
		return "", fmt.Errorf("get MongoDB config failed: %w", err)
	}
	return credential_resolver.Default().ResolvePasswordGeneric(ctx, cfg)
}

func (h *mongodbHandler) DefaultPolicy() any { return asset_entity.DefaultMongoPolicy() }

func (h *mongodbHandler) ApplyCreateArgs(a *asset_entity.Asset, args map[string]any) error {
	a.SSHTunnelID = ArgInt64(args, "ssh_asset_id")
	return a.SetMongoDBConfig(&asset_entity.MongoDBConfig{
		Host:       ArgString(args, "host"),
		Port:       ArgInt(args, "port"),
		Username:   ArgString(args, "username"),
		Database:   ArgString(args, "database"),
		AuthSource: "admin",
	})
}

func (h *mongodbHandler) ApplyUpdateArgs(a *asset_entity.Asset, args map[string]any) error {
	cfg, err := a.GetMongoDBConfig()
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
	if v := ArgString(args, "database"); v != "" {
		cfg.Database = v
	}
	return a.SetMongoDBConfig(cfg)
}
