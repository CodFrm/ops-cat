package extension_svc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opskat/opskat/internal/model/entity/extension_state_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/extension_data_repo"
	"github.com/opskat/opskat/internal/repository/extension_state_repo"
	"github.com/opskat/opskat/internal/service/snippet_svc"
	"github.com/opskat/opskat/pkg/extension"
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

// Service manages the extension lifecycle: init, reload, enable/disable, install/uninstall.
type Service struct {
	manager     *extension.Manager
	bridge      *extension.Bridge
	stateRepo   extension_state_repo.ExtensionStateRepo
	dataRepo    extension_data_repo.ExtensionDataRepo
	assetRepo   asset_repo.AssetRepo
	snippetHook SnippetExtensionHook
	logger      *zap.Logger

	onBridgeChanged func(bridge *extension.Bridge)
	onReload        func()

	mu       sync.Mutex
	initDone atomic.Bool
}

// New creates a new extension lifecycle service. snippetHook is optional — pass nil
// to disable snippet integration (existing tests that don't exercise snippets do this).
func New(
	manager *extension.Manager,
	stateRepo extension_state_repo.ExtensionStateRepo,
	dataRepo extension_data_repo.ExtensionDataRepo,
	assetRepo asset_repo.AssetRepo,
	logger *zap.Logger,
	onBridgeChanged func(bridge *extension.Bridge),
	onReload func(),
	snippetHook SnippetExtensionHook,
) *Service {
	return &Service{
		manager:         manager,
		bridge:          extension.NewBridge(),
		stateRepo:       stateRepo,
		dataRepo:        dataRepo,
		assetRepo:       assetRepo,
		snippetHook:     snippetHook,
		logger:          logger,
		onBridgeChanged: onBridgeChanged,
		onReload:        onReload,
	}
}

// Bridge returns the current extension bridge.
func (s *Service) Bridge() *extension.Bridge { return s.bridge }

// Manager returns the underlying extension manager.
func (s *Service) Manager() *extension.Manager { return s.manager }

// Init scans extensions and applies DB state. Called once at startup.
func (s *Service) Init(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadAndApplyState(ctx)
	s.initDone.Store(true)
	return nil
}

// Reload closes all extensions and reinitializes from disk.
func (s *Service) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.manager.Close(ctx)
	s.loadAndApplyState(ctx)
	s.notifyReload()
	return nil
}

// StartWatch begins filesystem monitoring with debounced reload.
func (s *Service) StartWatch(ctx context.Context) error {
	var (
		timerMu sync.Mutex
		timer   *time.Timer
	)
	return s.manager.Watch(ctx, func() {
		timerMu.Lock()
		defer timerMu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(500*time.Millisecond, func() {
			if err := s.Reload(ctx); err != nil {
				s.logger.Error("debounced reload failed", zap.Error(err))
			}
		})
	})
}

// Enable loads a disabled extension and registers it.
func (s *Service) Enable(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ext := s.manager.GetExtension(name); ext != nil {
		return nil
	}

	dir := s.manager.ExtDir(name)
	loadedManifest, err := s.manager.LoadExtension(ctx, dir)
	if err != nil {
		return fmt.Errorf("load extension: %w", err)
	}

	// Cross-extension snippet category conflict check — mirrors Install. If the
	// extension being enabled was installed while another declarer was also disabled,
	// we only catch the collision now. Roll back the load if we detect one.
	if s.snippetHook != nil && loadedManifest != nil && len(loadedManifest.Snippets.Categories) > 0 {
		if conflict := s.findCrossExtensionCategoryConflict(loadedManifest); conflict != "" {
			if uErr := s.manager.Unload(ctx, name); uErr != nil {
				s.logger.Warn("unload after enable-conflict failed", zap.String("name", name), zap.Error(uErr))
			}
			return fmt.Errorf("cannot enable %q: snippet category %q is already registered by another extension", name, conflict)
		}
	}

	if ext := s.manager.GetExtension(name); ext != nil {
		s.bridge.Register(ext)
	}
	if s.snippetHook != nil {
		s.snippetHook.RefreshCategories()
	}
	s.notifyBridgeChanged()
	s.ensureState(ctx, name, true)
	s.notifyReload()
	return nil
}

