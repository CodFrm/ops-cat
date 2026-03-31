package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/model/entity/extension_state_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/extension_data_repo"
	"github.com/opskat/opskat/internal/repository/extension_state_repo"
	"github.com/opskat/opskat/pkg/extension"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

// ExtensionInfo is the frontend-facing extension descriptor.
type ExtensionInfo struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Icon        string              `json:"icon"`
	DisplayName string              `json:"displayName"`
	Description string              `json:"description"`
	Enabled     bool                `json:"enabled"`
	Manifest    *extension.Manifest `json:"manifest"`
}

// AssetTypeInfo combines built-in and extension asset types for the frontend.
type AssetTypeInfo struct {
	Type          string `json:"type"`
	ExtensionName string `json:"extensionName,omitempty"`
	DisplayName   string `json:"displayName"`
	SSHTunnel     bool   `json:"sshTunnel"`
}

// ListInstalledExtensions returns all loaded extensions.
func (a *App) ListInstalledExtensions() []ExtensionInfo {
	if a.extManager == nil {
		return nil
	}

	// Build set of loaded (enabled) extensions
	loaded := make(map[string]*extension.Extension)
	for _, ext := range a.extManager.ListExtensions() {
		loaded[ext.Name] = ext
	}

	// Scan all manifests from disk (includes disabled ones)
	allManifests, err := a.extManager.ScanManifests()
	if err != nil {
		zap.L().Warn("scan manifests failed", zap.Error(err))
		// Fall back to only loaded extensions
		result := make([]ExtensionInfo, 0, len(loaded))
		for _, ext := range loaded {
			result = append(result, ExtensionInfo{
				Name:        ext.Name,
				Version:     ext.Manifest.Version,
				Icon:        ext.Manifest.Icon,
				DisplayName: ext.Manifest.I18n.DisplayName,
				Description: ext.Manifest.I18n.Description,
				Enabled:     true,
				Manifest:    ext.Manifest,
			})
		}
		return result
	}

	result := make([]ExtensionInfo, 0, len(allManifests))
	for _, mi := range allManifests {
		ext, isLoaded := loaded[mi.Name]
		info := ExtensionInfo{
			Name:        mi.Name,
			Version:     mi.Manifest.Version,
			Icon:        mi.Manifest.Icon,
			DisplayName: mi.Manifest.I18n.DisplayName,
			Description: mi.Manifest.I18n.Description,
			Enabled:     isLoaded,
			Manifest:    mi.Manifest,
		}
		if isLoaded {
			info.Manifest = ext.Manifest
		}
		result = append(result, info)
	}
	return result
}

// GetExtensionManifest returns a single extension's manifest.
func (a *App) GetExtensionManifest(name string) (*extension.Manifest, error) {
	if a.extManager == nil {
		return nil, fmt.Errorf("extension system not initialized")
	}
	ext := a.extManager.GetExtension(name)
	if ext == nil {
		return nil, fmt.Errorf("extension %q not found", name)
	}
	return ext.Manifest, nil
}

// GetAvailableAssetTypes returns built-in + extension asset types.
func (a *App) GetAvailableAssetTypes() []AssetTypeInfo {
	types := []AssetTypeInfo{
		{Type: "ssh", DisplayName: "SSH"},
		{Type: "database", DisplayName: "Database"},
		{Type: "redis", DisplayName: "Redis"},
	}
	if a.extBridge != nil {
		for _, at := range a.extBridge.GetAssetTypes() {
			types = append(types, AssetTypeInfo{
				Type:          at.Type,
				ExtensionName: at.ExtensionName,
				DisplayName:   at.I18n.Name,
				SSHTunnel:     true,
			})
		}
	}
	return types
}

// CallExtensionAction calls an extension action and streams events via Wails Events.
func (a *App) CallExtensionAction(extName, action string, argsJSON string) (string, error) {
	if a.extBridge == nil {
		return "", fmt.Errorf("extension system not initialized")
	}
	ext := a.extManager.GetExtension(extName)
	if ext == nil {
		return "", fmt.Errorf("extension %q not loaded", extName)
	}
	if ext.Plugin == nil {
		return "", fmt.Errorf("extension %q has no backend plugin", extName)
	}

	var args json.RawMessage
	if argsJSON != "" {
		args = json.RawMessage(argsJSON)
	} else {
		args = json.RawMessage("{}")
	}

	result, err := ext.Plugin.CallAction(a.langCtx(), action, args)
	if err != nil {
		return "", fmt.Errorf("call action %s/%s: %w", extName, action, err)
	}
	return string(result), nil
}

