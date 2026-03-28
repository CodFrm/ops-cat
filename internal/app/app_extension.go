package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/extension"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/model/entity/policy_group_entity"
	"github.com/opskat/opskat/internal/repository/extension_data_repo"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/credential_resolver"
	"github.com/opskat/opskat/internal/service/credential_svc"

	"github.com/cago-frame/cago/configs"
	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

// ExtensionListItem 返回给前端的扩展信息
type ExtensionListItem struct {
	Name          string                   `json:"name"`
	DisplayName   string                   `json:"displayName"`
	DisplayNameZh string                   `json:"displayName_zh"`
	Version       string                   `json:"version"`
	Icon          string                   `json:"icon"`
	Description   string                   `json:"description"`
	DescriptionZh string                   `json:"description_zh"`
	AssetTypes    []extension.AssetTypeDef `json:"assetTypes"`
	Pages         []extension.PageDef      `json:"pages"`
	PolicyType    string                   `json:"policyType,omitempty"`
	PolicyActions []string                 `json:"policyActions,omitempty"`
}

// GetExtensions 返回已加载的扩展列表（Wails binding）
func (a *App) GetExtensions() []ExtensionListItem {
	if a.extManager == nil {
		return nil
	}
	exts := a.extManager.Extensions()
	items := make([]ExtensionListItem, 0, len(exts))
	for _, ext := range exts {
		items = append(items, ExtensionListItem{
			Name:          ext.Manifest.Name,
			DisplayName:   ext.Manifest.DisplayName,
			DisplayNameZh: ext.Manifest.DisplayNameZh,
			Version:       ext.Manifest.Version,
			Icon:          ext.Manifest.Icon,
			Description:   ext.Manifest.Description,
			DescriptionZh: ext.Manifest.DescriptionZh,
			AssetTypes:    ext.Manifest.AssetTypes,
			Pages:         ext.Manifest.Frontend.Pages,
			PolicyType:    ext.Manifest.Policies.Type,
			PolicyActions: ext.Manifest.Policies.Actions,
		})
	}
	return items
}

// SelectExtensionDir 弹出目录选择对话框，返回所选目录路径（Wails binding）
func (a *App) SelectExtensionDir() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择扩展目录",
	})
}

// InstallExtension 从本地路径安装扩展（Wails binding）
func (a *App) InstallExtension(sourcePath string) error {
	if a.extManager == nil {
		return fmt.Errorf("extension system not initialized")
	}
	info, err := a.extManager.Install(sourcePath)
	if err != nil {
		return err
	}
	// 注册扩展声明的资产类型
	if info != nil {
		for _, at := range info.Manifest.AssetTypes {
			asset_entity.RegisterExtensionAssetType(at.Type)
		}
	}
	return nil
}

// RemoveExtension 卸载扩展（Wails binding）
func (a *App) RemoveExtension(name string) error {
	if a.extManager == nil {
		return fmt.Errorf("extension system not initialized")
	}
	// 卸载前注销扩展声明的资产类型和权限组
	ext := a.extManager.GetExtension(name)
	if ext != nil {
		for _, at := range ext.Manifest.AssetTypes {
			asset_entity.UnregisterExtensionAssetType(at.Type)
		}
		unregisterExtensionPolicyGroups(ext)
	}
	return a.extManager.Remove(name)
}

// TestExtensionConnection 测试扩展资产类型的连接（Wails binding）
// configJSON 是前端表单的原始配置（密码字段为明文）
func (a *App) TestExtensionConnection(assetType string, configJSON string) error {
	if a.extHost == nil || a.extManager == nil {
		return fmt.Errorf("extension system not initialized")
	}
	// 查找声明该 assetType 的扩展
	var extName string
	for _, ext := range a.extManager.Extensions() {
		for _, at := range ext.Manifest.AssetTypes {
			if at.Type == assetType {
				extName = ext.Manifest.Name
				break
			}
		}
		if extName != "" {
			break
		}
	}
	if extName == "" {
		return fmt.Errorf("no extension found for asset type %q", assetType)
	}
	plugin := a.extHost.GetPlugin(extName)
	if plugin == nil {
		return fmt.Errorf("extension %q plugin not loaded", extName)
	}
	ctx := a.langCtx()
	return plugin.CallTestConnection(ctx, json.RawMessage(configJSON))
}

