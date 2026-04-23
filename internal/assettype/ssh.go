package assettype

import (
	"context"
	"fmt"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/service/credential_mgr_svc"
	"github.com/opskat/opskat/internal/service/credential_resolver"
	"github.com/opskat/opskat/internal/service/credential_svc"
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

func (h *sshHandler) ApplyCreateArgs(ctx context.Context, a *asset_entity.Asset, args map[string]any) error {
	authType := ArgString(args, "auth_type")
	password := ArgString(args, "password")
	privateKey := ArgString(args, "private_key")
	if authType == "" {
		if privateKey != "" {
			authType = "key"
		} else {
			authType = "password"
		}
	}

	cfg := &asset_entity.SSHConfig{
		Host:     ArgString(args, "host"),
		Port:     ArgInt(args, "port"),
		Username: ArgString(args, "username"),
		AuthType: authType,
	}

	if privateKey != "" {
		credName := a.Name
		if credName == "" {
			credName = "ai-imported-key"
		}
		cred, err := credential_mgr_svc.ImportSSHKeyFromPEM(ctx, credName, "", privateKey, ArgString(args, "passphrase"))
		if err != nil {
			return fmt.Errorf("import SSH key: %w", err)
		}
		cfg.CredentialID = cred.ID
	} else if password != "" {
		encrypted, err := credential_svc.Default().Encrypt(password)
		if err != nil {
			return fmt.Errorf("encrypt SSH password: %w", err)
		}
		cfg.Password = encrypted
	}

	return a.SetSSHConfig(cfg)
}

func (h *sshHandler) ApplyUpdateArgs(ctx context.Context, a *asset_entity.Asset, args map[string]any) error {
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
	if password := ArgString(args, "password"); password != "" {
		encrypted, err := credential_svc.Default().Encrypt(password)
		if err != nil {
			return fmt.Errorf("encrypt SSH password: %w", err)
		}
		cfg.Password = encrypted
		cfg.CredentialID = 0 // 切换为内联密码，与原先关联的统一凭证解绑
		cfg.AuthType = "password"
	}
	if privateKey := ArgString(args, "private_key"); privateKey != "" {
		credName := a.Name
		if credName == "" {
			credName = "ai-imported-key"
		}
		cred, err := credential_mgr_svc.ImportSSHKeyFromPEM(ctx, credName, "", privateKey, ArgString(args, "passphrase"))
		if err != nil {
			return fmt.Errorf("import SSH key: %w", err)
		}
		cfg.CredentialID = cred.ID
		cfg.Password = ""
		cfg.AuthType = "key"
	}
	// 用户显式传入 auth_type 时最终覆盖（用于仅切换认证方式而不动凭据的场景）
	if v := ArgString(args, "auth_type"); v != "" {
		cfg.AuthType = v
	}
	return a.SetSSHConfig(cfg)
}
