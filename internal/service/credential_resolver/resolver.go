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

// ResolveSSHCredentials 从 SSHConfig 解析明文密码/密钥/passphrase
// 优先使用统一凭证，向后兼容内联密码和本地密钥文件
func (r *Resolver) ResolveSSHCredentials(ctx context.Context, cfg *asset_entity.SSHConfig) (password, key, passphrase string, err error) {
	// 优先使用统一凭证
	if cfg.CredentialID > 0 {
		cred, err := credential_mgr_svc.Get(ctx, cfg.CredentialID)
		if err != nil {
			return "", "", "", fmt.Errorf("获取凭证失败: %w", err)
		}
		switch cred.Type {
		case credential_entity.TypePassword:
			decrypted, err := credential_svc.Default().Decrypt(cred.Password)
			if err != nil {
				return "", "", "", fmt.Errorf("解密密码失败: %w", err)
			}
			return decrypted, "", "", nil
		case credential_entity.TypeSSHKey:
			privKey, err := credential_mgr_svc.GetDecryptedPrivateKey(ctx, cfg.CredentialID)
			if err != nil {
				return "", "", "", fmt.Errorf("获取密钥失败: %w", err)
			}
			passphrase, err := credential_mgr_svc.GetDecryptedPassphrase(ctx, cfg.CredentialID)
			if err != nil {
				return "", "", "", fmt.Errorf("获取 passphrase 失败: %w", err)
			}
			return "", privKey, passphrase, nil
		}
	}
	// 向后兼容：内联密码
	if cfg.AuthType == "password" && cfg.Password != "" {
		decrypted, err := credential_svc.Default().Decrypt(cfg.Password)
		if err != nil {
			return "", "", "", fmt.Errorf("解密密码失败: %w", err)
		}
		return decrypted, "", "", nil
	}
	// 向后兼容：本地密钥文件
	if cfg.AuthType == "key" && len(cfg.PrivateKeys) > 0 {
		data, err := os.ReadFile(cfg.PrivateKeys[0])
		if err != nil {
			return "", "", "", fmt.Errorf("读取私钥文件失败: %w", err)
		}
		// 解密本地密钥的 passphrase
		var passphrase string
		if cfg.PrivateKeyPassphrase != "" {
			passphrase, err = credential_svc.Default().Decrypt(cfg.PrivateKeyPassphrase)
			if err != nil {
				return "", "", "", fmt.Errorf("解密私钥密码失败: %w", err)
			}
		}
		return "", string(data), passphrase, nil
	}
	return "", "", "", nil
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
	password, key, passphrase, err := r.ResolveSSHCredentials(ctx, jumpCfg)
	if err != nil {
		return nil, fmt.Errorf("解析跳板机凭据失败: %w", err)
	}

	entry := ssh_svc.JumpHostEntry{
		Host:       jumpCfg.Host,
		Port:       jumpCfg.Port,
		Username:   jumpCfg.Username,
		AuthType:   jumpCfg.AuthType,
		Password:   password,
		Key:        key,
		Passphrase: passphrase,
	}

	nextJumpID := jumpAsset.SSHTunnelID
	if nextJumpID == 0 {
		nextJumpID = jumpCfg.JumpHostID // backward compat
	}
	if nextJumpID > 0 {
		parentHosts, err := r.ResolveJumpHosts(ctx, nextJumpID, maxDepth-1)
		if err != nil {
			return nil, err
		}
		return append(parentHosts, entry), nil
	}

	return []ssh_svc.JumpHostEntry{entry}, nil
}

// PasswordSource 密码来源接口。
type PasswordSource interface {
	GetCredentialID() int64
	GetPassword() string
}

// ResolvePasswordGeneric 通用密码解密。
// 仅供 assettype handler 使用。现有 ResolveXxxPassword 方法保持不变。
func (r *Resolver) ResolvePasswordGeneric(ctx context.Context, src PasswordSource) (string, error) {
	if src.GetCredentialID() > 0 {
		password, err := credential_mgr_svc.GetDecryptedPassword(ctx, src.GetCredentialID())
		if err != nil {
			return "", fmt.Errorf("获取凭证失败: %w", err)
		}
		return password, nil
	}
	if src.GetPassword() == "" {
		return "", nil
	}
	decrypted, err := credential_svc.Default().Decrypt(src.GetPassword())
	if err != nil {
		return "", fmt.Errorf("解密密码失败: %w", err)
	}
	return decrypted, nil
}

