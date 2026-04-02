package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/extension_data_repo"
	"github.com/opskat/opskat/internal/service/credential_svc"
	"github.com/opskat/opskat/pkg/extension"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// appAssetConfigGetter implements extension.AssetConfigGetter.
// It decrypts format:"password" fields in the config before returning to the extension.
type appAssetConfigGetter struct {
	app *App
}

func (g *appAssetConfigGetter) GetAssetConfig(assetID int64) (json.RawMessage, error) {
	ctx := context.Background()
	asset, err := asset_repo.Asset().Find(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("asset %d not found: %w", assetID, err)
	}
	if asset.Config == "" {
		return json.RawMessage("{}"), nil
	}
	raw := json.RawMessage(asset.Config)
	return decryptConfigPasswordFields(raw, asset.Type, g.app.extSvc.Bridge())
}

// appFileDialogOpener implements extension.FileDialogOpener
type appFileDialogOpener struct {
	ctx context.Context // Wails app context
}

func (o *appFileDialogOpener) FileDialog(dialogType string, opts extension.DialogOptions) (string, error) {
	switch dialogType {
	case "open":
		return wailsRuntime.OpenFileDialog(o.ctx, wailsRuntime.OpenDialogOptions{
			Title:   opts.Title,
			Filters: toWailsFilters(opts.Filters),
		})
	case "save":
		return wailsRuntime.SaveFileDialog(o.ctx, wailsRuntime.SaveDialogOptions{
			Title:           opts.Title,
			DefaultFilename: opts.DefaultName,
			Filters:         toWailsFilters(opts.Filters),
		})
	default:
		return "", fmt.Errorf("unknown dialog type: %q", dialogType)
	}
}

func toWailsFilters(filters []string) []wailsRuntime.FileFilter {
	if len(filters) == 0 {
		return nil
	}
	result := make([]wailsRuntime.FileFilter, 0, len(filters))
	for _, f := range filters {
		result = append(result, wailsRuntime.FileFilter{
			DisplayName: f,
			Pattern:     f,
		})
	}
	return result
}

// appKVStore implements extension.KVStore, scoped to one extension
type appKVStore struct {
	extName string
}

func (s *appKVStore) Get(key string) ([]byte, error) {
	val, err := extension_data_repo.ExtensionData().Get(context.Background(), s.extName, key)
	if err != nil {
		return nil, nil //nolint:nilerr // KV miss returns nil, not error
	}
	return val, nil
}

func (s *appKVStore) Set(key string, value []byte) error {
	return extension_data_repo.ExtensionData().Set(context.Background(), s.extName, key, value)
}

// appActionEventHandler implements extension.ActionEventHandler
type appActionEventHandler struct {
	ctx     context.Context // Wails app context
	extName string
}

func (h *appActionEventHandler) OnActionEvent(eventType string, data json.RawMessage) error {
	wailsRuntime.EventsEmit(h.ctx, "ext:action:event", map[string]any{
		"extension": h.extName,
		"eventType": eventType,
		"data":      json.RawMessage(data),
	})
	return nil
}

// getDecryptedExtConfig returns the asset config with password fields decrypted.
func getDecryptedExtConfig(assetID int64, bridge *extension.Bridge) (string, error) {
	ctx := context.Background()
	asset, err := asset_repo.Asset().Find(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("asset %d not found: %w", assetID, err)
	}
	if asset.Config == "" {
		return "{}", nil
	}
	raw := json.RawMessage(asset.Config)
	decrypted, err := decryptConfigPasswordFields(raw, asset.Type, bridge)
	if err != nil {
		return "", err
	}
	return string(decrypted), nil
}

// decryptConfigPasswordFields decrypts fields marked as format:"password" in the configSchema.
func decryptConfigPasswordFields(raw json.RawMessage, assetType string, bridge *extension.Bridge) (json.RawMessage, error) {
	if bridge == nil {
		return raw, nil
	}
	// Find the extension that provides this asset type
	var schema map[string]any
	for _, at := range bridge.GetAssetTypes() {
		if at.Type == assetType {
			schema = at.ConfigSchema
			break
		}
	}
	if len(schema) == 0 {
		return raw, nil
	}
	passwordFields := extension.PasswordFieldsFromSchema(schema)
	if len(passwordFields) == 0 {
		return raw, nil
	}

	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return raw, err
	}

	for _, field := range passwordFields {
		val, ok := cfg[field]
		if !ok {
			continue
		}
		var encrypted string
		if err := json.Unmarshal(val, &encrypted); err != nil || encrypted == "" {
			continue
		}
		decrypted, err := credential_svc.Default().Decrypt(encrypted)
		if err != nil {
			// May already be plaintext (e.g. test_connection); keep as-is
			continue
		}
		b, _ := json.Marshal(decrypted)
		cfg[field] = b
	}
	return json.Marshal(cfg)
}

// appTunnelDialer implements extension.TunnelDialer using the SSH pool
type appTunnelDialer struct {
	app *App
}

func (d *appTunnelDialer) Dial(tunnelAssetID int64, addr string) (net.Conn, error) {
	if d.app.sshPool == nil {
		return nil, fmt.Errorf("SSH pool not initialized")
	}
	client, err := d.app.sshPool.Get(context.Background(), tunnelAssetID)
	if err != nil {
		return nil, fmt.Errorf("get SSH tunnel: %w", err)
	}
	conn, err := client.Dial("tcp", addr)
	if err != nil {
		d.app.sshPool.Release(tunnelAssetID)
		return nil, fmt.Errorf("dial through tunnel: %w", err)
	}
	return conn, nil
}