// Disable unloads a running extension without removing files.
func (s *Service) Disable(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.bridge.Unregister(name)
	_ = s.manager.Unload(ctx, name)
	if s.snippetHook != nil {
		s.snippetHook.RefreshCategories()
	}
	s.notifyBridgeChanged()
	s.ensureState(ctx, name, false)
	s.notifyReload()
	return nil
}

// Install installs an extension from a file/directory path.
//
// Flow:
//  1. manager.Install parses manifest + copies files + loads WASM.
//  2. Cross-extension snippet category id conflict check: if the new manifest declares
//     a snippets.categories[].id that collides with another currently installed
//     extension's category, roll back via manager.Uninstall and return an error.
//  3. snippet hook: RefreshCategories → SyncExtensionSeeds.
//  4. bridge.Register, notify, persist enabled state.
func (s *Service) Install(ctx context.Context, sourcePath string) (*extension.Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	manifest, err := s.manager.Install(ctx, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("install extension: %w", err)
	}

	// Cross-extension category id dedup (after install+load, before bridge.Register).
	// Intra-manifest duplicates are already rejected by manifest.validate().
	if s.snippetHook != nil && len(manifest.Snippets.Categories) > 0 {
		if conflict := s.findCrossExtensionCategoryConflict(manifest); conflict != "" {
			// Roll back manager-side state (files + loaded WASM).
			if rbErr := s.manager.Uninstall(ctx, manifest.Name); rbErr != nil {
				return nil, fmt.Errorf("cannot install %q: snippet category %q already registered; rollback also failed: %v",
					manifest.Name, conflict, rbErr)
			}
			return nil, fmt.Errorf("cannot install %q: snippet category %q is already registered by another extension",
				manifest.Name, conflict)
		}
	}

	// Snippet hook: refresh categories first so SyncExtensionSeeds sees the new ones when
	// validating seed.category (although manifest already validated, this keeps the
	// registry consistent for immediate queries from the frontend).
	if s.snippetHook != nil {
		s.snippetHook.RefreshCategories()
		seeds := manifestSeedsToSvc(manifest)
		if err := s.snippetHook.SyncExtensionSeeds(ctx, manifest.Name, seeds); err != nil {
			s.logger.Warn("sync extension seeds failed",
				zap.String("name", manifest.Name), zap.Error(err))
		}
	}

	if ext := s.manager.GetExtension(manifest.Name); ext != nil {
		s.bridge.Register(ext)
	}
	s.notifyBridgeChanged()
	s.ensureState(ctx, manifest.Name, true)
	s.notifyReload()
	return manifest, nil
}

// findCrossExtensionCategoryConflict scans every installed extension — loaded AND
// disabled-but-on-disk — and returns the first category id from the new manifest that
// clashes with an existing extension's declaration. Empty string = no conflict.
//
// Why scan disk too: a disabled extension keeps its manifest in the extensions dir but
// is not in manager.ListExtensions(); if we only checked loaded ones, installing a new
// extension could create a latent collision that surfaces later when the disabled one
// is enabled (registry would silently drop one of them).
//
// We filter out the new extension itself so a reinstall/upgrade of the same extension
// does not self-conflict.
func (s *Service) findCrossExtensionCategoryConflict(newManifest *extension.Manifest) string {
	if len(newManifest.Snippets.Categories) == 0 {
		return ""
	}
	newCats := make(map[string]struct{}, len(newManifest.Snippets.Categories))
	for _, c := range newManifest.Snippets.Categories {
		newCats[c.ID] = struct{}{}
	}
	// Deduplicate by extension name — an extension might be both loaded and on disk.
	seen := make(map[string]struct{})
	check := func(name string, mf *extension.Manifest) string {
		if mf == nil || name == newManifest.Name {
			return ""
		}
		if _, dup := seen[name]; dup {
			return ""
		}
		seen[name] = struct{}{}
		for _, c := range mf.Snippets.Categories {
			if _, hit := newCats[c.ID]; hit {
				return c.ID
			}
		}
		return ""
	}
	for _, ext := range s.manager.ListExtensions() {
		if ext == nil {
			continue
		}
		if conflict := check(ext.Name, ext.Manifest); conflict != "" {
			return conflict
		}
	}
	// Also scan disabled extensions on disk. ScanManifests errors are non-fatal here —
	// we'd rather let the loaded-extension check stand than block install on a
	// transient FS hiccup.
	if infos, err := s.manager.ScanManifests(); err == nil {
		for _, info := range infos {
			if info == nil {
				continue
			}
			if conflict := check(info.Name, info.Manifest); conflict != "" {
				return conflict
			}
		}
	} else {
		s.logger.Warn("scan manifests for conflict check failed", zap.Error(err))
	}
	return ""
}

