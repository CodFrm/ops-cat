package assettype

import (
	"context"
	"fmt"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/service/credential_resolver"
)

type sshHandler struct{}

func init() {
	Register(&sshHandler{})
	policy.RegisterDefaultPolicy("ssh", func() any { return asset_entity.DefaultCommandPolicy() })
}

func (h *sshHandler) Type() string     { return asset_entity.AssetTypeSSH }
func (h *sshHandler) DefaultPort() int { return 22 }

func (h *sshHandler) SafeView(a *asset_entity.Asset) map[string]any {
	cfg, err := a.GetSSHConfig()
	if err != nil || cfg == nil {
		return nil
	}
	return map[string]any{
		"host": cfg.Host, "port": cfg.Port,
		"username": cfg.Username, "auth_type": cfg.AuthType,
	}
}

func (h *sshHandler) ResolvePassword(ctx context.Context, a *asset_entity.Asset) (string, error) {
	cfg, err := a.GetSSHConfig()
	if err != nil {
		return "", fmt.Errorf("get SSH config failed: %w", err)
	}
	password, _, _, err := credential_resolver.Default().ResolveSSHCredentials(ctx, cfg)
	return password, err
}

func (h *sshHandler) DefaultPolicy() any { return asset_entity.DefaultCommandPolicy() }

func (h *sshHandler) ApplyCreateArgs(a *asset_entity.Asset, args map[string]any) error {
	authType := ArgString(args, "auth_type")
	if authType == "" {
		authType = "password"
	}
	return a.SetSSHConfig(&asset_entity.SSHConfig{
		Host: ArgString(args, "host"), Port: ArgInt(args, "port"),
		Username: ArgString(args, "username"), AuthType: authType,
	})
}

func (h *sshHandler) ApplyUpdateArgs(a *asset_entity.Asset, args map[string]any) error {
	cfg, err := a.GetSSHConfig()
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
	return a.SetSSHConfig(cfg)
}
