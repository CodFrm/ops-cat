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
	manager   *extension.Manager
	bridge    *extension.Bridge
	stateRepo extension_state_repo.ExtensionStateRepo
	dataRepo  extension_data_repo.ExtensionDataRepo
	assetRepo asset_repo.AssetRepo
	logger    *zap.Logger

	onBridgeChanged func(bridge *extension.Bridge)
	onReload        func()

	mu       sync.Mutex
	initDone atomic.Bool
}

// New creates a new extension lifecycle service.
func New(
	manager *extension.Manager,
	stateRepo extension_state_repo.ExtensionStateRepo,
	dataRepo extension_data_repo.ExtensionDataRepo,
	assetRepo asset_repo.AssetRepo,
	logger *zap.Logger,
	onBridgeChanged func(bridge *extension.Bridge),
	onReload func(),
) *Service {
	return &Service{
		manager:         manager,
		bridge:          extension.NewBridge(),
		stateRepo:       stateRepo,
		dataRepo:        dataRepo,
		assetRepo:       assetRepo,
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
	err := s.loadAndApplyState(ctx)
	s.initDone.Store(true)
	return err
}

// Reload closes all extensions and reinitializes from disk.
func (s *Service) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.manager.Close(ctx)
	if err := s.loadAndApplyState(ctx); err != nil {
		return err
	}
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
	if _, err := s.manager.LoadExtension(ctx, dir); err != nil {
		return fmt.Errorf("load extension: %w", err)
	}

	if ext := s.manager.GetExtension(name); ext != nil {
		s.bridge.Register(ext)
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
	s.notifyBridgeChanged()
	s.ensureState(ctx, name, false)
	s.notifyReload()
	return nil
}

// Install installs an extension from a file/directory path.
func (s *Service) Install(ctx context.Context, sourcePath string) (*extension.Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	manifest, err := s.manager.Install(ctx, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("install extension: %w", err)
	}

	if ext := s.manager.GetExtension(manifest.Name); ext != nil {
		s.bridge.Register(ext)
	}
	s.notifyBridgeChanged()
	s.ensureState(ctx, manifest.Name, true)
	s.notifyReload()
	return manifest, nil
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

	s.bridge.Unregister(name)
	s.notifyBridgeChanged()

	if err := s.manager.Uninstall(ctx, name); err != nil {
		return fmt.Errorf("uninstall extension: %w", err)
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
func (s *Service) loadAndApplyState(ctx context.Context) error {
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
	return nil
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
