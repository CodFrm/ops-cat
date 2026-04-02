# Extension Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add install/uninstall/enable/disable/detail capabilities to the desktop extension management UI.

**Architecture:** Extend `pkg/extension/Manager` with Install/Uninstall/LoadExtension/ScanManifests methods. Persist enable/disable state in a new `extension_state` SQLite table via the standard repository pattern. Expose 5 new Wails IPC methods in `app_extension.go`. Redesign `ExtensionSection.tsx` with install button, enable/disable switch, uninstall dialog, and detail dialog.

**Tech Stack:** Go 1.25, GORM/SQLite, gormigrate, Wails v2, React 19, TypeScript, shadcn/ui (`@opskat/ui`), Zustand, i18next, Lucide icons.

---

### Task 1: Entity and Migration

**Files:**
- Create: `internal/model/entity/extension_state_entity/extension_state.go`
- Create: `migrations/202603310001_create_extension_state.go`
- Modify: `migrations/migrations.go`

- [ ] **Step 1: Create entity**

```go
// internal/model/entity/extension_state_entity/extension_state.go
package extension_state_entity

type ExtensionState struct {
	ID         int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name       string `gorm:"column:name;uniqueIndex:idx_ext_state_name"`
	Enabled    bool   `gorm:"column:enabled;default:true"`
	Createtime int64  `gorm:"column:createtime"`
	Updatetime int64  `gorm:"column:updatetime"`
}

func (ExtensionState) TableName() string {
	return "extension_state"
}
```

- [ ] **Step 2: Create migration**

```go
// migrations/202603310001_create_extension_state.go
package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration202603310001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603310001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS extension_state (
				id         INTEGER PRIMARY KEY AUTOINCREMENT,
				name       VARCHAR(255) NOT NULL,
				enabled    INTEGER NOT NULL DEFAULT 1,
				createtime INTEGER NOT NULL,
				updatetime INTEGER NOT NULL
			)`).Error; err != nil {
				return err
			}
			return tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ext_state_name ON extension_state (name)`).Error
		},
	}
}
```

- [ ] **Step 3: Register migration**

In `migrations/migrations.go`, add `migration202603310001()` to the `RunMigrations` slice:

```go
func RunMigrations(db *gorm.DB) error {
	m := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		migration202603220001(),
		migration202603260001(),
		migration202603270001(),
		migration202603290001(),
		migration202603300001(),
		migration202603300002(),
		migration202603310001(), // <-- add this line
	})
	return m.Migrate()
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd /Users/codfrm/Code/opskat/opskat && go build ./...`
Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add internal/model/entity/extension_state_entity/extension_state.go migrations/202603310001_create_extension_state.go migrations/migrations.go
git commit -m "✨ Add extension_state entity and migration"
```

---

### Task 2: Repository

**Files:**
- Create: `internal/repository/extension_state_repo/extension_state.go`
- Modify: `internal/bootstrap/bootstrap.go` (register repo at startup)

- [ ] **Step 1: Create repository**

```go
// internal/repository/extension_state_repo/extension_state.go
package extension_state_repo

import (
	"context"
	"time"

	"github.com/opskat/opskat/internal/model/entity/extension_state_entity"

	"github.com/cago-frame/cago/database/db"
)

type ExtensionStateRepo interface {
	Find(ctx context.Context, name string) (*extension_state_entity.ExtensionState, error)
	FindAll(ctx context.Context) ([]*extension_state_entity.ExtensionState, error)
	Create(ctx context.Context, state *extension_state_entity.ExtensionState) error
	Update(ctx context.Context, state *extension_state_entity.ExtensionState) error
	Delete(ctx context.Context, name string) error
}

var defaultRepo ExtensionStateRepo

func ExtensionState() ExtensionStateRepo {
	return defaultRepo
}

func RegisterExtensionState(r ExtensionStateRepo) {
	defaultRepo = r
}

type extensionStateRepo struct{}