// CallExtensionTool calls an extension tool (for frontend config testing etc.)
func (a *App) CallExtensionTool(extName, tool string, argsJSON string) (string, error) {
	if a.extBridge == nil {
		return "", fmt.Errorf("extension system not initialized")
	}
	ext := a.extManager.GetExtension(extName)
	if ext == nil {
		return "", fmt.Errorf("extension %q not loaded", extName)
	}
	if ext.Plugin == nil {
		return "", fmt.Errorf("extension %q has no backend plugin", extName)
	}

	var args json.RawMessage
	if argsJSON != "" {
		args = json.RawMessage(argsJSON)
	} else {
		args = json.RawMessage("{}")
	}

	result, err := ext.Plugin.CallTool(a.langCtx(), tool, args)
	if err != nil {
		return "", fmt.Errorf("call tool %s/%s: %w", extName, tool, err)
	}
	return string(result), nil
}

// GetDecryptedExtensionConfig returns the asset config with password fields decrypted.
// Used by the frontend when editing an extension-type asset.
func (a *App) GetDecryptedExtensionConfig(assetID int64, extName string) (string, error) {
	ctx := context.Background()
	asset, err := asset_repo.Asset().Find(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("asset %d not found: %w", assetID, err)
	}
	if asset.Config == "" {
		return "{}", nil
	}
	raw := json.RawMessage(asset.Config)
	decrypted, err := decryptConfigPasswordFields(raw, asset.Type, a.extBridge)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}

// InstallExtension opens a file dialog and installs an extension from zip or directory.
func (a *App) InstallExtension() (*ExtensionInfo, error) {
	if a.extManager == nil {
		return nil, fmt.Errorf("extension system not initialized")
	}

	selected, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Extension",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Extension Package (*.zip)", Pattern: "*.zip"},
			{DisplayName: "All Files", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("file dialog: %w", err)
	}
	if selected == "" {
		return nil, nil // user cancelled
	}

	// Check if selected path is a directory
	info, err := os.Stat(selected)
	if err != nil {
		return nil, fmt.Errorf("stat selected path: %w", err)
	}
	sourcePath := selected
	if info.IsDir() {
		// Directory selected — use directly
	} else if !strings.HasSuffix(strings.ToLower(selected), ".zip") {
		return nil, fmt.Errorf("unsupported file type: %s", selected)
	}

	manifest, err := a.extManager.Install(a.langCtx(), sourcePath)
	if err != nil {
		return nil, fmt.Errorf("install extension: %w", err)
	}

	// Register in bridge
	ext := a.extManager.GetExtension(manifest.Name)
	if ext != nil {
		a.extBridge.Register(ext)
		ai.SetExecToolExecutor(a.extBridge)
	}

	// Save enabled state
	a.ensureExtensionState(manifest.Name, true)

	wailsRuntime.EventsEmit(a.ctx, "ext:reload", nil)

	return &ExtensionInfo{
		Name:        manifest.Name,
		Version:     manifest.Version,
		Icon:        manifest.Icon,
		DisplayName: manifest.I18n.DisplayName,
		Description: manifest.I18n.Description,
		Enabled:     true,
		Manifest:    manifest,
	}, nil
}

// UninstallExtension removes an extension and optionally cleans up its data.
func (a *App) UninstallExtension(name string, cleanData bool) error {
	if a.extManager == nil {
		return fmt.Errorf("extension system not initialized")
	}

	// Unregister from bridge first
	a.extBridge.Unregister(name)
	ai.SetExecToolExecutor(a.extBridge)

	// Uninstall (unload + remove directory)
	if err := a.extManager.Uninstall(a.langCtx(), name); err != nil {
		return fmt.Errorf("uninstall extension: %w", err)
	}

	// Clean database records
	ctx := context.Background()
	extension_state_repo.ExtensionState().Delete(ctx, name)
	if cleanData {
		extension_data_repo.ExtensionData().DeleteAll(ctx, name)
	}

	wailsRuntime.EventsEmit(a.ctx, "ext:reload", nil)
	return nil
}

