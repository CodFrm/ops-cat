//go:build !no_wasm

package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/extension_data_repo"
	"github.com/opskat/opskat/internal/service/credential_svc"
	"github.com/opskat/opskat/pkg/extension"

	"go.uber.org/zap"
)

// localExtExec loads the extension locally and executes the tool via WASM.
func localExtExec(extName string, toolName string, toolArgs json.RawMessage) int {
	ctx := context.Background()
	extDir := filepath.Join(bootstrap.AppDataDir(), "extensions")

	mgr := extension.NewManager(extDir, func(name string) extension.HostProvider {
		return extension.NewDefaultHostProvider(extension.DefaultHostConfig{
			Logger:       zap.L(),
			Credentials:  &cliCredentialGetter{},
			AssetConfigs: &cliAssetConfigGetter{},
			FileDialogs:  &cliFileDialogOpener{},
			KV:           &cliKVStore{extName: name},
			ActionEvents: &cliActionEventHandler{},
		})
	}, zap.L())
	defer mgr.Close(ctx)

	if _, err := mgr.Scan(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: scan extensions: %v\n", err)
		return 1
	}

	ext := mgr.GetExtension(extName)
	if ext == nil {
		fmt.Fprintf(os.Stderr, "Error: extension %q not found\n", extName)
		return 1
	}
	if ext.Plugin == nil {
		fmt.Fprintf(os.Stderr, "Error: extension %q has no backend plugin\n", extName)
		return 1
	}

	result, err := ext.Plugin.CallTool(ctx, toolName, toolArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	printToolResult(string(result))
	return 0
}

// --- CLI-specific HostProvider adapters ---

// cliCredentialGetter reads credentials from the database (available after bootstrap.Init).
type cliCredentialGetter struct{}

func (g *cliCredentialGetter) GetCredential(assetID int64) (string, error) {
	ctx := context.Background()
	asset, err := asset_repo.Asset().Find(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("asset %d not found: %w", assetID, err)
	}
	if asset.Config == "" {
		return "", nil
	}
	var cfg struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(asset.Config), &cfg); err != nil {
		return "", err
	}
	if cfg.Password != "" {
		return credential_svc.Default().Decrypt(cfg.Password)
	}
	return "", nil
}

// cliAssetConfigGetter reads asset config from the database.
type cliAssetConfigGetter struct{}

func (g *cliAssetConfigGetter) GetAssetConfig(assetID int64) (json.RawMessage, error) {
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

// cliFileDialogOpener returns an error — file dialogs not available in CLI mode.
type cliFileDialogOpener struct{}

func (o *cliFileDialogOpener) FileDialog(dialogType string, opts extension.DialogOptions) (string, error) {
	return "", fmt.Errorf("file dialogs are not supported in CLI mode")
}

// cliKVStore uses the extension_data_repo backed by SQLite.
type cliKVStore struct {
	extName string
}

func (s *cliKVStore) Get(key string) ([]byte, error) {
	val, err := extension_data_repo.ExtData().Get(context.Background(), s.extName, key)
	if err != nil {
		return nil, nil //nolint:nilerr // KV miss returns nil, not error
	}
	return val, nil
}

func (s *cliKVStore) Set(key string, value []byte) error {
	return extension_data_repo.ExtData().Set(context.Background(), s.extName, key, value)
}

// cliActionEventHandler is a no-op — action events not supported in CLI.
type cliActionEventHandler struct{}

func (h *cliActionEventHandler) OnActionEvent(eventType string, data json.RawMessage) error {
	fmt.Fprintf(os.Stderr, "Warning: action event %q not supported in CLI mode\n", eventType)
	return nil
}