func NewExtensionState() ExtensionStateRepo {
	return &extensionStateRepo{}
}

func (r *extensionStateRepo) Find(ctx context.Context, name string) (*extension_state_entity.ExtensionState, error) {
	var row extension_state_entity.ExtensionState
	err := db.Ctx(ctx).Where("name = ?", name).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *extensionStateRepo) FindAll(ctx context.Context) ([]*extension_state_entity.ExtensionState, error) {
	var rows []*extension_state_entity.ExtensionState
	err := db.Ctx(ctx).Find(&rows).Error
	return rows, err
}

func (r *extensionStateRepo) Create(ctx context.Context, state *extension_state_entity.ExtensionState) error {
	now := time.Now().Unix()
	state.Createtime = now
	state.Updatetime = now
	return db.Ctx(ctx).Create(state).Error
}

func (r *extensionStateRepo) Update(ctx context.Context, state *extension_state_entity.ExtensionState) error {
	state.Updatetime = time.Now().Unix()
	return db.Ctx(ctx).Save(state).Error
}

func (r *extensionStateRepo) Delete(ctx context.Context, name string) error {
	return db.Ctx(ctx).Where("name = ?", name).Delete(&extension_state_entity.ExtensionState{}).Error
}
```

- [ ] **Step 2: Register repository at startup**

Find the bootstrap or app initialization code where other repos are registered (e.g., `RegisterExtensionData`). Add the same pattern:

```go
import "github.com/opskat/opskat/internal/repository/extension_state_repo"

