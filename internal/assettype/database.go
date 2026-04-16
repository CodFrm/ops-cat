package assettype

import (
	"context"
	"fmt"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/service/credential_resolver"
)

type databaseHandler struct{}

func init() {
	Register(&databaseHandler{})
	policy.RegisterDefaultPolicy("database", func() any { return asset_entity.DefaultQueryPolicy() })
}

func (h *databaseHandler) Type() string     { return asset_entity.AssetTypeDatabase }
func (h *databaseHandler) DefaultPort() int { return 3306 }

func (h *databaseHandler) SafeView(a *asset_entity.Asset) map[string]any {
	cfg, err := a.GetDatabaseConfig()
	if err != nil || cfg == nil {
		return nil
	}
	return map[string]any{
		"host": cfg.Host, "port": cfg.Port,
		"username": cfg.Username, "driver": string(cfg.Driver),
		"database": cfg.Database, "read_only": cfg.ReadOnly,
	}
}

func (h *databaseHandler) ResolvePassword(ctx context.Context, a *asset_entity.Asset) (string, error) {
	cfg, err := a.GetDatabaseConfig()
	if err != nil {
		return "", fmt.Errorf("get database config failed: %w", err)
	}
	return credential_resolver.Default().ResolvePasswordGeneric(ctx, cfg)
}

func (h *databaseHandler) DefaultPolicy() any { return asset_entity.DefaultQueryPolicy() }

func (h *databaseHandler) ApplyCreateArgs(a *asset_entity.Asset, args map[string]any) error {
	driver := ArgString(args, "driver")
	if driver == "" {
		return fmt.Errorf("database type requires driver parameter (mysql or postgresql)")
	}
	return a.SetDatabaseConfig(&asset_entity.DatabaseConfig{
		Driver:     asset_entity.DatabaseDriver(driver),
		Host:       ArgString(args, "host"),
		Port:       ArgInt(args, "port"),
		Username:   ArgString(args, "username"),
		Database:   ArgString(args, "database"),
		ReadOnly:   ArgString(args, "read_only") == "true",
		SSHAssetID: ArgInt64(args, "ssh_asset_id"),
	})
}

func (h *databaseHandler) ApplyUpdateArgs(a *asset_entity.Asset, args map[string]any) error {
	cfg, err := a.GetDatabaseConfig()
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
	return a.SetDatabaseConfig(cfg)
}