// CallExtensionTool 调用扩展工具（Wails binding）
func (a *App) CallExtensionTool(name string, argsJSON string) (string, error) {
	if a.extBridge == nil {
		return "", fmt.Errorf("extension system not initialized")
	}
	extName, toolName, ok := extension.ParseExtensionToolName(name)
	if !ok {
		return "", fmt.Errorf("invalid extension tool name %q, expected format: ext.tool", name)
	}
	ctx := a.langCtx()
	return a.extBridge.ExecuteTool(ctx, extName, toolName, json.RawMessage(argsJSON))
}

// initExtensions 启动时加载扩展系统
func (a *App) initExtensions() {
	dataDir := bootstrap.AppDataDir()
	extensionsDir := filepath.Join(dataDir, "extensions")

	a.extManager = extension.NewManager(extensionsDir, configs.Version)
	if err := a.extManager.Scan(); err != nil {
		logger.Default().Warn("scan extensions", zap.Error(err))
		return
	}

	// 注册扩展声明的资产类型和默认权限组
	for _, ext := range a.extManager.Extensions() {
		for _, at := range ext.Manifest.AssetTypes {
			asset_entity.RegisterExtensionAssetType(at.Type)
		}
		registerExtensionPolicyGroups(ext)
	}

	services := &appHostServices{app: a}

	a.extHost = extension.NewExtensionHost(services)
	ctx := context.Background()
	for _, ext := range a.extManager.Extensions() {
		if _, err := a.extHost.LoadPlugin(ctx, ext); err != nil {
			logger.Default().Warn("load extension plugin",
				zap.String("extension", ext.Manifest.Name),
				zap.Error(err),
			)
			continue
		}
		logger.Default().Info("loaded extension",
			zap.String("name", ext.Manifest.Name),
			zap.String("version", ext.Manifest.Version),
		)
	}

	a.extBridge = extension.NewBridge(a.extHost)
	for _, ext := range a.extManager.Extensions() {
		if a.extHost.GetPlugin(ext.Manifest.Name) != nil {
			a.extBridge.RegisterExtension(ext)
		}
	}
}

// registerExtensionPolicyGroups 注册扩展的默认权限组到内存
func registerExtensionPolicyGroups(ext *extension.ExtensionInfo) {
	if len(ext.Manifest.Policies.DefaultGroups) == 0 {
		return
	}

	// 注册扩展策略类型
	if ext.Manifest.Policies.Type != "" {
		policy_group_entity.RegisterExtensionPolicyType(ext.Manifest.Policies.Type)
	}

	groups := make([]*policy_group_entity.PolicyGroup, 0, len(ext.Manifest.Policies.DefaultGroups))
	for _, def := range ext.Manifest.Policies.DefaultGroups {
		policyJSON, err := json.Marshal(&policy.ExtensionPolicy{
			AllowList: def.Policy.AllowList,
			DenyList:  def.Policy.DenyList,
		})
		if err != nil {
			logger.Default().Warn("marshal extension policy group", zap.String("ext", ext.Manifest.Name), zap.Error(err))
			continue
		}
		groups = append(groups, &policy_group_entity.PolicyGroup{
			StringID:      fmt.Sprintf("ext.%s.%s", ext.Manifest.Name, def.Slug),
			Name:          def.Name,
			NameZh:        def.NameZh,
			Description:   def.Description,
			DescriptionZh: def.DescriptionZh,
			PolicyType:    ext.Manifest.Policies.Type,
			Policy:        string(policyJSON),
		})
	}
	policy_group_entity.RegisterExtensionGroups(ext.Manifest.Name, groups)
}

// unregisterExtensionPolicyGroups 注销扩展的权限组和策略类型
func unregisterExtensionPolicyGroups(ext *extension.ExtensionInfo) {
	if ext.Manifest.Policies.Type != "" {
		policy_group_entity.UnregisterExtensionPolicyType(ext.Manifest.Policies.Type)
	}
	policy_group_entity.UnregisterExtensionGroups(ext.Manifest.Name)
}

// getExtensionDefaultPolicy 获取扩展类型的默认策略 JSON
func getExtensionDefaultPolicy(assetType string) string {
	// 遍历扩展权限组，找到匹配 asset type 的扩展默认组
	extGroups := policy_group_entity.ExtensionGroups()
	var groupIDs []string
	for _, pg := range extGroups {
		if pg.PolicyType == assetType {
			groupIDs = append(groupIDs, pg.StringID)
		}
	}
	if len(groupIDs) == 0 {
		return ""
	}
	data, err := json.Marshal(map[string]any{
		"groups": groupIDs,
	})
	if err != nil {
		return ""
	}
	return string(data)
}