// EnableExtension loads a disabled extension and registers it.
func (a *App) EnableExtension(name string) error {
	if a.extManager == nil {
		return fmt.Errorf("extension system not initialized")
	}

	// Check if already loaded
	if ext := a.extManager.GetExtension(name); ext != nil {
		return nil // already enabled
	}

	dir := a.extManager.ExtDir(name)
	if _, err := a.extManager.LoadExtension(a.langCtx(), dir); err != nil {
		return fmt.Errorf("load extension: %w", err)
	}

	ext := a.extManager.GetExtension(name)
	if ext != nil {
		a.extBridge.Register(ext)
		ai.SetExecToolExecutor(a.extBridge)
	}

	a.ensureExtensionState(name, true)

	wailsRuntime.EventsEmit(a.ctx, "ext:reload", nil)
	return nil
}

// DisableExtension unloads a running extension without removing files.
func (a *App) DisableExtension(name string) error {
	if a.extManager == nil {
		return fmt.Errorf("extension system not initialized")
	}

	a.extBridge.Unregister(name)
	_ = a.extManager.Unload(a.langCtx(), name)
	ai.SetExecToolExecutor(a.extBridge)

	a.ensureExtensionState(name, false)

	wailsRuntime.EventsEmit(a.ctx, "ext:reload", nil)
	return nil
}

// GetExtensionDetail returns the full manifest and state for a single extension.
func (a *App) GetExtensionDetail(name string) (*ExtensionInfo, error) {
	if a.extManager == nil {
		return nil, fmt.Errorf("extension system not initialized")
	}

	// Try loaded extension first
	ext := a.extManager.GetExtension(name)
	if ext != nil {
		return &ExtensionInfo{
			Name:        ext.Name,
			Version:     ext.Manifest.Version,
			Icon:        ext.Manifest.Icon,
			DisplayName: ext.Manifest.I18n.DisplayName,
			Description: ext.Manifest.I18n.Description,
			Enabled:     true,
			Manifest:    ext.Manifest,
		}, nil
	}

	// Try reading manifest from disk (disabled extension)
	dir := a.extManager.ExtDir(name)
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("extension %q not found", name)
	}
	manifest, err := extension.ParseManifest(data)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return &ExtensionInfo{
		Name:        manifest.Name,
		Version:     manifest.Version,
		Icon:        manifest.Icon,
		DisplayName: manifest.I18n.DisplayName,
		Description: manifest.I18n.Description,
		Enabled:     false,
		Manifest:    manifest,
	}, nil
}

// ensureExtensionState creates or updates the extension_state record.
func (a *App) ensureExtensionState(name string, enabled bool) {
	ctx := context.Background()
	state, err := extension_state_repo.ExtensionState().Find(ctx, name)
	if err != nil {
		// Not found, create
		extension_state_repo.ExtensionState().Create(ctx, &extension_state_entity.ExtensionState{
			Name:    name,
			Enabled: enabled,
		})
		return
	}
	state.Enabled = enabled
	extension_state_repo.ExtensionState().Update(ctx, state)
}

// ReloadExtensions re-scans extensions directory and updates the bridge.
func (a *App) ReloadExtensions() error {
	if a.extManager == nil {
		return fmt.Errorf("extension system not initialized")
	}

	a.extManager.Close(a.langCtx())

	if _, err := a.extManager.Scan(a.langCtx()); err != nil {
		return fmt.Errorf("scan extensions: %w", err)
	}

	a.extBridge = extension.NewBridge()
	for _, ext := range a.extManager.ListExtensions() {
		a.extBridge.Register(ext)
	}

	ai.SetExecToolExecutor(a.extBridge)

	wailsRuntime.EventsEmit(a.ctx, "ext:reload", nil)
	zap.L().Info("extensions reloaded")
	return nil
}
