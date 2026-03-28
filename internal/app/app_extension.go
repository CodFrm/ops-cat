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
	"github.com/opskat/opskat/internal/repository/extension_data_repo"
	"github.com/opskat/opskat/internal/service/asset_svc"
	"github.com/opskat/opskat/internal/service/credential_resolver"

	"github.com/cago-frame/cago/configs"
	"github.com/cago-frame/cago/pkg/logger"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

// ExtensionListItem 返回给前端的扩展信息
type ExtensionListItem struct {
	Name        string                   `json:"name"`
	DisplayName string                   `json:"displayName"`
	Version     string                   `json:"version"`
	Icon        string                   `json:"icon"`
	Description string                   `json:"description"`
	AssetTypes  []extension.AssetTypeDef `json:"assetTypes"`
	Pages       []extension.PageDef      `json:"pages"`
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
			Name:        ext.Manifest.Name,
			DisplayName: ext.Manifest.DisplayName,
			Version:     ext.Manifest.Version,
			Icon:        ext.Manifest.Icon,
			Description: ext.Manifest.Description,
			AssetTypes:  ext.Manifest.AssetTypes,
			Pages:       ext.Manifest.Frontend.Pages,
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
	_, err := a.extManager.Install(sourcePath)
	return err
}

// RemoveExtension 卸载扩展（Wails binding）
func (a *App) RemoveExtension(name string) error {
	if a.extManager == nil {
		return fmt.Errorf("extension system not initialized")
	}
	return a.extManager.Remove(name)
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

func (s *appHostServices) GetAssetConfig(ctx context.Context, assetID int64) (json.RawMessage, error) {
	asset, err := asset_svc.Asset().Get(ctx, assetID)
	if err != nil {
		return nil, err
	}
	if asset.Config == "" {
		return json.RawMessage("{}"), nil
	}
	return json.RawMessage(asset.Config), nil
}