// manifestSeedsToSvc copies manifest.Snippets.Seed into snippet_svc.SeedDef to avoid
// snippet_svc importing pkg/extension.
func manifestSeedsToSvc(m *extension.Manifest) []snippet_svc.SeedDef {
	if m == nil || len(m.Snippets.Seed) == 0 {
		return nil
	}
	out := make([]snippet_svc.SeedDef, 0, len(m.Snippets.Seed))
	for _, s := range m.Snippets.Seed {
		out = append(out, snippet_svc.SeedDef{
			Key:         s.Key,
			Name:        s.Name,
			Category:    s.Category,
			Content:     s.Content,
			Description: s.Description,
		})
	}
	return out
}

// Uninstall removes an extension and optionally cleans its data.
// Pass force=true to skip the orphan-asset check.
func (s *Service) Uninstall(ctx context.Context, name string, cleanData bool, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Orphan check: refuse if active assets reference this extension's asset types.
	if !force && s.assetRepo != nil {
		ext := s.manager.GetExtension(name)
		if ext != nil && len(ext.Manifest.AssetTypes) > 0 {
			assetTypes := make([]string, 0, len(ext.Manifest.AssetTypes))
			for _, at := range ext.Manifest.AssetTypes {
				assetTypes = append(assetTypes, at.Type)
			}
			count, err := s.assetRepo.CountByTypes(ctx, assetTypes)
			if err == nil && count > 0 {
				return fmt.Errorf("cannot uninstall %q: %d asset(s) still reference its asset types %v; delete them first or use force uninstall", name, count, assetTypes)
			}
		}
	}

	// Snippet hook: remove seeds + refresh categories BEFORE manager.Uninstall removes
	// the manifest from memory. Hook failures are logged but do not block uninstall —
	// leftover seeds are cleaned up on the next install or via manual DB ops.
	if s.snippetHook != nil {
		if err := s.snippetHook.RemoveExtensionSeeds(ctx, name); err != nil {
			s.logger.Warn("remove extension seeds failed",
				zap.String("name", name), zap.Error(err))
		}
	}

	s.bridge.Unregister(name)
	s.notifyBridgeChanged()

	if err := s.manager.Uninstall(ctx, name); err != nil {
		return fmt.Errorf("uninstall extension: %w", err)
	}

	// Refresh categories AFTER the extension is unloaded from the manager, so the
	// snippet category registry rebuilds without the uninstalled extension's entries.
	if s.snippetHook != nil {
		s.snippetHook.RefreshCategories()
	}

	if err := s.stateRepo.Delete(ctx, name); err != nil {
		s.logger.Warn("delete extension state", zap.String("name", name), zap.Error(err))
	}
	if cleanData {
		if err := s.dataRepo.DeleteAll(ctx, name); err != nil {
			s.logger.Warn("delete extension data", zap.String("name", name), zap.Error(err))
		}
	}

	s.notifyReload()
	return nil
}