// In the registration block:
extension_state_repo.RegisterExtensionState(extension_state_repo.NewExtensionState())
```

Search for `RegisterExtensionData` in the codebase to find the exact file and location.

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/codfrm/Code/opskat/opskat && go build ./...`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add internal/repository/extension_state_repo/extension_state.go
git commit -m "✨ Add extension_state repository"
```

Note: include the bootstrap file in the commit if it was modified.

---

### Task 3: Manager — Install, Uninstall, LoadExtension, ScanManifests

**Files:**
- Modify: `pkg/extension/manager.go`

- [ ] **Step 1: Export loadExtension as LoadExtension**

In `pkg/extension/manager.go`, rename `loadExtension` to `LoadExtension`:

Change the method signature:
```go
func (m *Manager) LoadExtension(ctx context.Context, dir string) (*Manifest, error) {
```

And update the call site in `Scan()`:
```go
manifest, err := m.LoadExtension(ctx, extDir)
```

- [ ] **Step 2: Add ScanManifests method**

Append to `pkg/extension/manager.go`:

```go
// ManifestInfo holds manifest data for an extension that may not be loaded.
type ManifestInfo struct {
	Name     string
	Dir      string
	Manifest *Manifest
}

// ScanManifests reads manifests from disk without loading WASM plugins.
func (m *Manager) ScanManifests() ([]*ManifestInfo, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read extensions dir: %w", err)
	}

	var result []*ManifestInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		extDir := filepath.Join(m.dir, entry.Name())
		manifestPath := filepath.Join(extDir, "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		manifest, err := ParseManifest(data)
		if err != nil {
			continue
		}
		result = append(result, &ManifestInfo{
			Name:     manifest.Name,
			Dir:      extDir,
			Manifest: manifest,
		})
	}
	return result, nil
}
```

- [ ] **Step 3: Add Install method**

Append to `pkg/extension/manager.go`:

```go
// Install installs an extension from a zip file or directory.
func (m *Manager) Install(ctx context.Context, sourcePath string) (*Manifest, error) {
	sourceDir := sourcePath
	var tmpDir string

	// If zip, extract to temp directory
	if strings.HasSuffix(strings.ToLower(sourcePath), ".zip") {
		var err error
		tmpDir, err = os.MkdirTemp("", "opskat-ext-*")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)
		if err := extractZip(sourcePath, tmpDir); err != nil {
			return nil, fmt.Errorf("extract zip: %w", err)
		}
		sourceDir = tmpDir
	}

	// Read and validate manifest
	manifestPath := filepath.Join(sourceDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		return nil, err
	}

	// Unload existing if already loaded
	m.mu.RLock()
	_, exists := m.extensions[manifest.Name]
	m.mu.RUnlock()
	if exists {
		if err := m.Unload(ctx, manifest.Name); err != nil {
			m.logger.Warn("unload existing extension", zap.String("name", manifest.Name), zap.Error(err))
		}
	}

	// Copy to extensions directory
	destDir := filepath.Join(m.dir, manifest.Name)
	if err := os.RemoveAll(destDir); err != nil {
		return nil, fmt.Errorf("remove existing dir: %w", err)
	}
	if err := copyDir(sourceDir, destDir); err != nil {
		return nil, fmt.Errorf("copy extension: %w", err)
	}

	// Load the extension
	if _, err := m.LoadExtension(ctx, destDir); err != nil {
		os.RemoveAll(destDir)
		return nil, fmt.Errorf("load extension: %w", err)
	}

	return manifest, nil
}
```

Add the `strings` import to the import block at the top of the file.

- [ ] **Step 4: Add Uninstall method**

Append to `pkg/extension/manager.go`:

```go
// Uninstall stops and removes an extension from disk.
func (m *Manager) Uninstall(ctx context.Context, name string) error {
	// Unload if loaded (ignore error if not loaded)
	_ = m.Unload(ctx, name)

	// Remove extension directory
	extDir := filepath.Join(m.dir, name)
	if err := os.RemoveAll(extDir); err != nil {
		return fmt.Errorf("remove extension dir: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Add ExtDir getter**

Append to `pkg/extension/manager.go`:

```go
// ExtDir returns the path to a named extension's directory.
func (m *Manager) ExtDir(name string) string {
	return filepath.Join(m.dir, name)
}
```

- [ ] **Step 6: Add helper functions (extractZip, copyDir)**

Create `pkg/extension/fileutil.go`:

```go
package extension

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		if strings.Contains(name, "..") {
			return fmt.Errorf("zip contains path traversal: %s", f.Name)
		}
		target := filepath.Join(destDir, name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		out, err := os.Create(target)
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}

		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}
```

- [ ] **Step 7: Verify compilation**

Run: `cd /Users/codfrm/Code/opskat/opskat && go build ./...`
Expected: No errors.

- [ ] **Step 8: Commit**

```bash
git add pkg/extension/manager.go pkg/extension/fileutil.go
git commit -m "✨ Add Install, Uninstall, LoadExtension, ScanManifests to Manager"
```

---

### Task 4: App Binding — Wails Methods

**Files:**
- Modify: `internal/app/app_extension.go`
- Modify: `internal/app/app.go` (startup flow)

- [ ] **Step 1: Add Enabled field to ExtensionInfo**

In `internal/app/app_extension.go`, add the `Enabled` field:

```go
type ExtensionInfo struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Icon        string              `json:"icon"`
	DisplayName string              `json:"displayName"`
	Description string              `json:"description"`
	Enabled     bool                `json:"enabled"`
	Manifest    *extension.Manifest `json:"manifest"`
}
```

- [ ] **Step 2: Rewrite ListInstalledExtensions**

Replace the existing `ListInstalledExtensions` method to include disabled extensions:

```go
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
```

- [ ] **Step 3: Add InstallExtension method**

Append to `internal/app/app_extension.go`:

```go
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
```

- [ ] **Step 4: Add UninstallExtension method**

Append to `internal/app/app_extension.go`:

```go
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
```

- [ ] **Step 5: Add EnableExtension method**

Append to `internal/app/app_extension.go`:

```go
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
```

- [ ] **Step 6: Add DisableExtension method**

Append to `internal/app/app_extension.go`:

```go
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
```

- [ ] **Step 7: Add GetExtensionDetail method**

Append to `internal/app/app_extension.go`:

```go
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
```

- [ ] **Step 8: Add ensureExtensionState helper**

Append to `internal/app/app_extension.go`:

```go
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
```

- [ ] **Step 9: Add imports**

Add the necessary imports to `internal/app/app_extension.go`:

```go
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
```

- [ ] **Step 10: Modify startup to unload disabled extensions**

In `internal/app/app.go`, after the extension scan and bridge registration block, add disabled-extension unloading:

```go
	// Register loaded extensions into bridge
	for _, ext := range a.extManager.ListExtensions() {
		a.extBridge.Register(ext)
	}

	// Unload disabled extensions
	ctx2 := context.Background()
	states, _ := extension_state_repo.ExtensionState().FindAll(ctx2)
	for _, state := range states {
		if !state.Enabled {
			a.extBridge.Unregister(state.Name)
			_ = a.extManager.Unload(ctx, state.Name)
		}
	}
```

Add `extension_state_repo` to the imports of `app.go`.

- [ ] **Step 11: Verify compilation**

Run: `cd /Users/codfrm/Code/opskat/opskat && go build ./...`
Expected: No errors.

- [ ] **Step 12: Commit**

```bash
git add internal/app/app_extension.go internal/app/app.go
git commit -m "✨ Add extension Install/Uninstall/Enable/Disable/Detail Wails bindings"
```

---

### Task 5: Frontend — i18n Keys

**Files:**
- Modify: `frontend/src/i18n/locales/zh-CN/common.json`
- Modify: `frontend/src/i18n/locales/en/common.json`

- [ ] **Step 1: Add Chinese i18n keys**

In `frontend/src/i18n/locales/zh-CN/common.json`, replace the `"extension"` block:

```json
"extension": {
  "title": "扩展",
  "installed": "已安装扩展",
  "noExtensions": "未安装扩展",
  "noExtensionsDesc": "扩展安装到应用数据目录的 extensions/ 文件夹下",
  "version": "版本",
  "reload": "重新加载",
  "reloadSuccess": "扩展已重新加载",
  "reloadError": "重新加载失败",
  "install": "安装扩展",
  "installSuccess": "扩展安装成功",
  "installError": "安装失败",
  "uninstall": "卸载",
  "uninstallConfirm": "确定要卸载扩展 \"{{name}}\" 吗？此操作不可撤销。",
  "uninstallSuccess": "扩展已卸载",
  "cleanData": "同时清理扩展数据",
  "cleanDataDesc": "删除该扩展存储的所有数据，取消勾选则保留数据以便重新安装时恢复",
  "enable": "启用",
  "disable": "禁用",
  "disabled": "已禁用",
  "enableSuccess": "扩展已启用",
  "disableSuccess": "扩展已禁用",
  "detail": "详情",
  "tools": "工具",
  "toolName": "名称",
  "toolDescription": "描述",
  "policies": "策略组",
  "policyName": "策略名称",
  "policyActions": "允许/拒绝操作",
  "pages": "页面",
  "pageName": "页面名称",
  "pageSlot": "插槽",
  "noTools": "无工具",
  "noPolicies": "无策略组",
  "noPages": "无页面"
}
```

- [ ] **Step 2: Add English i18n keys**

In `frontend/src/i18n/locales/en/common.json`, replace the `"extension"` block:

```json
"extension": {
  "title": "Extensions",
  "installed": "Installed Extensions",
  "noExtensions": "No extensions installed",
  "noExtensionsDesc": "Extensions are installed to the extensions/ folder in the app data directory",
  "version": "Version",
  "reload": "Reload",
  "reloadSuccess": "Extensions reloaded",
  "reloadError": "Reload failed",
  "install": "Install Extension",
  "installSuccess": "Extension installed successfully",
  "installError": "Installation failed",
  "uninstall": "Uninstall",
  "uninstallConfirm": "Are you sure you want to uninstall \"{{name}}\"? This action cannot be undone.",
  "uninstallSuccess": "Extension uninstalled",
  "cleanData": "Clean up extension data",
  "cleanDataDesc": "Delete all data stored by this extension. Uncheck to keep data for reinstallation.",
  "enable": "Enable",
  "disable": "Disable",
  "disabled": "Disabled",
  "enableSuccess": "Extension enabled",
  "disableSuccess": "Extension disabled",
  "detail": "Details",
  "tools": "Tools",
  "toolName": "Name",
  "toolDescription": "Description",
  "policies": "Policy Groups",
  "policyName": "Policy Name",
  "policyActions": "Allow/Deny Actions",
  "pages": "Pages",
  "pageName": "Page Name",
  "pageSlot": "Slot",
  "noTools": "No tools",
  "noPolicies": "No policy groups",
  "noPages": "No pages"
}
```

- [ ] **Step 3: Commit**

```bash
cd /Users/codfrm/Code/opskat/opskat
git add frontend/src/i18n/locales/zh-CN/common.json frontend/src/i18n/locales/en/common.json
git commit -m "🌐 Add extension management i18n keys"
```

---

### Task 6: Frontend — ExtensionSection Redesign

**Files:**
- Modify: `frontend/src/components/settings/ExtensionSection.tsx`

- [ ] **Step 1: Rewrite ExtensionSection.tsx**

Replace the entire content of `frontend/src/components/settings/ExtensionSection.tsx`:

```tsx
import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  RefreshCw,
  Puzzle,
  Plus,
  MoreVertical,
  Info,
  Trash2,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Button,
  Switch,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogCancel,
  AlertDialogAction,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Separator,
} from "@opskat/ui";
import {
  ListInstalledExtensions,
  ReloadExtensions,
  InstallExtension,
  UninstallExtension,
  EnableExtension,
  DisableExtension,
} from "../../../wailsjs/go/app/App";

interface ExtInfo {
  name: string;
  version: string;
  icon: string;
  displayName: string;
  description: string;
  enabled: boolean;
  manifest: {
    tools?: { name: string; i18n?: { description?: string } }[];
    policies?: {
      groups?: {
        id: string;
        i18n?: { name?: string };
        policy?: { allow_list?: string[]; deny_list?: string[] };
      }[];
    };
    frontend?: {
      pages?: { id: string; i18n?: { name?: string }; slot?: string }[];
    };
  };
}

export function ExtensionSection() {
  const { t } = useTranslation();
  const [extensions, setExtensions] = useState<ExtInfo[]>([]);
  const [reloading, setReloading] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [uninstallTarget, setUninstallTarget] = useState<ExtInfo | null>(null);
  const [cleanData, setCleanData] = useState(false);
  const [detailTarget, setDetailTarget] = useState<ExtInfo | null>(null);

  const loadExtensions = async () => {
    try {
      const exts = await ListInstalledExtensions();
      setExtensions(exts || []);
    } catch {
      setExtensions([]);
    }
  };

  useEffect(() => {
    loadExtensions();
  }, []);

  const handleReload = async () => {
    setReloading(true);
    try {
      await ReloadExtensions();
      await loadExtensions();
      toast.success(t("extension.reloadSuccess"));
    } catch (e) {
      toast.error(`${t("extension.reloadError")}: ${String(e)}`);
    } finally {
      setReloading(false);
    }
  };

  const handleInstall = async () => {
    setInstalling(true);
    try {
      const result = await InstallExtension();
      if (result) {
        await loadExtensions();
        toast.success(t("extension.installSuccess"));
      }
    } catch (e) {
      toast.error(`${t("extension.installError")}: ${String(e)}`);
    } finally {
      setInstalling(false);
    }
  };

  const handleUninstall = async () => {
    if (!uninstallTarget) return;
    try {
      await UninstallExtension(uninstallTarget.name, cleanData);
      await loadExtensions();
      toast.success(t("extension.uninstallSuccess"));
    } catch (e) {
      toast.error(String(e));
    } finally {
      setUninstallTarget(null);
      setCleanData(false);
    }
  };

  const handleToggle = async (ext: ExtInfo) => {
    try {
      if (ext.enabled) {
        await DisableExtension(ext.name);
        toast.success(t("extension.disableSuccess"));
      } else {
        await EnableExtension(ext.name);
        toast.success(t("extension.enableSuccess"));
      }
      await loadExtensions();
    } catch (e) {
      toast.error(String(e));
    }
  };

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle className="text-base">
              {t("extension.installed")}
            </CardTitle>
            <CardDescription>
              {extensions.length > 0
                ? `${extensions.length} ${t("extension.title").toLowerCase()}`
                : t("extension.noExtensionsDesc")}
            </CardDescription>
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={handleInstall}
              disabled={installing}
              className="gap-1"
            >
              <Plus className="h-3.5 w-3.5" />
              {t("extension.install")}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleReload}
              disabled={reloading}
              className="gap-1"
            >
              <RefreshCw
                className={`h-3.5 w-3.5 ${reloading ? "animate-spin" : ""}`}
              />
              {t("extension.reload")}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {extensions.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              <Puzzle className="h-8 w-8 mx-auto mb-2 opacity-50" />
              <p className="text-sm">{t("extension.noExtensions")}</p>
            </div>
          ) : (
            <div className="space-y-3">
              {extensions.map((ext) => (
                <div
                  key={ext.name}
                  className={`flex items-center justify-between p-3 border rounded-lg ${
                    !ext.enabled ? "opacity-60" : ""
                  }`}
                >
                  <div className="flex items-center gap-3">
                    <div className="h-8 w-8 rounded bg-muted flex items-center justify-center">
                      <Puzzle className="h-4 w-4 text-muted-foreground" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <p className="font-medium text-sm">
                          {ext.displayName || ext.name}
                        </p>
                        {!ext.enabled && (
                          <span className="text-xs px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
                            {t("extension.disabled")}
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {ext.description && <span>{ext.description} · </span>}
                        {t("extension.version")} {ext.version}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Switch
                      checked={ext.enabled}
                      onCheckedChange={() => handleToggle(ext)}
                    />
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                          <MoreVertical className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => setDetailTarget(ext)}>
                          <Info className="h-4 w-4 mr-2" />
                          {t("extension.detail")}
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onClick={() => setUninstallTarget(ext)}
                          className="text-destructive"
                        >
                          <Trash2 className="h-4 w-4 mr-2" />
                          {t("extension.uninstall")}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Uninstall Confirmation Dialog */}
      <AlertDialog
        open={!!uninstallTarget}
        onOpenChange={(open) => {
          if (!open) {
            setUninstallTarget(null);
            setCleanData(false);
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("extension.uninstall")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("extension.uninstallConfirm", {
                name: uninstallTarget?.displayName || uninstallTarget?.name,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="flex items-center gap-3 py-2">
            <Switch
              id="clean-data"
              checked={cleanData}
              onCheckedChange={setCleanData}
            />
            <div>
              <label htmlFor="clean-data" className="text-sm font-medium cursor-pointer">
                {t("extension.cleanData")}
              </label>
              <p className="text-xs text-muted-foreground">
                {t("extension.cleanDataDesc")}
              </p>
            </div>
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={handleUninstall}>
              {t("extension.uninstall")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Detail Dialog */}
      <Dialog
        open={!!detailTarget}
        onOpenChange={(open) => !open && setDetailTarget(null)}
      >
        <DialogContent className="max-w-lg max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {detailTarget?.displayName || detailTarget?.name}
            </DialogTitle>
          </DialogHeader>
          {detailTarget && <ExtensionDetail ext={detailTarget} />}
        </DialogContent>
      </Dialog>
    </>
  );
}

function ExtensionDetail({ ext }: { ext: ExtInfo }) {
  const { t } = useTranslation();
  const tools = ext.manifest?.tools || [];
  const policyGroups = ext.manifest?.policies?.groups || [];
  const pages = ext.manifest?.frontend?.pages || [];

  return (
    <div className="space-y-4">
      {/* Basic Info */}
      <div className="space-y-1 text-sm">
        <p>
          <span className="text-muted-foreground">{t("extension.version")}:</span>{" "}
          {ext.version}
        </p>
        {ext.description && (
          <p className="text-muted-foreground">{ext.description}</p>
        )}
      </div>

      <Separator />

      {/* Tools */}
      <div>
        <h4 className="text-sm font-medium mb-2">{t("extension.tools")}</h4>
        {tools.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("extension.noTools")}</p>
        ) : (
          <div className="space-y-1">
            {tools.map((tool) => (
              <div key={tool.name} className="flex justify-between text-xs p-2 rounded bg-muted/50">
                <span className="font-mono">{tool.name}</span>
                <span className="text-muted-foreground ml-2 text-right">
                  {tool.i18n?.description || ""}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

      <Separator />

      {/* Policy Groups */}
      <div>
        <h4 className="text-sm font-medium mb-2">{t("extension.policies")}</h4>
        {policyGroups.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("extension.noPolicies")}</p>
        ) : (
          <div className="space-y-1">
            {policyGroups.map((pg) => (
              <div key={pg.id} className="text-xs p-2 rounded bg-muted/50">
                <span className="font-medium">{pg.i18n?.name || pg.id}</span>
                {pg.policy && (
                  <div className="mt-1 text-muted-foreground">
                    {pg.policy.allow_list && (
                      <span>Allow: {pg.policy.allow_list.join(", ")}</span>
                    )}
                    {pg.policy.deny_list && (
                      <span className="ml-2">Deny: {pg.policy.deny_list.join(", ")}</span>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      <Separator />

      {/* Pages */}
      <div>
        <h4 className="text-sm font-medium mb-2">{t("extension.pages")}</h4>
        {pages.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("extension.noPages")}</p>
        ) : (
          <div className="space-y-1">
            {pages.map((page) => (
              <div key={page.id} className="flex justify-between text-xs p-2 rounded bg-muted/50">
                <span>{page.i18n?.name || page.id}</span>
                <span className="text-muted-foreground font-mono">{page.slot || "-"}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Generate Wails bindings**

Run: `cd /Users/codfrm/Code/opskat/opskat && make dev`

Or if there's a separate bindings generation step, run that. The new Go methods (`InstallExtension`, `UninstallExtension`, `EnableExtension`, `DisableExtension`) need to be reflected in `frontend/wailsjs/go/app/App.js`.

- [ ] **Step 3: Verify frontend builds**

Run: `cd /Users/codfrm/Code/opskat/opskat/frontend && pnpm lint && pnpm build`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
cd /Users/codfrm/Code/opskat/opskat
git add frontend/src/components/settings/ExtensionSection.tsx
git commit -m "✨ Redesign ExtensionSection with install/uninstall/enable/disable/detail"
```

Note: include any auto-generated Wails binding files if they changed.

---

### Task 7: Integration Verification

- [ ] **Step 1: Full backend build**

Run: `cd /Users/codfrm/Code/opskat/opskat && go build ./...`
Expected: No errors.

- [ ] **Step 2: Run Go tests**

Run: `cd /Users/codfrm/Code/opskat/opskat && make test`
Expected: All tests pass.

- [ ] **Step 3: Frontend lint and build**

Run: `cd /Users/codfrm/Code/opskat/opskat/frontend && pnpm lint && pnpm build`
Expected: No errors.

- [ ] **Step 4: Run frontend tests**

Run: `cd /Users/codfrm/Code/opskat/opskat/frontend && pnpm test`
Expected: All tests pass.
