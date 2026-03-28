package app

import (
	"encoding/json"
	"fmt"

	"github.com/opskat/opskat/internal/extension"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/credential_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/service/credential_mgr_svc"
	"github.com/opskat/opskat/internal/service/credential_resolver"
	"github.com/opskat/opskat/internal/service/credential_svc"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// --- 凭证操作 ---

// EncryptPassword 加密密码，返回加密后的字符串（用于前端保存资产配置）
func (a *App) EncryptPassword(plaintext string) (string, error) {
	return credential_svc.Default().Encrypt(plaintext)
}

// GetAssetPassword 获取指定资产的解密密码（用于编辑时回看密码）
func (a *App) GetAssetPassword(assetID int64) (string, error) {
	ctx := a.langCtx()
	asset, err := asset_repo.Asset().Find(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("asset not found: %w", err)
	}

	resolver := credential_resolver.Default()
	switch asset.Type {
	case asset_entity.AssetTypeSSH:
		cfg, err := asset.GetSSHConfig()
		if err != nil {
			return "", fmt.Errorf("get SSH config failed: %w", err)
		}
		password, _, err := resolver.ResolveSSHCredentials(ctx, cfg)
		return password, err

	case asset_entity.AssetTypeDatabase:
		cfg, err := asset.GetDatabaseConfig()
		if err != nil {
			return "", fmt.Errorf("get database config failed: %w", err)
		}
		return resolver.ResolveDatabasePassword(ctx, cfg)

	case asset_entity.AssetTypeRedis:
		cfg, err := asset.GetRedisConfig()
		if err != nil {
			return "", fmt.Errorf("get Redis config failed: %w", err)
		}
		return resolver.ResolveRedisPassword(ctx, cfg)

	default:
		// Extension asset type — decrypt the primary password field
		return a.getExtensionAssetPassword(asset, "")
	}
}

// GetExtensionAssetPassword 获取扩展资产指定字段的解密密码
func (a *App) GetExtensionAssetPassword(assetID int64, fieldName string) (string, error) {
	ctx := a.langCtx()
	asset, err := asset_repo.Asset().Find(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("asset not found: %w", err)
	}
	return a.getExtensionAssetPassword(asset, fieldName)
}

func (a *App) getExtensionAssetPassword(asset *asset_entity.Asset, fieldName string) (string, error) {
	if asset.Config == "" {
		return "", nil
	}
	if a.extManager == nil {
		return "", fmt.Errorf("extension system not initialized")
	}

	// Find the extension that owns this asset type
	var ext *extension.ExtensionInfo
	for _, e := range a.extManager.Extensions() {
		for _, at := range e.Manifest.AssetTypes {
			if at.Type == asset.Type {
				ext = e
				break
			}
		}
		if ext != nil {
			break
		}
	}
	if ext == nil {
		return "", fmt.Errorf("extension not found for asset type: %s", asset.Type)
	}

	// Find password fields from configSchema
	pwdFields := make(map[string]bool)
	for _, at := range ext.Manifest.AssetTypes {
		if at.ConfigSchema == nil {
			continue
		}
		var schema struct {
			Properties map[string]struct {
				Format string `json:"format"`
			} `json:"properties"`
		}
		if json.Unmarshal(at.ConfigSchema, &schema) != nil {
			continue
		}
		for name, prop := range schema.Properties {
			if prop.Format == "password" {
				pwdFields[name] = true
			}
		}
	}

	if fieldName != "" {
		if !pwdFields[fieldName] {
			return "", fmt.Errorf("field %q is not a password field", fieldName)
		}
	}

	var configMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(asset.Config), &configMap); err != nil {
		return "", fmt.Errorf("parse config failed: %w", err)
	}

	// If fieldName specified, decrypt that field; otherwise decrypt the first password field
	if fieldName == "" {
		for f := range pwdFields {
			fieldName = f
			break
		}
	}
	if fieldName == "" {
		return "", nil
	}

	raw, ok := configMap[fieldName]
	if !ok {
		return "", nil
	}
	var encrypted string
	_ = json.Unmarshal(raw, &encrypted)
	if encrypted == "" {
		return "", nil
	}
	return credential_svc.Default().Decrypt(encrypted)
}

// --- 密钥管理 ---

// ListCredentials 列出所有凭证
func (a *App) ListCredentials() ([]*credential_entity.Credential, error) {
	return credential_mgr_svc.List(a.langCtx())
}

// ListCredentialsByType 按类型列出凭证
func (a *App) ListCredentialsByType(credType string) ([]*credential_entity.Credential, error) {
	return credential_mgr_svc.ListByType(a.langCtx(), credType)
}

// CreatePasswordCredential 创建密码凭证
func (a *App) CreatePasswordCredential(name, username, password, description string) (*credential_entity.Credential, error) {
	return credential_mgr_svc.CreatePassword(a.langCtx(), credential_mgr_svc.CreatePasswordRequest{
		Name:        name,
		Username:    username,
		Password:    password,
		Description: description,
	})
}

// GenerateSSHKey 生成新的 SSH 密钥对
func (a *App) GenerateSSHKey(name, comment, keyType string, keySize int) (*credential_entity.Credential, error) {
	return credential_mgr_svc.GenerateSSHKey(a.langCtx(), credential_mgr_svc.GenerateKeyRequest{
		Name:    name,
		Comment: comment,
		KeyType: keyType,
		KeySize: keySize,
	})
}

// ImportSSHKeyFile 通过文件选择框导入 SSH 密钥
func (a *App) ImportSSHKeyFile(name, comment string) (*credential_entity.Credential, error) {
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择 SSH 私钥文件",
	})
	if err != nil {
		return nil, fmt.Errorf("打开文件对话框失败: %w", err)
	}
	if filePath == "" {
		return nil, nil
	}
	return credential_mgr_svc.ImportSSHKeyFromFile(a.langCtx(), name, comment, filePath)
}

// ImportSSHKeyPEM 通过粘贴 PEM 内容导入 SSH 密钥
func (a *App) ImportSSHKeyPEM(name, comment, pemData string) (*credential_entity.Credential, error) {
	return credential_mgr_svc.ImportSSHKeyFromPEM(a.langCtx(), name, comment, pemData)
}

// UpdateCredential 更新凭证
func (a *App) UpdateCredential(id int64, name, comment, description, username string) (*credential_entity.Credential, error) {
	return credential_mgr_svc.Update(a.langCtx(), credential_mgr_svc.UpdateRequest{
		ID:          id,
		Name:        name,
		Comment:     comment,
		Description: description,
		Username:    username,
	})
}

// UpdateCredentialPassword 更新密码凭证的密码
func (a *App) UpdateCredentialPassword(id int64, password string) error {
	return credential_mgr_svc.UpdatePassword(a.langCtx(), id, password)
}

// GetCredentialUsage 获取引用此凭证的资产名称列表
func (a *App) GetCredentialUsage(id int64) ([]string, error) {
	assets, err := asset_repo.Asset().FindByCredentialID(a.langCtx(), id)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(assets))
	for i, asset := range assets {
		names[i] = asset.Name
	}
	return names, nil
}

// DeleteCredential 删除凭证
func (a *App) DeleteCredential(id int64) error {
	return credential_mgr_svc.Delete(a.langCtx(), id)
}

// GetCredentialPublicKey 获取 SSH 密钥凭证的公钥（用于复制）
func (a *App) GetCredentialPublicKey(id int64) (string, error) {
	cred, err := credential_mgr_svc.Get(a.langCtx(), id)
	if err != nil {
		return "", err
	}
	return cred.PublicKey, nil
}
