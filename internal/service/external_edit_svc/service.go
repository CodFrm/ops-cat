package external_edit_svc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/repository/audit_repo"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

type Service struct {
	dataDir      string
	storageDir   string
	manifestPath string

	configProvider func() *bootstrap.AppConfig
	configSaver    func(cfg *bootstrap.AppConfig) error
	remote         RemoteFileService
	findSessions   func(assetID int64) []string
	assets         AssetFinder
	auditRepo      audit_repo.AuditRepo
	emit           func(Event)
	launch         Launcher
	now            func() time.Time

	mu              sync.RWMutex
	sessions        map[string]*Session
	watcher         *fsnotify.Watcher
	watchedDirs     map[string]int
	reconcileTimers map[string]*time.Timer
	autoSaveTimers  map[string]*time.Timer
	autoSavePaused  map[string]bool
	autoSaveTried   map[string]string
	documentRunners map[string]*sync.Mutex
	cleanupTicker   *time.Ticker
	closeCh         chan struct{}
	closeOnce       sync.Once
}

func NewService(opts Options) (*Service, error) {
	if opts.DataDir == "" {
		opts.DataDir = bootstrap.AppDataDir()
	}
	if opts.ConfigProvider == nil {
		return nil, fmt.Errorf("missing config provider")
	}
	if opts.ConfigSaver == nil {
		return nil, fmt.Errorf("missing config saver")
	}
	if opts.Remote == nil {
		return nil, fmt.Errorf("missing remote file service")
	}
	if opts.Emit == nil {
		opts.Emit = func(Event) {}
	}
	if opts.Launch == nil {
		opts.Launch = launcherFunc(func(execPath string, args []string) error {
			cmd := exec.Command(execPath, args...) //nolint:gosec // path and args are validated before launch
			return cmd.Start()
		})
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	s := &Service{
		dataDir:         opts.DataDir,
		storageDir:      filepath.Join(opts.DataDir, "storage"),
		manifestPath:    filepath.Join(opts.DataDir, "storage", "manifest.json"),
		configProvider:  opts.ConfigProvider,
		configSaver:     opts.ConfigSaver,
		remote:          opts.Remote,
		findSessions:    opts.FindSessions,
		assets:          opts.Assets,
		auditRepo:       opts.Audit,
		emit:            opts.Emit,
		launch:          opts.Launch,
		now:             opts.Now,
		sessions:        make(map[string]*Session),
		watchedDirs:     make(map[string]int),
		reconcileTimers: make(map[string]*time.Timer),
		autoSaveTimers:  make(map[string]*time.Timer),
		autoSavePaused:  make(map[string]bool),
		autoSaveTried:   make(map[string]string),
		documentRunners: make(map[string]*sync.Mutex),
		closeCh:         make(chan struct{}),
	}

	return s, nil
}

func (s *Service) Start(context.Context) error {
	if err := os.MkdirAll(s.storageDir, 0o700); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	s.watcher = watcher

	if err := s.loadManifest(); err != nil {
		logger.Default().Warn("load external edit manifest", zap.Error(err))
	}

	go s.watchLoop()
	if err := s.restoreSessions(); err != nil {
		return err
	}
	s.startCleanupLoop()
	return nil
}

func (s *Service) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		close(s.closeCh)

		s.mu.Lock()
		for _, timer := range s.reconcileTimers {
			timer.Stop()
		}
		s.reconcileTimers = map[string]*time.Timer{}
		for _, timer := range s.autoSaveTimers {
			timer.Stop()
		}
		s.autoSaveTimers = map[string]*time.Timer{}
		if s.cleanupTicker != nil {
			s.cleanupTicker.Stop()
			s.cleanupTicker = nil
		}
		s.mu.Unlock()

		if s.watcher != nil {
			closeErr = s.watcher.Close()
		}
	})
	return closeErr
}

func (s *Service) GetSettings() (*Settings, error) {
	cfg := s.configProvider()
	if cfg == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	workspaceRoot, err := s.resolveWorkspaceRoot(cfg.ExternalEditWorkspaceRoot)
	if err != nil {
		return nil, err
	}

	editors := s.detectEditors(cfg.ExternalEditCustomEditors, cfg.ExternalEditDefaultEditorID)
	defaultID := cfg.ExternalEditDefaultEditorID
	if defaultID == "" {
		defaultID = firstAvailableEditorID(editors)
	}
	for i := range editors {
		editors[i].Default = editors[i].ID == defaultID
	}

	return &Settings{
		DefaultEditorID:      defaultID,
		WorkspaceRoot:        workspaceRoot,
		CleanupRetentionDays: normalizeCleanupRetentionDays(cfg.ExternalEditCleanupRetentionDays),
		Editors:              editors,
		CustomEditors:        cloneCustomEditors(cfg.ExternalEditCustomEditors),
	}, nil
}

func (s *Service) SaveSettings(input SettingsInput) (*Settings, error) {
	cfg := s.configProvider()
	if cfg == nil {
		return nil, fmt.Errorf("config not loaded")
	}

	workspaceRoot, err := s.resolveWorkspaceRoot(input.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "workspaces"), 0o700); err != nil {
		return nil, fmt.Errorf("create workspace root: %w", err)
	}

	customEditors, err := s.normalizeCustomEditors(input.CustomEditors)
	if err != nil {
		return nil, err
	}

	editors := s.detectEditors(customEditors, input.DefaultEditorID)
	defaultID := strings.TrimSpace(input.DefaultEditorID)
	if defaultID == "" || !containsEditorID(editors, defaultID) {
		defaultID = firstAvailableEditorID(editors)
	}
	if defaultID != "" && !containsAvailableEditor(editors, defaultID) {
		return nil, fmt.Errorf("默认外部编辑器不可用")
	}

	cfg.ExternalEditDefaultEditorID = defaultID
	cfg.ExternalEditWorkspaceRoot = workspaceRoot
	cfg.ExternalEditCustomEditors = customEditors
	cfg.ExternalEditCleanupRetentionDays = normalizeCleanupRetentionDays(input.CleanupRetentionDays)
	if err := s.configSaver(cfg); err != nil {
		return nil, fmt.Errorf("save external edit settings: %w", err)
	}

	s.runRetentionCleanup()
	return s.GetSettings()
}

func (s *Service) ListSessions() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		if isExternalEditClipboardResidueSession(session) {
			continue
		}
		sessions = append(sessions, cloneSession(session))
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt > sessions[j].UpdatedAt
	})
	return sessions
}
