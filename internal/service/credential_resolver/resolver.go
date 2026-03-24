package credential_resolver

import (
	"context"
	"fmt"
	"os"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/credential_entity"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/credential_mgr_svc"
	"github.com/opskat/opskat/internal/service/credential_svc"
	"github.com/opskat/opskat/internal/service/ssh_svc"
)

// Resolver 统一凭据解析服务
type Resolver struct{}

var defaultResolver = &Resolver{}

// Default 获取默认 Resolver 实例
func Default() *Resolver {
	return defaultResolver
}

// ResolveSSHCredentials 从 SSHConfig 解析明文密码/密钥
// 优先使用统一凭证，向后兼容内联密码和本地密钥文件
func (r *Resolver) ResolveSSHCredentials(ctx context.Context, cfg *asset_entity.SSHConfig) (password, key string, err error) {
	// 优先使用统一凭证
	if cfg.CredentialID > 0 {
		cred, err := credential_mgr_svc.Get(ctx, cfg.CredentialID)
		if err != nil {
			return "", "", fmt.Errorf("获取凭证失败: %w", err)
		}
		switch cred.Type {
		case credential_entity.TypePassword:
			decrypted, err := credential_svc.Default().Decrypt(cred.Password)
			if err != nil {
				return "", "", fmt.Errorf("解密密码失败: %w", err)
			}
			return decrypted, "", nil
		case credential_entity.TypeSSHKey:
			privKey, err := credential_mgr_svc.GetDecryptedPrivateKey(ctx, cfg.CredentialID)
			if err != nil {
				return "", "", fmt.Errorf("获取密钥失败: %w", err)
			}
			return "", privKey, nil
		}
	}
	// 向后兼容：内联密码
	if cfg.AuthType == "password" && cfg.Password != "" {
		decrypted, err := credential_svc.Default().Decrypt(cfg.Password)
		if err != nil {
			return "", "", fmt.Errorf("解密密码失败: %w", err)
		}
		return decrypted, "", nil
	}
	// 向后兼容：本地密钥文件
	if cfg.AuthType == "key" && len(cfg.PrivateKeys) > 0 {
		data, err := os.ReadFile(cfg.PrivateKeys[0])
		if err != nil {
			return "", "", fmt.Errorf("读取私钥文件失败: %w", err)
		}
		return "", string(data), nil
	}
	return "", "", nil
}

// ResolveJumpHosts 递归解析跳板机链（含凭据解密），返回从第一跳到最后一跳的顺序
func (r *Resolver) ResolveJumpHosts(ctx context.Context, jumpHostID int64, maxDepth int) ([]ssh_svc.JumpHostEntry, error) {
	if maxDepth <= 0 {
		return nil, fmt.Errorf("跳板机链过深，可能存在循环引用")
	}

	jumpAsset, err := asset_svc.Asset().Get(ctx, jumpHostID)
	if err != nil {
		return nil, fmt.Errorf("跳板机资产不存在(ID=%d): %w", jumpHostID, err)
	}
	jumpCfg, err := jumpAsset.GetSSHConfig()
	if err != nil {
		return nil, err
	}

	// 解析跳板机凭证
	password, key, err := r.ResolveSSHCredentials(ctx, jumpCfg)
	if err != nil {
		return nil, fmt.Errorf("解析跳板机凭据失败: %w", err)
	}

	entry := ssh_svc.JumpHostEntry{
		Host:     jumpCfg.Host,
		Port:     jumpCfg.Port,
		Username: jumpCfg.Username,
		AuthType: jumpCfg.AuthType,
		Password: password,
		Key:      key,
	}

	if jumpCfg.JumpHostID > 0 {
		parentHosts, err := r.ResolveJumpHosts(ctx, jumpCfg.JumpHostID, maxDepth-1)
		if err != nil {
			return nil, err
		}
		return append(parentHosts, entry), nil
	}

	return []ssh_svc.JumpHostEntry{entry}, nil
}

// DecryptProxyPassword 解密代理配置中的密码，返回新的 ProxyConfig（不修改原始对象）
func (r *Resolver) DecryptProxyPassword(proxy *asset_entity.ProxyConfig) *asset_entity.ProxyConfig {
	if proxy == nil || proxy.Password == "" {
		return proxy
	}
	decrypted, err := credential_svc.Default().Decrypt(proxy.Password)
	if err != nil {
		// 解密失败，可能是明文（测试连接场景），原样返回
		return proxy
	}
	cp := *proxy
	cp.Password = decrypted
	return &cp
}

// ResolveSSHConnectConfig 从资产 ID 解析完整的 SSH 连接信息
// 返回 SSHConfig、明文密码、明文密钥、跳板机链
func (r *Resolver) ResolveSSHConnectConfig(ctx context.Context, assetID int64) (*asset_entity.SSHConfig, string, string, []ssh_svc.JumpHostEntry, error) {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsSSH() {
		return nil, "", "", nil, fmt.Errorf("资产不是SSH类型")
	}
	sshCfg, err := asset.GetSSHConfig()
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("获取SSH配置失败: %w", err)
	}
	password, key, err := r.ResolveSSHCredentials(ctx, sshCfg)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("解析凭据失败: %w", err)
	}

	var jumpHosts []ssh_svc.JumpHostEntry
	if sshCfg.JumpHostID > 0 {
		jumpHosts, err = r.ResolveJumpHosts(ctx, sshCfg.JumpHostID, 5)
		if err != nil {
			return nil, "", "", nil, fmt.Errorf("解析跳板机失败: %w", err)
		}
	}

	return sshCfg, password, key, jumpHosts, nil
}