// ResolveDatabasePassword 解密 DatabaseConfig 中的密码
// 优先使用统一凭证，向后兼容内联密码
func (r *Resolver) ResolveDatabasePassword(ctx context.Context, cfg *asset_entity.DatabaseConfig) (string, error) {
	if cfg.CredentialID > 0 {
		password, err := credential_mgr_svc.GetDecryptedPassword(ctx, cfg.CredentialID)
		if err != nil {
			return "", fmt.Errorf("获取数据库凭证失败: %w", err)
		}
		return password, nil
	}
	if cfg.Password == "" {
		return "", nil
	}
	decrypted, err := credential_svc.Default().Decrypt(cfg.Password)
	if err != nil {
		return "", fmt.Errorf("解密数据库密码失败: %w", err)
	}
	return decrypted, nil
}

// ResolveRedisPassword 解密 RedisConfig 中的密码
// 优先使用统一凭证，向后兼容内联密码
func (r *Resolver) ResolveRedisPassword(ctx context.Context, cfg *asset_entity.RedisConfig) (string, error) {
	if cfg.CredentialID > 0 {
		password, err := credential_mgr_svc.GetDecryptedPassword(ctx, cfg.CredentialID)
		if err != nil {
			return "", fmt.Errorf("获取 Redis 凭证失败: %w", err)
		}
		return password, nil
	}
	if cfg.Password == "" {
		return "", nil
	}
	decrypted, err := credential_svc.Default().Decrypt(cfg.Password)
	if err != nil {
		return "", fmt.Errorf("解密 Redis 密码失败: %w", err)
	}
	return decrypted, nil
}

// ResolveMongoDBPassword 解密 MongoDBConfig 中的密码
// 优先使用统一凭证，向后兼容内联密码
func (r *Resolver) ResolveMongoDBPassword(ctx context.Context, cfg *asset_entity.MongoDBConfig) (string, error) {
	if cfg.CredentialID > 0 {
		password, err := credential_mgr_svc.GetDecryptedPassword(ctx, cfg.CredentialID)
		if err != nil {
			return "", fmt.Errorf("获取 MongoDB 凭证失败: %w", err)
		}
		return password, nil
	}
	if cfg.Password == "" {
		return "", nil
	}
	decrypted, err := credential_svc.Default().Decrypt(cfg.Password)
	if err != nil {
		return "", fmt.Errorf("解密 MongoDB 密码失败: %w", err)
	}
	return decrypted, nil
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
// 返回 SSHConfig、明文密码、明文密钥、passphrase、跳板机链
func (r *Resolver) ResolveSSHConnectConfig(ctx context.Context, assetID int64) (*asset_entity.SSHConfig, string, string, string, []ssh_svc.JumpHostEntry, error) {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil, "", "", "", nil, fmt.Errorf("资产不存在: %w", err)
	}
	if !asset.IsSSH() {
		return nil, "", "", "", nil, fmt.Errorf("资产不是SSH类型")
	}
	sshCfg, err := asset.GetSSHConfig()
	if err != nil {
		return nil, "", "", "", nil, fmt.Errorf("获取SSH配置失败: %w", err)
	}
	password, key, passphrase, err := r.ResolveSSHCredentials(ctx, sshCfg)
	if err != nil {
		return nil, "", "", "", nil, fmt.Errorf("解析凭据失败: %w", err)
	}

	var jumpHosts []ssh_svc.JumpHostEntry
	jumpHostID := asset.SSHTunnelID
	if jumpHostID == 0 {
		jumpHostID = sshCfg.JumpHostID // backward compat
	}
	if jumpHostID > 0 {
		jumpHosts, err = r.ResolveJumpHosts(ctx, jumpHostID, 5)
		if err != nil {
			return nil, "", "", "", nil, fmt.Errorf("解析跳板机失败: %w", err)
		}
	}

	return sshCfg, password, key, passphrase, jumpHosts, nil
}