// ListInstalled returns all extensions (enabled and disabled) for the frontend.
// Returns nil immediately if Init has not completed yet (avoids blocking on mutex).
func (s *Service) ListInstalled(lang string) []ExtensionInfo {
	if !s.initDone.Load() {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	loaded := make(map[string]*extension.Extension)
	for _, ext := range s.manager.ListExtensions() {
		loaded[ext.Name] = ext
	}

	allManifests, err := s.manager.ScanManifests()
	if err != nil {
		s.logger.Warn("scan manifests failed", zap.Error(err))
		result := make([]ExtensionInfo, 0, len(loaded))
		for _, ext := range loaded {
			lm := ext.Manifest.Localized(func(key string) string { return ext.Translate(lang, key) })
			result = append(result, ExtensionInfo{
				Name: ext.Name, Version: lm.Version, Icon: lm.Icon,
				DisplayName: lm.I18n.DisplayName, Description: lm.I18n.Description,
				Enabled: true, Manifest: lm,
			})
		}
		return result
	}

	result := make([]ExtensionInfo, 0, len(allManifests))
	for _, mi := range allManifests {
		ext, isLoaded := loaded[mi.Name]
		tr := func(key string) string { return mi.Translate(lang, key) }
		if isLoaded {
			tr = func(key string) string { return ext.Translate(lang, key) }
		}
		lm := mi.Manifest.Localized(tr)
		result = append(result, ExtensionInfo{
			Name: mi.Name, Version: lm.Version, Icon: lm.Icon,
			DisplayName: lm.I18n.DisplayName, Description: lm.I18n.Description,
			Enabled: isLoaded, Manifest: lm,
		})
	}
	return result
}

// GetDetail returns detailed info for a single extension.
func (s *Service) GetDetail(name, lang string) (*ExtensionInfo, error) {
	ext := s.manager.GetExtension(name)
	if ext != nil {
		lm := ext.Manifest.Localized(func(key string) string { return ext.Translate(lang, key) })
		return &ExtensionInfo{
			Name: ext.Name, Version: lm.Version, Icon: lm.Icon,
			DisplayName: lm.I18n.DisplayName, Description: lm.I18n.Description,
			Enabled: true, Manifest: lm,
		}, nil
	}

	dir := s.manager.ExtDir(name)
	mi, err := extension.LoadManifestInfo(dir)
	if err != nil {
		return nil, fmt.Errorf("extension %q not found", name)
	}
	lm := mi.Manifest.Localized(func(key string) string { return mi.Translate(lang, key) })
	return &ExtensionInfo{
		Name: lm.Name, Version: lm.Version, Icon: lm.Icon,
		DisplayName: lm.I18n.DisplayName, Description: lm.I18n.Description,
		Enabled: false, Manifest: lm,
	}, nil
}

// Close shuts down all extensions and releases the compilation cache.
func (s *Service) Close(ctx context.Context) {
	s.manager.Shutdown(ctx)
}

// loadAndApplyState is the single source of truth: scan, register bridge, apply DB state.
func (s *Service) loadAndApplyState(ctx context.Context) {
	// Unregister old bridge entries from package-global registries before replacing the bridge.
	if s.bridge != nil {
		for _, name := range s.bridge.ListNames() {
			s.bridge.Unregister(name)
		}
	}

	if _, err := s.manager.Scan(ctx); err != nil {
		s.logger.Error("scan extensions failed", zap.Error(err))
	}

	s.bridge = extension.NewBridge()
	for _, ext := range s.manager.ListExtensions() {
		s.bridge.Register(ext)
	}

	states, _ := s.stateRepo.FindAll(context.Background())
	for _, state := range states {
		if !state.Enabled {
			s.bridge.Unregister(state.Name)
			_ = s.manager.Unload(ctx, state.Name)
		}
	}

	s.notifyBridgeChanged()
}

func (s *Service) ensureState(ctx context.Context, name string, enabled bool) {
	state, err := s.stateRepo.Find(ctx, name)
	if err != nil {
		if err := s.stateRepo.Create(ctx, &extension_state_entity.ExtensionState{
			Name: name, Enabled: enabled,
		}); err != nil {
			s.logger.Warn("create extension state", zap.String("name", name), zap.Error(err))
		}
		return
	}
	state.Enabled = enabled
	if err := s.stateRepo.Update(ctx, state); err != nil {
		s.logger.Warn("update extension state", zap.String("name", name), zap.Error(err))
	}
}

func (s *Service) notifyBridgeChanged() {
	if s.onBridgeChanged != nil {
		s.onBridgeChanged(s.bridge)
	}
}

func (s *Service) notifyReload() {
	if s.onReload != nil {
		s.onReload()
	}
}
