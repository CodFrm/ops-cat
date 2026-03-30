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

// appCredentialGetter implements extension.CredentialGetter
type appCredentialGetter struct{}

func (g *appCredentialGetter) GetCredential(assetID int64) (string, error) {
	ctx := context.Background()
	asset, err := asset_repo.Asset().Find(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("asset %d not found: %w", assetID, err)
	}
	if asset.Config == "" {
		return "", nil
	}
	var cfg struct {
		Password     string `json:"password"`
		CredentialID int64  `json:"credential_id"`
	}
	if err := json.Unmarshal([]byte(asset.Config), &cfg); err != nil {
		return "", err
	}
	if cfg.Password != "" {
		return credential_svc.Default().Decrypt(cfg.Password)
	}
	return "", nil
}

// appAssetConfigGetter implements extension.AssetConfigGetter
type appAssetConfigGetter struct{}

func (g *appAssetConfigGetter) GetAssetConfig(assetID int64) (json.RawMessage, error) {
	ctx := context.Background()
	asset, err := asset_repo.Asset().Find(ctx, assetID)
	if err != nil {
		return nil, fmt.Errorf("asset %d not found: %w", assetID, err)
	}
	if asset.Config == "" {
		return json.RawMessage("{}"), nil
	}
	return json.RawMessage(asset.Config), nil
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
	val, err := extension_data_repo.ExtData().Get(context.Background(), s.extName, key)
	if err != nil {
		return nil, nil //nolint:nilerr // KV miss returns nil, not error
	}
	return val, nil
}

func (s *appKVStore) Set(key string, value []byte) error {
	return extension_data_repo.ExtData().Set(context.Background(), s.extName, key, value)
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
