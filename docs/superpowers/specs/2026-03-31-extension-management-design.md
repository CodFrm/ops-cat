# Extension Management Design

Desktop extension management: install (local zip/directory), uninstall, enable/disable, detail view.

## Scope

- Install extensions from local `.zip` file or directory
- Uninstall extensions with optional data cleanup
- Enable/disable extensions (persisted in database)
- Extension detail panel (manifest info, tools, policies, pages)
- No online marketplace, no extension global config, no CLI extension management

## Data Layer

### New Table: `extension_state`

| Column     | Type   | Description                    |
|------------|--------|--------------------------------|
| id         | int64  | Primary key                    |
| name       | string | Extension name (unique index)  |
| enabled    | bool   | Enabled state (default: true)  |
| createtime | int64  | Creation timestamp             |
| updatetime | int64  | Last update timestamp          |

- New extension installed → auto-insert with `enabled=true`
- Uninstall with `cleanData=true` → delete `extension_state` + all `extension_data` rows for that extension
- Uninstall with `cleanData=false` → delete `extension_state` only, keep `extension_data`

### Repository

`internal/repository/extension_state_repo/` following project's interface + impl + Register/getter pattern.

Interface:
```go
type ExtensionStateRepo interface {
    Find(ctx context.Context, name string) (*entity.ExtensionState, error)
    FindAll(ctx context.Context) ([]*entity.ExtensionState, error)
    Create(ctx context.Context, state *entity.ExtensionState) error
    Update(ctx context.Context, state *entity.ExtensionState) error
    Delete(ctx context.Context, name string) error
}
```

Entity in `internal/model/entity/extension_state_entity/`.

Migration added to `/migrations/`.

## Manager Layer (`pkg/extension/Manager`)

### New Methods

**`Install(ctx context.Context, sourcePath string) (*Manifest, error)`**

1. Detect if `sourcePath` is a `.zip` file or directory
2. If zip: extract to temp directory
3. Read and validate `manifest.json` from source
4. If extension with same name is already loaded: call `Unload()` first
5. Copy contents to `{extensions_dir}/{manifest.Name}/`
6. Call `LoadExtension()` to load and register
7. Return manifest

**`Uninstall(ctx context.Context, name string) error`**

1. Call `Unload()` to stop the extension
2. Remove `{extensions_dir}/{name}/` directory
3. Database cleanup handled by caller (app layer)

### Exported Method

Rename `loadExtension` → `LoadExtension` (public). Used by app layer when enabling a previously disabled extension.

### Scan Behavior Change

`Scan()` continues to load all extensions from disk as before. After Scan, the app layer reads `extension_state` table and calls `Unload()` for any extension marked `enabled=false`. This keeps Manager unaware of the database — it remains a pure filesystem + WASM loader.

### Disabled Extension Discovery

New method `ScanManifests(ctx context.Context) ([]*ManifestInfo, error)` — scans directories and reads manifests without loading WASM. Returns name, version, display info. Used by `ListInstalledExtensions` to include disabled extensions in the list.

```go
type ManifestInfo struct {
    Name     string
    Dir      string
    Manifest *Manifest
}
```

## App Binding Layer (`internal/app/app_extension.go`)

### Modified

**`ListInstalledExtensions() []ExtensionInfo`**

- ExtensionInfo adds `Enabled bool` field
- Returns all extensions found on disk (both loaded and disabled)
- For loaded extensions: read from Manager
- For disabled extensions: read manifest from disk via `ScanManifests()`
- Merge with `extension_state` table for enabled status

**Startup flow change:**

After `extManager.Scan()`, read all `extension_state` records. For any extension marked `enabled=false`, call `extManager.Unload()`. This ensures disabled extensions don't run.

### New Methods

**`InstallExtension() (*ExtensionInfo, error)`**

1. Open system file dialog (Wails runtime) — filter for `.zip` files and directories
2. Call `extManager.Install(ctx, selectedPath)`
3. Insert `extension_state` record with `enabled=true`
4. Register in Bridge
5. Emit `ext:reload` event
6. Return extension info

