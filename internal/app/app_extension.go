package app

import (
	"encoding/json"
	"fmt"

	"github.com/opskat/opskat/internal/service/extension_svc"
	"github.com/opskat/opskat/pkg/extension"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// AssetTypeInfo combines built-in and extension asset types for the frontend.
type AssetTypeInfo struct {
	Type          string `json:"type"`
	ExtensionName string `json:"extensionName,omitempty"`
	DisplayName   string `json:"displayName"`
	SSHTunnel     bool   `json:"sshTunnel"`
}

// ListInstalledExtensions returns all loaded extensions.
func (a *App) ListInstalledExtensions() []extension_svc.ExtensionInfo {
	if a.extSvc == nil {
		return nil
	}
	return a.extSvc.ListInstalled(a.lang)
}

// GetExtensionManifest returns a single extension's manifest.
func (a *App) GetExtensionManifest(name string) (*extension.Manifest, error) {
	if a.extSvc == nil {
		return nil, fmt.Errorf("extension system not initialized")
	}
	ext := a.extSvc.Manager().GetExtension(name)
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
	if a.extSvc != nil {
		bridge := a.extSvc.Bridge()
		lang := a.lang
		for _, at := range bridge.GetAssetTypes() {
			displayName := at.I18n.Name
			if ext := a.extSvc.Manager().GetExtension(at.ExtensionName); ext != nil {
				displayName = ext.Translate(lang, at.I18n.Name)
			}
			types = append(types, AssetTypeInfo{
				Type:          at.Type,
				ExtensionName: at.ExtensionName,
				DisplayName:   displayName,
				SSHTunnel:     true,
			})
		}
	}
	return types
}

// CallExtensionAction calls an extension action and streams events via Wails Events.
func (a *App) CallExtensionAction(extName, action string, argsJSON string) (string, error) {
	if a.extSvc == nil {
		return "", fmt.Errorf("extension system not initialized")
	}
	ext := a.extSvc.Manager().GetExtension(extName)
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
	if a.extSvc == nil {
		return "", fmt.Errorf("extension system not initialized")
	}
	ext := a.extSvc.Manager().GetExtension(extName)
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
func (a *App) GetDecryptedExtensionConfig(assetID int64, extName string) (string, error) {
	if a.extSvc == nil {
		return "", fmt.Errorf("extension system not initialized")
	}
	return getDecryptedExtConfig(assetID, a.extSvc.Bridge())
}

// InstallExtension opens a file dialog and installs an extension from a zip file.
func (a *App) InstallExtension() (*extension_svc.ExtensionInfo, error) {
	if a.extSvc == nil {
		return nil, fmt.Errorf("extension system not initialized")
	}

	selected, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Extension Package",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Extension Package (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("file dialog: %w", err)
	}
	if selected == "" {
		return nil, nil // user canceled
	}

	return a.installExtensionFromPath(selected)
}

// InstallExtensionFromDirectory opens a directory dialog and installs a local extension.
func (a *App) InstallExtensionFromDirectory() (*extension_svc.ExtensionInfo, error) {
	if a.extSvc == nil {
		return nil, fmt.Errorf("extension system not initialized")
	}

	selected, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Extension Directory",
	})
	if err != nil {
		return nil, fmt.Errorf("directory dialog: %w", err)
	}
	if selected == "" {
		return nil, nil // user canceled
	}

	return a.installExtensionFromPath(selected)
}

func (a *App) installExtensionFromPath(sourcePath string) (*extension_svc.ExtensionInfo, error) {
	manifest, err := a.extSvc.Install(a.langCtx(), sourcePath)
	if err != nil {
		return nil, err
	}

	ext := a.extSvc.Manager().GetExtension(manifest.Name)
	lm := manifest
	if ext != nil {
		lm = manifest.Localized(func(key string) string { return ext.Translate(a.lang, key) })
	}

	return &extension_svc.ExtensionInfo{
		Name:        lm.Name,
		Version:     lm.Version,
		Icon:        lm.Icon,
		DisplayName: lm.I18n.DisplayName,
		Description: lm.I18n.Description,
		Enabled:     true,
		Manifest:    lm,
	}, nil
}

// UninstallExtension removes an extension and optionally cleans up its data.
// Returns an error if active assets still reference the extension's asset types.
func (a *App) UninstallExtension(name string, cleanData bool) error {
	if a.extSvc == nil {
		return fmt.Errorf("extension system not initialized")
	}
	return a.extSvc.Uninstall(a.langCtx(), name, cleanData, false)
}

// ForceUninstallExtension removes an extension and optionally cleans up its data,
// bypassing the orphan-asset check. Use when you intend to leave or manually clean
// assets that reference this extension's asset types.
func (a *App) ForceUninstallExtension(name string, cleanData bool) error {
	if a.extSvc == nil {
		return fmt.Errorf("extension system not initialized")
	}
	return a.extSvc.Uninstall(a.langCtx(), name, cleanData, true)
}

// EnableExtension loads a disabled extension and registers it.
func (a *App) EnableExtension(name string) error {
	if a.extSvc == nil {
		return fmt.Errorf("extension system not initialized")
	}
	return a.extSvc.Enable(a.langCtx(), name)
}

// DisableExtension unloads a running extension without removing files.
func (a *App) DisableExtension(name string) error {
	if a.extSvc == nil {
		return fmt.Errorf("extension system not initialized")
	}
	return a.extSvc.Disable(a.langCtx(), name)
}

// GetExtensionDetail returns the full manifest and state for a single extension.
func (a *App) GetExtensionDetail(name string) (*extension_svc.ExtensionInfo, error) {
	if a.extSvc == nil {
		return nil, fmt.Errorf("extension system not initialized")
	}
	return a.extSvc.GetDetail(name, a.lang)
}

// ReloadExtensions re-scans extensions directory and updates the bridge.
func (a *App) ReloadExtensions() error {
	if a.extSvc == nil {
		return fmt.Errorf("extension system not initialized")
	}
	return a.extSvc.Reload(a.langCtx())
}
