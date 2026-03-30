package app

import (
	"encoding/json"
	"fmt"

	"github.com/opskat/opskat/internal/ai"
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
	exts := a.extManager.ListExtensions()
	result := make([]ExtensionInfo, 0, len(exts))
	for _, ext := range exts {
		result = append(result, ExtensionInfo{
			Name:        ext.Name,
			Version:     ext.Manifest.Version,
			Icon:        ext.Manifest.Icon,
			DisplayName: ext.Manifest.I18n.DisplayName,
			Description: ext.Manifest.I18n.Description,
			Manifest:    ext.Manifest,
		})
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