**`UninstallExtension(name string, cleanData bool) error`**

1. Call `extManager.Uninstall(ctx, name)`
2. Unregister from Bridge
3. Delete `extension_state` record
4. If `cleanData`: delete all `extension_data` rows where `extension_name = name`
5. Emit `ext:reload` event

**`EnableExtension(name string) error`**

1. Get extension dir from extensions directory
2. Call `extManager.LoadExtension(ctx, dir)` to load WASM
3. Register in Bridge
4. Update `extension_state.enabled = true`
5. Update AI tool executor
6. Emit `ext:reload` event

**`DisableExtension(name string) error`**

1. Call `extManager.Unload(ctx, name)`
2. Unregister from Bridge
3. Update `extension_state.enabled = false`
4. Update AI tool executor
5. Emit `ext:reload` event

**`GetExtensionDetail(name string) (*ExtensionDetail, error)`**

Returns full extension info for the detail panel:

```go
type ExtensionDetail struct {
    Name        string              `json:"name"`
    Version     string              `json:"version"`
    Icon        string              `json:"icon"`
    DisplayName string              `json:"displayName"`
    Description string              `json:"description"`
    Enabled     bool                `json:"enabled"`
    Manifest    *extension.Manifest `json:"manifest"`
}
```

## Frontend UI

### ExtensionSection.tsx Redesign

**List view:**

- Top bar: title + description (left), "Install Extension" button + "Reload" button (right)
- Each extension card:
  - Left: icon + name + description + version
  - Right: Enable/Disable switch + dropdown menu (Detail, Uninstall)
  - Disabled extensions: reduced opacity, "Disabled" badge next to version

**Install flow:**

- Click "Install Extension" → calls `InstallExtension()` (backend handles file dialog)
- Success: toast + refresh list
- Error: toast with error message

**Uninstall flow:**

- Dropdown menu → "Uninstall" → AlertDialog confirmation
  - Dialog contains checkbox: "Clean up extension data"
  - Confirm → calls `UninstallExtension(name, cleanData)`
  - Success: toast + refresh list

**Detail panel:**

- Click card or "Detail" in dropdown → open Sheet (side panel) or expand inline
- Sections:
  - Basic info: name, version, description
  - Tools: table with name + description columns
  - Policy groups: table with group name + allowed/denied actions
  - Frontend pages: table with page name + slot
- Read-only, no edit actions

### i18n

New keys under `extension.*` namespace in both `zh-CN` and `en` locales:

- `extension.install` / Install Extension
- `extension.uninstall` / Uninstall
- `extension.uninstallConfirm` / Confirm uninstall message
- `extension.cleanData` / Clean up extension data
- `extension.cleanDataDesc` / description text
- `extension.enable` / Enable
- `extension.disable` / Disable
- `extension.disabled` / Disabled (badge)
- `extension.detail` / Detail
- `extension.tools` / Tools
- `extension.policies` / Policies
- `extension.pages` / Pages
- `extension.installSuccess` / Install success
- `extension.uninstallSuccess` / Uninstall success
- `extension.enableSuccess` / Enable success
- `extension.disableSuccess` / Disable success

## File Changes Summary

### New Files
- `internal/model/entity/extension_state_entity/extension_state.go` — entity
- `internal/repository/extension_state_repo/extension_state.go` — repo interface + impl
- `migrations/NNNN_create_extension_state.go` — migration

### Modified Files
- `pkg/extension/manager.go` — Install, Uninstall, LoadExtension (export), ScanManifests
- `internal/app/app_extension.go` — 5 new Wails methods, modified ListInstalledExtensions, startup flow
- `internal/app/app.go` — startup: unload disabled extensions after Scan
- `frontend/src/components/settings/ExtensionSection.tsx` — full redesign
- `frontend/src/i18n/locales/zh-CN.json` — new keys
- `frontend/src/i18n/locales/en.json` — new keys
- `frontend/wailsjs/` — auto-generated bindings (after adding Go methods)
