package app

import (
	"context"
	"path/filepath"

	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/extension"
	"github.com/opskat/opskat/internal/repository/extension_data_repo"

	"github.com/cago-frame/cago/configs"
	"github.com/cago-frame/cago/pkg/logger"
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
	// Phase 2 实现前端事件推送
}