// appHostServices 实现 extension.HostServices
type appHostServices struct {
	app *App
}

func (s *appHostServices) KVGet(ctx context.Context, ext, key string) ([]byte, error) {
	data, err := extension_data_repo.ExtensionData().Get(ctx, ext, key)
	if err != nil {
		return nil, err
	}
	return data.Value, nil
}

func (s *appHostServices) KVSet(ctx context.Context, ext, key string, value []byte) error {
	return extension_data_repo.ExtensionData().Set(ctx, ext, key, value)
}

func (s *appHostServices) EmitEvent(event string, data any) {
	if s.app.ctx != nil {
		wailsRuntime.EventsEmit(s.app.ctx, event, data)
	}
}

func (s *appHostServices) HTTPRequest(ctx context.Context, method, url string, headers map[string]string, body []byte) (int, map[string]string, []byte, error) {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Default().Warn("close http response body", zap.Error(closeErr))
		}
	}()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return 0, nil, nil, err
	}
	respHeaders := make(map[string]string, len(resp.Header))
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}
	return resp.StatusCode, respHeaders, respBody, nil
}

func (s *appHostServices) GetCredential(ctx context.Context, assetID int64) (map[string]string, error) {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	switch {
	case asset.IsSSH():
		sshCfg, err := asset.GetSSHConfig()
		if err != nil {
			return nil, err
		}
		password, key, err := credential_resolver.Default().ResolveSSHCredentials(ctx, sshCfg)
		if err != nil {
			return nil, err
		}
		result["username"] = sshCfg.Username
		result["password"] = password
		if key != "" {
			result["private_key"] = key
		}
	case asset.IsDatabase():
		dbCfg, err := asset.GetDatabaseConfig()
		if err != nil {
			return nil, err
		}
		password, err := credential_resolver.Default().ResolveDatabasePassword(ctx, dbCfg)
		if err != nil {
			return nil, err
		}
		result["username"] = dbCfg.Username
		result["password"] = password
	case asset.IsRedis():
		redisCfg, err := asset.GetRedisConfig()
		if err != nil {
			return nil, err
		}
		password, err := credential_resolver.Default().ResolveRedisPassword(ctx, redisCfg)
		if err != nil {
			return nil, err
		}
		result["password"] = password
	}
	return result, nil
}

func (s *appHostServices) GetAssetConfig(ctx context.Context, assetID int64, extName string) (json.RawMessage, error) {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil, err
	}
	if asset.Config == "" {
		return json.RawMessage("{}"), nil
	}

	// Auto-decrypt password-format fields based on the extension's configSchema
	configJSON := json.RawMessage(asset.Config)
	if s.app.extManager != nil {
		ext := s.app.extManager.GetExtension(extName)
		if ext != nil {
			configJSON = s.decryptPasswordFields(configJSON, ext)
		}
	}
	return configJSON, nil
}

// decryptPasswordFields 根据 configSchema 中 format=password 的字段自动解密
func (s *appHostServices) decryptPasswordFields(configJSON json.RawMessage, ext *extension.ExtensionInfo) json.RawMessage {
	if len(ext.Manifest.AssetTypes) == 0 {
		return configJSON
	}

	// Collect password field names from all assetType configSchemas
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
	if len(pwdFields) == 0 {
		return configJSON
	}

	// Parse config, decrypt password fields
	var configMap map[string]json.RawMessage
	if json.Unmarshal(configJSON, &configMap) != nil {
		return configJSON
	}

	changed := false
	for field := range pwdFields {
		raw, ok := configMap[field]
		if !ok {
			continue
		}
		var encrypted string
		if json.Unmarshal(raw, &encrypted) != nil || encrypted == "" {
			continue
		}
		decrypted, err := credential_svc.Default().Decrypt(encrypted)
		if err != nil {
			// Not encrypted or invalid — return as-is
			continue
		}
		newVal, _ := json.Marshal(decrypted)
		configMap[field] = newVal
		changed = true
	}
	if !changed {
		return configJSON
	}
	result, err := json.Marshal(configMap)
	if err != nil {
		return configJSON
	}
	return result
}
