package external_edit_svc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/audit_entity"
	"github.com/opskat/opskat/internal/repository/audit_repo"
	"github.com/opskat/opskat/internal/service/sftp_svc"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const (
	manifestVersion = 2

	// clean / dirty / conflict / remote_missing 描述“当前可继续推进的主会话”；
	// stale / expired 则是保护性状态：前者保留冲突现场但禁止继续回写，后者提醒本地副本已脱离近期活跃窗口。
	sessionStateClean         = "clean"
	sessionStateDirty         = "dirty"
	sessionStateConflict      = "conflict"
	sessionStateRemoteMissing = "remote_missing"
	sessionStateStale         = "stale"
	sessionStateExpired       = "expired"

	saveStatusSaved         = "saved"
	saveStatusConflict      = "conflict_remote_changed"
	saveStatusRemoteMissing = "remote_missing"
	saveStatusReread        = "reread"
	saveStatusNoop          = "noop"

	resolutionOverwrite = "overwrite"
	resolutionRecreate  = "recreate"
	resolutionReread    = "reread"

	eventSessionOpened   = "session_opened"
	eventSessionRestored = "session_restored"
	eventSessionChanged  = "session_changed"
	eventSessionSaved    = "session_saved"
	eventSessionConflict = "session_conflict"
	eventSessionCleaned  = "session_cleaned"
)

const (
	textEncodingUTF8    = "utf-8"
	textEncodingUTF16LE = "utf-16le"
	textEncodingUTF16BE = "utf-16be"
	textEncodingGB18030 = "gb18030"
)

var textExtensions = map[string]struct{}{
	".txt":        {},
	".md":         {},
	".markdown":   {},
	".json":       {},
	".jsonl":      {},
	".yaml":       {},
	".yml":        {},
	".xml":        {},
	".svg":        {},
	".conf":       {},
	".config":     {},
	".ini":        {},
	".log":        {},
	".sql":        {},
	".sh":         {},
	".bash":       {},
	".zsh":        {},
	".fish":       {},
	".ps1":        {},
	".go":         {},
	".ts":         {},
	".tsx":        {},
	".js":         {},
	".jsx":        {},
	".mjs":        {},
	".cjs":        {},
	".css":        {},
	".scss":       {},
	".html":       {},
	".htm":        {},
	".java":       {},
	".kt":         {},
	".py":         {},
	".rb":         {},
	".rs":         {},
	".c":          {},
	".cc":         {},
	".cpp":        {},
	".h":          {},
	".hpp":        {},
	".toml":       {},
	".env":        {},
	".properties": {},
	".csv":        {},
	".tsv":        {},
	".proto":      {},
	".dockerfile": {},
}

type RemoteFileService interface {
	Stat(sessionID, remotePath string) (*sftp_svc.RemoteFileInfo, error)
	ReadFile(sessionID, remotePath string) ([]byte, *sftp_svc.RemoteFileInfo, error)
	WriteFile(sessionID, remotePath string, data []byte) error
}

type AssetFinder interface {
	Find(ctx context.Context, id int64) (*asset_entity.Asset, error)
}

type Launcher interface {
	Launch(path string, args []string) error
}

type launcherFunc func(path string, args []string) error

func (f launcherFunc) Launch(path string, args []string) error {
	return f(path, args)
}

type Settings struct {
	DefaultEditorID string                           `json:"defaultEditorId"`
	WorkspaceRoot   string                           `json:"workspaceRoot"`
	Editors         []Editor                         `json:"editors"`
	CustomEditors   []bootstrap.ExternalEditorConfig `json:"customEditors"`
}

type SettingsInput struct {
	DefaultEditorID string                           `json:"defaultEditorId"`
	WorkspaceRoot   string                           `json:"workspaceRoot"`
	CustomEditors   []bootstrap.ExternalEditorConfig `json:"customEditors"`
}

type Editor struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Args      []string `json:"args,omitempty"`
	BuiltIn   bool     `json:"builtIn"`
	Available bool     `json:"available"`
	Default   bool     `json:"default"`
}

type OpenRequest struct {
	AssetID    int64  `json:"assetId"`
	SessionID  string `json:"sessionId"`
	RemotePath string `json:"remotePath"`
	EditorID   string `json:"editorId,omitempty"`
}

type textEncodingSnapshot struct {
	Encoding   string
	BOM        string
	ByteSample string
}

// Session 是桌面端外部编辑的单一事实记录：
// 它同时串起远端基线、本地副本、编辑器选择、冲突状态和恢复信息，前后端都围绕这份记录推进状态。
type Session struct {
	ID                    string   `json:"id"`
	AssetID               int64    `json:"assetId"`
	AssetName             string   `json:"assetName"`
	SessionID             string   `json:"sessionId"`
	RemotePath            string   `json:"remotePath"`
	RemoteRealPath        string   `json:"remoteRealPath"`
	LocalPath             string   `json:"localPath"`
	WorkspaceRoot         string   `json:"workspaceRoot"`
	WorkspaceDir          string   `json:"workspaceDir"`
	EditorID              string   `json:"editorId"`
	EditorName            string   `json:"editorName"`
	EditorPath            string   `json:"editorPath"`
	EditorArgs            []string `json:"editorArgs,omitempty"`
	OriginalSHA256        string   `json:"originalSha256"`
	OriginalSize          int64    `json:"originalSize"`
	OriginalModTime       int64    `json:"originalModTime"`
	OriginalEncoding      string   `json:"originalEncoding"`
	OriginalBOM           string   `json:"originalBom,omitempty"`
	OriginalByteSample    string   `json:"originalByteSample,omitempty"`
	LastLocalSHA256       string   `json:"lastLocalSha256"`
	Dirty                 bool     `json:"dirty"`
	State                 string   `json:"state"`
	Expired               bool     `json:"expired"`
	SourceSessionID       string   `json:"sourceSessionId,omitempty"`
	SupersededBySessionID string   `json:"supersededBySessionId,omitempty"`
	CreatedAt             int64    `json:"createdAt"`
	UpdatedAt             int64    `json:"updatedAt"`
	LastLaunchedAt        int64    `json:"lastLaunchedAt"`
	LastSyncedAt          int64    `json:"lastSyncedAt"`
}

type SaveResult struct {
	Status  string   `json:"status"`
	Message string   `json:"message,omitempty"`
	Session *Session `json:"session,omitempty"`
}

type auditSessionPayload struct {
	ID                    string `json:"id,omitempty"`
	AssetID               int64  `json:"assetId,omitempty"`
	AssetName             string `json:"assetName,omitempty"`
	SessionID             string `json:"sessionId,omitempty"`
	RemotePath            string `json:"remotePath,omitempty"`
	RemoteRealPath        string `json:"remoteRealPath,omitempty"`
	EditorID              string `json:"editorId,omitempty"`
	EditorName            string `json:"editorName,omitempty"`
	OriginalSize          int64  `json:"originalSize,omitempty"`
	OriginalModTime       int64  `json:"originalModTime,omitempty"`
	OriginalEncoding      string `json:"originalEncoding,omitempty"`
	OriginalBOM           string `json:"originalBom,omitempty"`
	Dirty                 bool   `json:"dirty"`
	State                 string `json:"state,omitempty"`
	Expired               bool   `json:"expired"`
	SourceSessionID       string `json:"sourceSessionId,omitempty"`
	SupersededBySessionID string `json:"supersededBySessionId,omitempty"`
	CreatedAt             int64  `json:"createdAt,omitempty"`
	UpdatedAt             int64  `json:"updatedAt,omitempty"`
	LastLaunchedAt        int64  `json:"lastLaunchedAt,omitempty"`
	LastSyncedAt          int64  `json:"lastSyncedAt,omitempty"`
}

type auditSaveResultPayload struct {
	Status  string               `json:"status,omitempty"`
	Message string               `json:"message,omitempty"`
	Session *auditSessionPayload `json:"session,omitempty"`
}

type Event struct {
	Type       string      `json:"type"`
	Session    *Session    `json:"session,omitempty"`
	SaveResult *SaveResult `json:"saveResult,omitempty"`
}

type manifestFile struct {
	Version  int        `json:"version"`
	Sessions []*Session `json:"sessions"`
}

type Options struct {
	DataDir        string
	ConfigProvider func() *bootstrap.AppConfig
	ConfigSaver    func(cfg *bootstrap.AppConfig) error
	Remote         RemoteFileService
	Assets         AssetFinder
	Audit          audit_repo.AuditRepo
	Emit           func(Event)
	Launch         Launcher
	Now            func() time.Time
}

type Service struct {
	dataDir      string
	storageDir   string
	manifestPath string

	configProvider func() *bootstrap.AppConfig
	configSaver    func(cfg *bootstrap.AppConfig) error
	remote         RemoteFileService
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
		assets:          opts.Assets,
		auditRepo:       opts.Audit,
		emit:            opts.Emit,
		launch:          opts.Launch,
		now:             opts.Now,
		sessions:        make(map[string]*Session),
		watchedDirs:     make(map[string]int),
		reconcileTimers: make(map[string]*time.Timer),
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
	return s.restoreSessions()
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
		DefaultEditorID: defaultID,
		WorkspaceRoot:   workspaceRoot,
		Editors:         editors,
		CustomEditors:   cloneCustomEditors(cfg.ExternalEditCustomEditors),
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
	if defaultID == "" {
		defaultID = firstAvailableEditorID(editors)
	}
	if defaultID != "" && !containsAvailableEditor(editors, defaultID) {
		return nil, fmt.Errorf("默认外部编辑器不可用")
	}

	cfg.ExternalEditDefaultEditorID = defaultID
	cfg.ExternalEditWorkspaceRoot = workspaceRoot
	cfg.ExternalEditCustomEditors = customEditors
	if err := s.configSaver(cfg); err != nil {
		return nil, fmt.Errorf("save external edit settings: %w", err)
	}

	return s.GetSettings()
}

func (s *Service) ListSessions() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, cloneSession(session))
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt > sessions[j].UpdatedAt
	})
	return sessions
}

func (s *Service) Open(ctx context.Context, req OpenRequest) (*Session, error) {
	if req.AssetID <= 0 {
		return nil, fmt.Errorf("assetId 不能为空")
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return nil, fmt.Errorf("sessionId 不能为空")
	}
	if strings.TrimSpace(req.RemotePath) == "" {
		return nil, fmt.Errorf("remotePath 不能为空")
	}

	editor, err := s.resolveEditor(req.EditorID)
	if err != nil {
		return nil, err
	}

	info, err := s.remote.Stat(req.SessionID, req.RemotePath)
	if err != nil {
		return nil, fmt.Errorf("读取远程文件信息失败: %w", err)
	}
	if info.IsDir || !info.Regular {
		return nil, fmt.Errorf("仅支持打开常规文本文件")
	}
	remoteRealPath := canonicalRemotePath(info, req.RemotePath)
	assetName := s.lookupAssetName(ctx, req.AssetID)
	nowUnix := s.now().Unix()

	s.mu.Lock()
	var reusable *Session
	// 这里优先复用已有主会话，而不是每次都重新拉一份本地副本：
	// 这样可以保留未保存的本地修改、watch 状态和审计上下文，避免双击同一文件时产生多份互相竞争的工作副本。
	for _, existing := range s.sessions {
		if existing.AssetID != req.AssetID || existing.RemoteRealPath != remoteRealPath || existing.State == sessionStateStale {
			continue
		}
		if _, statErr := os.Stat(existing.LocalPath); statErr != nil {
			s.removeSessionLocked(existing.ID)
			continue
		}
		if reusable == nil || existing.UpdatedAt > reusable.UpdatedAt {
			reusable = existing
		}
	}
	if reusable != nil {
		reusable.SessionID = req.SessionID
		reusable.AssetName = assetName
		reusable.RemotePath = req.RemotePath
		reusable.EditorID = editor.ID
		reusable.EditorName = editor.Name
		reusable.EditorPath = editor.Path
		reusable.EditorArgs = cloneArgs(editor.Args)
		reusable.LastLaunchedAt = nowUnix
		reusable.UpdatedAt = nowUnix
		if err := s.saveManifestLocked(); err != nil {
			s.mu.Unlock()
			return nil, err
		}
		session := cloneSession(reusable)
		s.mu.Unlock()

		if err := s.launch.Launch(editor.Path, append(cloneArgs(editor.Args), reusable.LocalPath)); err != nil {
			s.writeAudit(session, "external_edit_open", false, req, nil, err)
			return nil, fmt.Errorf("启动外部编辑器失败: %w", err)
		}
		s.writeAudit(session, "external_edit_open", true, req, map[string]any{"reuse": true}, nil)
		s.emit(Event{Type: eventSessionOpened, Session: session})
		return session, nil
	}
	s.mu.Unlock()

	data, fileInfo, err := s.remote.ReadFile(req.SessionID, req.RemotePath)
	if err != nil {
		return nil, fmt.Errorf("读取远程文件失败: %w", err)
	}
	if fileInfo.IsDir || !fileInfo.Regular {
		return nil, fmt.Errorf("仅支持打开常规文本文件")
	}
	if !isLikelyText(req.RemotePath, data) {
		return nil, fmt.Errorf("当前文件不是可编辑文本文件")
	}
	// 外部编辑链路必须先锁定原始编码/BOM，后续保存时才能判断“用户改的是文本内容”还是“编辑器偷偷改了编码容器”。
	encodingSnapshot, err := detectTextEncoding(data)
	if err != nil {
		return nil, fmt.Errorf("当前文件编码暂不支持外部编辑: %w", err)
	}

	cfg := s.configProvider()
	if cfg == nil {
		return nil, fmt.Errorf("config not loaded")
	}
	workspaceRoot, err := s.resolveWorkspaceRoot(cfg.ExternalEditWorkspaceRoot)
	if err != nil {
		return nil, err
	}
	sessionToken := uuid.NewString()
	localPath, workspaceDir, err := buildWorkspacePaths(workspaceRoot, req.AssetID, canonicalRemotePath(fileInfo, req.RemotePath), sessionToken)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建临时工作区失败: %w", err)
	}
	if err := os.WriteFile(localPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("写入临时副本失败: %w", err)
	}

	session := &Session{
		ID:              sessionToken,
		AssetID:         req.AssetID,
		AssetName:       assetName,
		SessionID:       req.SessionID,
		RemotePath:      req.RemotePath,
		RemoteRealPath:  canonicalRemotePath(fileInfo, req.RemotePath),
		LocalPath:       localPath,
		WorkspaceRoot:   workspaceRoot,
		WorkspaceDir:    workspaceDir,
		EditorID:        editor.ID,
		EditorName:      editor.Name,
		EditorPath:      editor.Path,
		EditorArgs:      cloneArgs(editor.Args),
		OriginalSHA256:  fileInfo.SHA256,
		OriginalSize:    fileInfo.Size,
		OriginalModTime: fileInfo.ModTime,
		LastLocalSHA256: fileInfo.SHA256,
		State:           sessionStateClean,
		CreatedAt:       nowUnix,
		UpdatedAt:       nowUnix,
		LastLaunchedAt:  nowUnix,
		LastSyncedAt:    nowUnix,
	}
	applyEncodingSnapshot(session, encodingSnapshot)

	s.mu.Lock()
	s.sessions[session.ID] = session
	// 只有在会话和 watcher 都注册成功后才允许落 manifest；
	// 否则下次恢复会看到一份不能追踪本地变化的残缺会话。
	if err := s.addWatchLocked(workspaceDir); err != nil {
		delete(s.sessions, session.ID)
		s.mu.Unlock()
		_ = os.RemoveAll(workspaceDir)
		return nil, err
	}
	if err := s.saveManifestLocked(); err != nil {
		s.removeSessionLocked(session.ID)
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()

	if err := s.launch.Launch(editor.Path, append(cloneArgs(editor.Args), localPath)); err != nil {
		s.cleanupSessionAfterLaunchFailure(session.ID)
		s.writeAudit(session, "external_edit_open", false, req, nil, err)
		return nil, fmt.Errorf("启动外部编辑器失败: %w", err)
	}

	cloned := cloneSession(session)
	s.writeAudit(cloned, "external_edit_open", true, req, nil, nil)
	s.emit(Event{Type: eventSessionOpened, Session: cloned})
	return cloned, nil
}

func (s *Service) Save(ctx context.Context, sessionID string) (*SaveResult, error) {
	return s.saveInternal(ctx, sessionID, "")
}

func (s *Service) Resolve(ctx context.Context, sessionID, resolution string) (*SaveResult, error) {
	switch resolution {
	case resolutionOverwrite, resolutionRecreate:
	case resolutionReread:
		return s.rereadRemoteSession(sessionID)
	default:
		return nil, fmt.Errorf("未知冲突处理动作: %s", resolution)
	}
	return s.saveInternal(ctx, sessionID, resolution)
}

func (s *Service) saveInternal(ctx context.Context, sessionID, resolution string) (*SaveResult, error) {
	session := s.getSession(sessionID)
	if session == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}

	localData, err := os.ReadFile(session.LocalPath) //nolint:gosec // local path is controlled by the service workspace
	if err != nil {
		return nil, fmt.Errorf("读取本地副本失败: %w", err)
	}
	if !isLikelyText(session.RemotePath, localData) {
		return nil, fmt.Errorf("本地副本已不是可编辑文本文件")
	}

	localHash := hashBytes(localData)
	// dirty 标记来自 watcher，hash 则来自当前磁盘内容；
	// 两者同时成立才说明确实需要回写，避免“watch 尚未来得及落地”或“内容没变只是触发了写入时间”造成误保存。
	if localHash == session.OriginalSHA256 && !session.Dirty {
		result := &SaveResult{
			Status:  saveStatusNoop,
			Message: "本地副本没有新的变更",
			Session: cloneSession(session),
		}
		return result, nil
	}
	if err := validateRoundTrip(session, localData); err != nil {
		s.writeAudit(session, "external_edit_save_validation_failed", false, map[string]any{"resolution": resolution}, nil, err)
		return nil, err
	}

	// 保存前永远重新读取远端状态。
	// overwrite / recreate 是显式用户决策；除此之外一旦发现远端内容漂移或文件缺失，就必须先停在冲突态，不能偷偷覆盖。
	currentInfo, err := s.remote.Stat(session.SessionID, session.RemotePath)
	if err != nil {
		if !isRemoteMissingError(err) {
			return nil, fmt.Errorf("读取远程文件状态失败: %w", err)
		}
		if resolution != resolutionRecreate {
			result := s.markSessionState(sessionID, sessionStateRemoteMissing, true, localHash)
			saveResult := &SaveResult{
				Status:  saveStatusRemoteMissing,
				Message: "远程文件不存在，可选择重新保存",
				Session: result,
			}
			s.writeAudit(result, "external_edit_conflict_remote_missing", true, map[string]any{"resolution": resolution}, saveResult, nil)
			s.emit(Event{Type: eventSessionConflict, Session: result, SaveResult: saveResult})
			return saveResult, nil
		}
	} else {
		if currentInfo.IsDir || !currentInfo.Regular {
			return nil, fmt.Errorf("远程路径已不是常规文件")
		}

		if resolution != resolutionOverwrite {
			remoteData, remoteInfo, readErr := s.remote.ReadFile(session.SessionID, session.RemotePath)
			if readErr != nil {
				if isRemoteMissingError(readErr) {
					result := s.markSessionState(sessionID, sessionStateRemoteMissing, true, localHash)
					saveResult := &SaveResult{
						Status:  saveStatusRemoteMissing,
						Message: "远程文件不存在，可选择重新保存",
						Session: result,
					}
					s.writeAudit(result, "external_edit_conflict_remote_missing", true, map[string]any{"resolution": resolution}, saveResult, nil)
					s.emit(Event{Type: eventSessionConflict, Session: result, SaveResult: saveResult})
					return saveResult, nil
				}
				return nil, fmt.Errorf("读取远程文件失败: %w", readErr)
			}
			if remoteInfo.SHA256 != session.OriginalSHA256 {
				result := s.markSessionState(sessionID, sessionStateConflict, true, localHash)
				saveResult := &SaveResult{
					Status:  saveStatusConflict,
					Message: "远程文件已变更，可选择覆盖保存",
					Session: result,
				}
				s.writeAudit(result, "external_edit_conflict_remote_changed", true, map[string]any{"resolution": resolution, "remoteSha256": remoteInfo.SHA256, "remoteBytes": len(remoteData)}, saveResult, nil)
				s.emit(Event{Type: eventSessionConflict, Session: result, SaveResult: saveResult})
				return saveResult, nil
			}
		}
	}

	if err := s.remote.WriteFile(session.SessionID, session.RemotePath, localData); err != nil {
		s.writeAudit(session, "external_edit_save", false, map[string]any{"resolution": resolution}, nil, err)
		return nil, fmt.Errorf("保存远程文件失败: %w", err)
	}

	// 回写成功后立即回收新的远端元信息，确保后续冲突比较基线更新到“刚刚保存成功的版本”，
	// 否则下一次 watcher 触发会误把自己刚写回的内容当成远端漂移。
	updatedInfo, err := s.remote.Stat(session.SessionID, session.RemotePath)
	if err != nil {
		logger.Default().Warn("stat remote file after external edit save", zap.String("path", session.RemotePath), zap.Error(err))
	}
	savedSession, err := s.markSaved(sessionID, localHash, localData, updatedInfo)
	if err != nil {
		return nil, err
	}

	saveResult := &SaveResult{
		Status:  saveStatusSaved,
		Message: "远程文件已保存",
		Session: savedSession,
	}
	toolName := "external_edit_save"
	if resolution == resolutionOverwrite {
		toolName = "external_edit_overwrite"
	}
	if resolution == resolutionRecreate {
		toolName = "external_edit_recreate"
	}
	s.writeAudit(savedSession, toolName, true, map[string]any{"resolution": resolution, "bytes": len(localData)}, saveResult, nil)
	s.emit(Event{Type: eventSessionSaved, Session: savedSession, SaveResult: saveResult})
	return saveResult, nil
}

func (s *Service) loadManifest() error {
	data, err := os.ReadFile(s.manifestPath) //nolint:gosec // manifest path is controlled by the application data dir
	if err != nil {
		if os.IsNotExist(err) {
			return s.writeManifest(&manifestFile{Version: manifestVersion, Sessions: []*Session{}})
		}
		return fmt.Errorf("read manifest: %w", err)
	}

	var manifest manifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.Version == 0 {
		manifest.Version = manifestVersion
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range manifest.Sessions {
		if session == nil || session.ID == "" {
			continue
		}
		s.sessions[session.ID] = session
	}
	return nil
}

func (s *Service) restoreSessions() error {
	now := s.now()
	expireAt := now.Add(-7 * 24 * time.Hour).Unix()
	cleanupAt := now.Add(-30 * 24 * time.Hour).Unix()

	var restored []*Session
	var cleaned []string

	s.mu.Lock()
	// 恢复阶段分两层处理：
	// 30 天以上或副本已丢失的会话直接清理，避免历史垃圾长期占用本地工作区；
	// 7 天以上但副本仍在的会话保留为 expired，给用户一个可见但受限的恢复入口。
	for id, session := range s.sessions {
		if session == nil {
			delete(s.sessions, id)
			continue
		}
		if session.UpdatedAt <= cleanupAt {
			cleaned = append(cleaned, id)
			s.removeSessionLocked(id)
			continue
		}
		if _, err := os.Stat(session.LocalPath); err != nil {
			cleaned = append(cleaned, id)
			s.removeSessionLocked(id)
			continue
		}
		if err := s.hydrateSessionEncodingLocked(session); err != nil {
			logger.Default().Warn("restore external edit encoding metadata", zap.String("sessionId", id), zap.Error(err))
			cleaned = append(cleaned, id)
			s.removeSessionLocked(id)
			continue
		}
		if session.UpdatedAt <= expireAt {
			session.Expired = true
			session.State = sessionStateExpired
		}
		if err := s.addWatchLocked(session.WorkspaceDir); err != nil {
			logger.Default().Warn("restore external edit watcher", zap.String("path", session.WorkspaceDir), zap.Error(err))
			continue
		}
		restored = append(restored, cloneSession(session))
	}
	saveErr := s.saveManifestLocked()
	s.mu.Unlock()
	if saveErr != nil {
		return saveErr
	}

	for _, session := range restored {
		s.emit(Event{Type: eventSessionRestored, Session: session})
	}
	for _, id := range cleaned {
		s.emit(Event{Type: eventSessionCleaned, Session: &Session{ID: id}})
	}
	return nil
}

func (s *Service) watchLoop() {
	for {
		select {
		case <-s.closeCh:
			return
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			// 这里只监听会影响文件最终内容的事件。
			// chmod 等元信息变化不应该把会话错误地标成 dirty，否则不同平台编辑器的保存行为会制造噪声。
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			s.scheduleReconcile(event.Name)
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			logger.Default().Warn("external edit watcher error", zap.Error(err))
		}
	}
}

func (s *Service) scheduleReconcile(changedPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, session := range s.sessions {
		if filepath.Dir(changedPath) != session.WorkspaceDir {
			continue
		}
		if timer, ok := s.reconcileTimers[id]; ok {
			timer.Stop()
		}
		sessionID := id
		s.reconcileTimers[id] = time.AfterFunc(250*time.Millisecond, func() {
			s.reconcileLocalCopy(sessionID)
		})
	}
}

func (s *Service) reconcileLocalCopy(sessionID string) {
	session := s.getSession(sessionID)
	if session == nil {
		return
	}

	data, err := os.ReadFile(session.LocalPath) //nolint:gosec // local path is controlled by the service workspace
	if err != nil {
		return
	}
	localHash := hashBytes(data)
	dirty := localHash != session.OriginalSHA256
	nextState := sessionStateClean
	if session.Expired {
		nextState = sessionStateExpired
	} else if session.State == sessionStateStale {
		nextState = sessionStateStale
	} else if dirty {
		nextState = sessionStateDirty
	}

	s.mu.Lock()
	current := s.sessions[sessionID]
	if current == nil {
		s.mu.Unlock()
		return
	}
	if current.LastLocalSHA256 == localHash && current.Dirty == dirty && current.State == nextState {
		s.mu.Unlock()
		return
	}
	current.LastLocalSHA256 = localHash
	current.Dirty = dirty
	current.State = nextState
	current.UpdatedAt = s.now().Unix()
	err = s.saveManifestLocked()
	cloned := cloneSession(current)
	s.mu.Unlock()
	if err != nil {
		logger.Default().Warn("persist external edit manifest after local change", zap.Error(err))
		return
	}
	s.emit(Event{Type: eventSessionChanged, Session: cloned})
}

func (s *Service) getSession(sessionID string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSession(s.sessions[sessionID])
}

func (s *Service) markSaved(sessionID, localHash string, localData []byte, remoteInfo *sftp_svc.RemoteFileInfo) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.sessions[sessionID]
	if session == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	if remoteInfo != nil {
		session.OriginalSize = remoteInfo.Size
		session.OriginalModTime = remoteInfo.ModTime
		session.RemoteRealPath = canonicalRemotePath(remoteInfo, session.RemotePath)
	} else {
		session.OriginalSize = int64(len(localData))
	}
	session.OriginalSHA256 = localHash
	session.OriginalByteSample = byteSampleHex(localData)
	session.LastLocalSHA256 = localHash
	session.Dirty = false
	session.State = sessionStateClean
	session.Expired = false
	session.SupersededBySessionID = ""
	session.UpdatedAt = s.now().Unix()
	session.LastSyncedAt = session.UpdatedAt
	if err := s.saveManifestLocked(); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func (s *Service) markSessionState(sessionID, state string, dirty bool, localHash string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.sessions[sessionID]
	if session == nil {
		return nil
	}
	session.State = state
	session.Dirty = dirty
	if localHash != "" {
		session.LastLocalSHA256 = localHash
	}
	session.UpdatedAt = s.now().Unix()
	if err := s.saveManifestLocked(); err != nil {
		logger.Default().Warn("persist external edit manifest after state change", zap.Error(err))
	}
	return cloneSession(session)
}

func (s *Service) addWatchLocked(dir string) error {
	if dir == "" {
		return fmt.Errorf("empty watch dir")
	}
	if s.watchedDirs[dir] > 0 {
		s.watchedDirs[dir]++
		return nil
	}
	if err := s.watcher.Add(dir); err != nil {
		return fmt.Errorf("watch workspace dir: %w", err)
	}
	s.watchedDirs[dir] = 1
	return nil
}

func (s *Service) removeWatchLocked(dir string) {
	if dir == "" {
		return
	}
	count := s.watchedDirs[dir]
	if count <= 1 {
		delete(s.watchedDirs, dir)
		if err := s.watcher.Remove(dir); err != nil && !strings.Contains(strings.ToLower(err.Error()), "can't remove non-existent") {
			logger.Default().Warn("remove external edit watcher", zap.String("path", dir), zap.Error(err))
		}
		return
	}
	s.watchedDirs[dir] = count - 1
}

func (s *Service) removeSessionLocked(sessionID string) {
	session := s.sessions[sessionID]
	if session == nil {
		return
	}
	s.removeWatchLocked(session.WorkspaceDir)
	delete(s.sessions, sessionID)
	if timer, ok := s.reconcileTimers[sessionID]; ok {
		timer.Stop()
		delete(s.reconcileTimers, sessionID)
	}
	if err := cleanupWorkspace(session.WorkspaceRoot, session.WorkspaceDir); err != nil {
		logger.Default().Warn("cleanup external edit workspace", zap.String("path", session.WorkspaceDir), zap.Error(err))
	}
}

func (s *Service) saveManifestLocked() error {
	manifest := &manifestFile{
		Version:  manifestVersion,
		Sessions: make([]*Session, 0, len(s.sessions)),
	}
	for _, session := range s.sessions {
		manifest.Sessions = append(manifest.Sessions, cloneSession(session))
	}
	sort.Slice(manifest.Sessions, func(i, j int) bool {
		return manifest.Sessions[i].UpdatedAt > manifest.Sessions[j].UpdatedAt
	})
	return s.writeManifest(manifest)
}

func (s *Service) writeManifest(manifest *manifestFile) error {
	if err := os.MkdirAll(s.storageDir, 0o700); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(s.manifestPath, data, 0o600); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func (s *Service) detectEditors(customEditors []bootstrap.ExternalEditorConfig, defaultID string) []Editor {
	editors := make([]Editor, 0, 8)
	seen := make(map[string]struct{})

	for _, editor := range builtInEditors() {
		if _, err := validateExecutable(editor.Path); err == nil {
			editor.Available = true
		}
		editor.Default = editor.ID == defaultID
		editors = append(editors, editor)
		seen[editor.ID] = struct{}{}
	}

	for _, editor := range customEditors {
		id := strings.TrimSpace(editor.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		available := validateCustomEditor(editor) == nil
		editors = append(editors, Editor{
			ID:        id,
			Name:      strings.TrimSpace(editor.Name),
			Path:      strings.TrimSpace(editor.Path),
			Args:      cloneArgs(editor.Args),
			BuiltIn:   false,
			Available: available,
			Default:   id == defaultID,
		})
		seen[id] = struct{}{}
	}

	sort.SliceStable(editors, func(i, j int) bool {
		if editors[i].Available != editors[j].Available {
			return editors[i].Available
		}
		if editors[i].BuiltIn != editors[j].BuiltIn {
			return editors[i].BuiltIn
		}
		return editors[i].Name < editors[j].Name
	})
	return editors
}

func (s *Service) resolveEditor(requestedID string) (*Editor, error) {
	settings, err := s.GetSettings()
	if err != nil {
		return nil, err
	}
	targetID := strings.TrimSpace(requestedID)
	if targetID == "" {
		targetID = settings.DefaultEditorID
	}
	if targetID == "" {
		targetID = firstAvailableEditorID(settings.Editors)
	}
	for _, editor := range settings.Editors {
		if editor.ID != targetID {
			continue
		}
		if !editor.Available {
			return nil, fmt.Errorf("外部编辑器不可用: %s", editor.Name)
		}
		return &editor, nil
	}
	return nil, fmt.Errorf("未找到外部编辑器配置")
}

func (s *Service) normalizeCustomEditors(customEditors []bootstrap.ExternalEditorConfig) ([]bootstrap.ExternalEditorConfig, error) {
	normalized := make([]bootstrap.ExternalEditorConfig, 0, len(customEditors))
	seenNames := make(map[string]struct{})
	seenPaths := make(map[string]struct{})
	seenIDs := make(map[string]struct{})

	for _, editor := range builtInEditors() {
		if editor.ID != "" {
			seenIDs[editor.ID] = struct{}{}
		}
		if name := strings.TrimSpace(editor.Name); name != "" {
			seenNames[strings.ToLower(name)] = struct{}{}
		}
		if path := strings.TrimSpace(editor.Path); path != "" {
			seenPaths[strings.ToLower(path)] = struct{}{}
		}
	}

	for idx, editor := range customEditors {
		editor.ID = strings.TrimSpace(editor.ID)
		editor.Name = strings.TrimSpace(editor.Name)
		editor.Path = strings.TrimSpace(editor.Path)
		editor.Args = trimArgs(editor.Args)
		if editor.ID == "" {
			editor.ID = fmt.Sprintf("custom-%d", idx+1)
		}
		if editor.Name == "" {
			return nil, fmt.Errorf("自定义编辑器名称不能为空")
		}
		if editor.Path == "" {
			return nil, fmt.Errorf("自定义编辑器路径不能为空")
		}
		if _, ok := seenIDs[editor.ID]; ok {
			return nil, fmt.Errorf("存在重复的编辑器 ID: %s", editor.ID)
		}
		if _, ok := seenNames[strings.ToLower(editor.Name)]; ok {
			return nil, fmt.Errorf("存在重复的编辑器名称: %s", editor.Name)
		}
		if _, ok := seenPaths[strings.ToLower(editor.Path)]; ok {
			return nil, fmt.Errorf("存在重复的编辑器路径: %s", editor.Path)
		}
		if err := validateCustomEditor(editor); err != nil {
			return nil, err
		}
		seenIDs[editor.ID] = struct{}{}
		seenNames[strings.ToLower(editor.Name)] = struct{}{}
		seenPaths[strings.ToLower(editor.Path)] = struct{}{}
		normalized = append(normalized, editor)
	}

	return normalized, nil
}

func (s *Service) resolveWorkspaceRoot(configured string) (string, error) {
	workspaceRoot := strings.TrimSpace(configured)
	if workspaceRoot == "" {
		workspaceRoot = filepath.Join(s.dataDir, "tmp")
	}
	if !filepath.IsAbs(workspaceRoot) {
		absPath, err := filepath.Abs(workspaceRoot)
		if err != nil {
			return "", fmt.Errorf("解析临时工作区路径失败: %w", err)
		}
		workspaceRoot = absPath
	}
	return workspaceRoot, nil
}

func (s *Service) lookupAssetName(ctx context.Context, assetID int64) string {
	if s.assets == nil {
		return fmt.Sprintf("asset-%d", assetID)
	}
	asset, err := s.assets.Find(ctx, assetID)
	if err != nil || asset == nil || strings.TrimSpace(asset.Name) == "" {
		return fmt.Sprintf("asset-%d", assetID)
	}
	return asset.Name
}

func (s *Service) writeAudit(session *Session, toolName string, success bool, request any, result any, actionErr error) {
	repo := s.auditRepo
	if repo == nil {
		repo = audit_repo.Audit()
	}
	if repo == nil || session == nil {
		return
	}

	errText := ""
	if actionErr != nil {
		errText = actionErr.Error()
	}

	entry := &audit_entity.AuditLog{
		Source:     "desktop",
		ToolName:   toolName,
		AssetID:    session.AssetID,
		AssetName:  session.AssetName,
		Command:    session.RemotePath,
		Request:    marshalAuditPayload(request, 4096),
		Result:     marshalAuditPayload(result, 8192),
		Error:      truncateText(errText, 2048),
		Success:    boolToSuccess(success),
		SessionID:  session.SessionID,
		Createtime: s.now().Unix(),
	}
	// desktop 审计既要给 QA/SEC 还原状态机，又不能把本地工作区路径、编辑器安装路径等敏感环境信息带进数据库。
	if err := repo.Create(context.Background(), entry); err != nil {
		logger.Default().Warn("write external edit audit log", zap.Error(err))
	}
}

func (s *Service) rereadRemoteSession(sessionID string) (*SaveResult, error) {
	current := s.getSession(sessionID)
	if current == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}

	remoteData, remoteInfo, err := s.remote.ReadFile(current.SessionID, current.RemotePath)
	if err != nil {
		if isRemoteMissingError(err) {
			result := s.markSessionState(sessionID, sessionStateRemoteMissing, true, current.LastLocalSHA256)
			saveResult := &SaveResult{
				Status:  saveStatusRemoteMissing,
				Message: "远程文件不存在，可选择重新保存",
				Session: result,
			}
			s.writeAudit(result, "external_edit_conflict_remote_missing", true, map[string]any{"resolution": resolutionReread}, saveResult, nil)
			s.emit(Event{Type: eventSessionConflict, Session: result, SaveResult: saveResult})
			return saveResult, nil
		}
		return nil, fmt.Errorf("重新读取远程文件失败: %w", err)
	}
	if remoteInfo.IsDir || !remoteInfo.Regular {
		return nil, fmt.Errorf("远程路径已不是常规文件")
	}
	if !isLikelyText(current.RemotePath, remoteData) {
		return nil, fmt.Errorf("当前远程文件不是可编辑文本文件")
	}
	encodingSnapshot, err := detectTextEncoding(remoteData)
	if err != nil {
		return nil, fmt.Errorf("当前远程文件编码暂不支持外部编辑: %w", err)
	}

	sessionToken := uuid.NewString()
	localPath, workspaceDir, err := buildWorkspacePaths(current.WorkspaceRoot, current.AssetID, canonicalRemotePath(remoteInfo, current.RemotePath), sessionToken)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建临时工作区失败: %w", err)
	}
	if err := os.WriteFile(localPath, remoteData, 0o600); err != nil {
		return nil, fmt.Errorf("写入远程新副本失败: %w", err)
	}

	nowUnix := s.now().Unix()
	next := &Session{
		ID:              sessionToken,
		AssetID:         current.AssetID,
		AssetName:       current.AssetName,
		SessionID:       current.SessionID,
		RemotePath:      current.RemotePath,
		RemoteRealPath:  canonicalRemotePath(remoteInfo, current.RemotePath),
		LocalPath:       localPath,
		WorkspaceRoot:   current.WorkspaceRoot,
		WorkspaceDir:    workspaceDir,
		EditorID:        current.EditorID,
		EditorName:      current.EditorName,
		EditorPath:      current.EditorPath,
		EditorArgs:      cloneArgs(current.EditorArgs),
		OriginalSHA256:  remoteInfo.SHA256,
		OriginalSize:    remoteInfo.Size,
		OriginalModTime: remoteInfo.ModTime,
		LastLocalSHA256: remoteInfo.SHA256,
		State:           sessionStateClean,
		SourceSessionID: current.ID,
		CreatedAt:       nowUnix,
		UpdatedAt:       nowUnix,
		LastLaunchedAt:  nowUnix,
		LastSyncedAt:    nowUnix,
	}
	applyEncodingSnapshot(next, encodingSnapshot)

	var staleCopy *Session
	s.mu.Lock()
	original := s.sessions[sessionID]
	if original == nil {
		s.mu.Unlock()
		_ = cleanupWorkspace(current.WorkspaceRoot, workspaceDir)
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	rollback := cloneSession(original)

	s.sessions[next.ID] = next
	// reread 不是覆盖旧副本，而是显式生成一份新的 clean 会话，
	// 同时把旧会话降级为 stale，保证用户仍能回看冲突现场而不会再误把旧副本保存回远端。
	if err := s.addWatchLocked(workspaceDir); err != nil {
		delete(s.sessions, next.ID)
		s.mu.Unlock()
		_ = cleanupWorkspace(current.WorkspaceRoot, workspaceDir)
		return nil, err
	}

	original.State = sessionStateStale
	original.Expired = false
	original.SupersededBySessionID = next.ID
	original.UpdatedAt = nowUnix
	staleCopy = cloneSession(original)
	if err := s.saveManifestLocked(); err != nil {
		s.removeWatchLocked(workspaceDir)
		delete(s.sessions, next.ID)
		s.sessions[rollback.ID] = rollback
		s.mu.Unlock()
		_ = cleanupWorkspace(current.WorkspaceRoot, workspaceDir)
		return nil, err
	}
	openedCopy := cloneSession(next)
	s.mu.Unlock()

	if err := s.launch.Launch(next.EditorPath, append(cloneArgs(next.EditorArgs), next.LocalPath)); err != nil {
		s.rollbackRereadSession(rollback, next.ID)
		s.writeAudit(current, "external_edit_reread", false, map[string]any{"sourceSessionId": sessionID}, nil, err)
		return nil, fmt.Errorf("启动外部编辑器失败: %w", err)
	}

	saveResult := &SaveResult{
		Status:  saveStatusReread,
		Message: "已重新读取远程新版本，旧副本已保留为冲突副本",
		Session: openedCopy,
	}
	s.writeAudit(openedCopy, "external_edit_reread", true, map[string]any{"sourceSessionId": sessionID}, saveResult, nil)
	s.emit(Event{Type: eventSessionChanged, Session: staleCopy})
	s.emit(Event{Type: eventSessionOpened, Session: openedCopy})
	return saveResult, nil
}

func (s *Service) rollbackRereadSession(original *Session, newSessionID string) {
	if original == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	created := s.sessions[newSessionID]
	if created != nil {
		s.removeWatchLocked(created.WorkspaceDir)
		delete(s.sessions, newSessionID)
		if timer, ok := s.reconcileTimers[newSessionID]; ok {
			timer.Stop()
			delete(s.reconcileTimers, newSessionID)
		}
	}
	s.sessions[original.ID] = original
	if err := s.saveManifestLocked(); err != nil {
		logger.Default().Warn("rollback reread external edit manifest", zap.Error(err))
	}
	if created != nil {
		if err := cleanupWorkspace(created.WorkspaceRoot, created.WorkspaceDir); err != nil {
			logger.Default().Warn("cleanup reread workspace", zap.String("path", created.WorkspaceDir), zap.Error(err))
		}
	}
}

func (s *Service) cleanupSessionAfterLaunchFailure(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return
	}
	s.removeSessionLocked(sessionID)
	if err := s.saveManifestLocked(); err != nil {
		logger.Default().Warn("cleanup external edit session after launch failure", zap.String("sessionId", sessionID), zap.Error(err))
	}
}

func (s *Service) hydrateSessionEncodingLocked(session *Session) error {
	if session == nil || strings.TrimSpace(session.OriginalEncoding) != "" {
		return nil
	}
	data, err := os.ReadFile(session.LocalPath) //nolint:gosec // local path is controlled by the service workspace
	if err != nil {
		return fmt.Errorf("读取本地副本失败: %w", err)
	}
	snapshot, err := detectTextEncoding(data)
	if err != nil {
		return err
	}
	applyEncodingSnapshot(session, snapshot)
	if session.OriginalSize == 0 {
		session.OriginalSize = int64(len(data))
	}
	if session.OriginalSHA256 == "" {
		session.OriginalSHA256 = hashBytes(data)
	}
	if session.LastLocalSHA256 == "" {
		session.LastLocalSHA256 = hashBytes(data)
	}
	return nil
}

func applyEncodingSnapshot(session *Session, snapshot *textEncodingSnapshot) {
	if session == nil || snapshot == nil {
		return
	}
	session.OriginalEncoding = snapshot.Encoding
	session.OriginalBOM = snapshot.BOM
	session.OriginalByteSample = snapshot.ByteSample
}

func detectTextEncoding(data []byte) (*textEncodingSnapshot, error) {
	// 这里只接受当前链路可稳定 round-trip 的编码集合。
	// 外部编辑的核心目标是“改文本内容而不破坏文件容器”，因此宁可保守拒绝，也不能把未知编码默默转坏。
	bomName, _, body := splitTextBOM(data)
	switch bomName {
	case textEncodingUTF16LE:
		if _, err := roundTripBody(textEncodingUTF16LE, body); err != nil {
			return nil, err
		}
		return &textEncodingSnapshot{
			Encoding:   textEncodingUTF16LE,
			BOM:        bomName,
			ByteSample: byteSampleHex(data),
		}, nil
	case textEncodingUTF16BE:
		if _, err := roundTripBody(textEncodingUTF16BE, body); err != nil {
			return nil, err
		}
		return &textEncodingSnapshot{
			Encoding:   textEncodingUTF16BE,
			BOM:        bomName,
			ByteSample: byteSampleHex(data),
		}, nil
	case textEncodingUTF8:
		if !utf8.Valid(body) {
			return nil, fmt.Errorf("UTF-8 内容无效")
		}
		return &textEncodingSnapshot{
			Encoding:   textEncodingUTF8,
			BOM:        bomName,
			ByteSample: byteSampleHex(data),
		}, nil
	}
	if utf8.Valid(body) {
		return &textEncodingSnapshot{
			Encoding:   textEncodingUTF8,
			ByteSample: byteSampleHex(data),
		}, nil
	}
	if roundTripped, err := roundTripBody(textEncodingGB18030, body); err == nil && bytes.Equal(roundTripped, body) {
		return &textEncodingSnapshot{
			Encoding:   textEncodingGB18030,
			ByteSample: byteSampleHex(data),
		}, nil
	}
	return nil, fmt.Errorf("暂不支持识别当前文本编码")
}

func validateRoundTrip(session *Session, data []byte) error {
	if session == nil || strings.TrimSpace(session.OriginalEncoding) == "" {
		return fmt.Errorf("当前会话缺少原始编码信息，请重新打开远程文件后再同步")
	}

	// 先校验 BOM，再校验编码回环。
	// 这样能把“编辑器切换编码容器”和“文本内容不可逆”拆成两类可解释错误，方便用户按原编辑器设置回退。
	currentBOM, _, body := splitTextBOM(data)
	if currentBOM != session.OriginalBOM {
		return fmt.Errorf(
			"检测到文件 BOM 已变化（原始 %s，当前 %s），请恢复原始 BOM 后再同步",
			describeBOM(session.OriginalBOM),
			describeBOM(currentBOM),
		)
	}

	roundTripped, err := roundTripBody(session.OriginalEncoding, body)
	if err != nil || !bytes.Equal(roundTripped, body) {
		return fmt.Errorf("检测到文件编码已偏离原始 %s，请使用原始编码重新保存后再同步", describeEncoding(session.OriginalEncoding))
	}
	return nil
}

func splitTextBOM(data []byte) (string, []byte, []byte) {
	switch {
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		return textEncodingUTF8, []byte{0xef, 0xbb, 0xbf}, data[3:]
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		return textEncodingUTF16LE, []byte{0xff, 0xfe}, data[2:]
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		return textEncodingUTF16BE, []byte{0xfe, 0xff}, data[2:]
	default:
		return "", nil, data
	}
}

func roundTripBody(encodingName string, body []byte) ([]byte, error) {
	switch encodingName {
	case textEncodingUTF8:
		if !utf8.Valid(body) {
			return nil, fmt.Errorf("UTF-8 内容无效")
		}
		return append([]byte(nil), body...), nil
	case textEncodingUTF16LE:
		return transformRoundTrip(unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), body)
	case textEncodingUTF16BE:
		return transformRoundTrip(unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), body)
	case textEncodingGB18030:
		return transformRoundTrip(simplifiedchinese.GB18030, body)
	default:
		return nil, fmt.Errorf("未知原始编码: %s", encodingName)
	}
}

func transformRoundTrip(textEncoding encoding.Encoding, body []byte) ([]byte, error) {
	decoderOutput, _, err := transform.Bytes(textEncoding.NewDecoder(), body)
	if err != nil {
		return nil, err
	}
	encoderOutput, _, err := transform.Bytes(textEncoding.NewEncoder(), decoderOutput)
	if err != nil {
		return nil, err
	}
	return encoderOutput, nil
}

func byteSampleHex(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sample := data
	if len(sample) > 24 {
		sample = sample[:24]
	}
	return hex.EncodeToString(sample)
}

func describeBOM(bom string) string {
	switch bom {
	case textEncodingUTF8:
		return "UTF-8 BOM"
	case textEncodingUTF16LE:
		return "UTF-16LE BOM"
	case textEncodingUTF16BE:
		return "UTF-16BE BOM"
	default:
		return "无 BOM"
	}
}

func describeEncoding(name string) string {
	switch name {
	case textEncodingUTF8:
		return "UTF-8"
	case textEncodingUTF16LE:
		return "UTF-16LE"
	case textEncodingUTF16BE:
		return "UTF-16BE"
	case textEncodingGB18030:
		return "GB18030"
	default:
		return name
	}
}

func builtInEditors() []Editor {
	switch {
	case isWindows():
		windir := os.Getenv("WINDIR")
		if windir == "" {
			windir = `C:\Windows`
		}
		localAppData := os.Getenv("LOCALAPPDATA")
		programFiles := os.Getenv("ProgramFiles")
		programFilesX86 := os.Getenv("ProgramFiles(x86)")
		return []Editor{
			{
				ID:      "cursor",
				Name:    "Cursor",
				Path:    firstExistingPath([]string{filepath.Join(localAppData, "Programs", "Cursor", "Cursor.exe"), filepath.Join(programFiles, "Cursor", "Cursor.exe")}),
				BuiltIn: true,
			},
			{
				ID:      "vscode",
				Name:    "VS Code",
				Path:    firstExistingPath([]string{filepath.Join(localAppData, "Programs", "Microsoft VS Code", "Code.exe"), filepath.Join(programFiles, "Microsoft VS Code", "Code.exe"), filepath.Join(programFilesX86, "Microsoft VS Code", "Code.exe")}),
				BuiltIn: true,
			},
			{
				ID:      "typora",
				Name:    "Typora",
				Path:    firstExistingPath([]string{filepath.Join(localAppData, "Programs", "Typora", "Typora.exe"), filepath.Join(programFiles, "Typora", "Typora.exe"), filepath.Join(programFilesX86, "Typora", "Typora.exe")}),
				BuiltIn: true,
			},
			{
				ID:      "system-text",
				Name:    "System Text Editor",
				Path:    filepath.Join(windir, "System32", "notepad.exe"),
				BuiltIn: true,
			},
		}
	default:
		return []Editor{
			{
				ID:      "cursor",
				Name:    "Cursor",
				Path:    firstExistingPath([]string{"/Applications/Cursor.app/Contents/MacOS/Cursor", "/usr/bin/cursor"}),
				BuiltIn: true,
			},
			{
				ID:      "vscode",
				Name:    "VS Code",
				Path:    firstExistingPath([]string{"/Applications/Visual Studio Code.app/Contents/MacOS/Electron", "/usr/bin/code"}),
				BuiltIn: true,
			},
			{
				ID:      "typora",
				Name:    "Typora",
				Path:    firstExistingPath([]string{"/Applications/Typora.app/Contents/MacOS/Typora", "/usr/bin/typora"}),
				BuiltIn: true,
			},
			{
				ID:      "system-text",
				Name:    "System Text Editor",
				Path:    firstExistingPath([]string{"/usr/bin/open", "/usr/bin/xdg-open", "/bin/xdg-open"}),
				Args:    nil,
				BuiltIn: true,
			},
		}
	}
}

func buildWorkspacePaths(workspaceRoot string, assetID int64, remotePath, sessionToken string) (string, string, error) {
	safeRemote := sanitizeRemotePath(remotePath)
	if safeRemote == "" {
		return "", "", fmt.Errorf("无法构建本地临时副本路径")
	}
	hashPrefix := shortHash(remotePath)
	tokenPrefix := shortHash(sessionToken)
	workspaceDir := filepath.Join(workspaceRoot, "workspaces", fmt.Sprintf("asset-%d", assetID), hashPrefix, tokenPrefix, filepath.Dir(safeRemote))
	localPath := filepath.Join(workspaceDir, filepath.Base(safeRemote))
	return localPath, workspaceDir, nil
}

func sanitizeRemotePath(remotePath string) string {
	cleaned := path.Clean(strings.TrimSpace(remotePath))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." || cleaned == "" {
		return ""
	}
	parts := strings.Split(cleaned, "/")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			part = "_"
		}
		replacer := strings.NewReplacer(":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", "\\", "_")
		part = replacer.Replace(part)
		if part == "" {
			part = "_"
		}
		parts[i] = part
	}
	return filepath.Join(parts...)
}

func cleanupWorkspace(workspaceRoot, targetDir string) error {
	if strings.TrimSpace(workspaceRoot) == "" || strings.TrimSpace(targetDir) == "" {
		return nil
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	// 这里必须强约束删除范围始终留在工作区根目录内；
	// 会话清理是自动流程，一旦路径逃逸就会把桌面端的“过期副本清扫”升级成危险删除。
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return fmt.Errorf("cleanup target escapes workspace root")
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	for parent := filepath.Dir(target); parent != "." && parent != root && strings.HasPrefix(parent, root+string(os.PathSeparator)); parent = filepath.Dir(parent) {
		if err := os.Remove(parent); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			var pathErr *os.PathError
			if ok := errors.As(err, &pathErr); ok && pathErr.Err != nil {
				if pathErr.Err.Error() == "directory not empty" {
					break
				}
			}
			break
		}
	}
	return nil
}

func validateCustomEditor(editor bootstrap.ExternalEditorConfig) error {
	if _, err := validateExecutable(editor.Path); err != nil {
		return err
	}
	return nil
}

func validateExecutable(execPath string) (string, error) {
	execPath = strings.TrimSpace(execPath)
	if execPath == "" {
		return "", fmt.Errorf("编辑器路径不能为空")
	}
	if !filepath.IsAbs(execPath) {
		return "", fmt.Errorf("编辑器路径必须是绝对路径")
	}
	ext := strings.ToLower(filepath.Ext(execPath))
	if ext == ".bat" || ext == ".cmd" {
		return "", fmt.Errorf("不允许使用 .bat 或 .cmd 作为外部编辑器")
	}
	info, err := os.Stat(execPath) //nolint:gosec // path is validated and explicitly provided by the user or built-in detector
	if err != nil {
		return "", fmt.Errorf("外部编辑器不可访问: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("外部编辑器路径不能是目录")
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("外部编辑器路径必须是常规文件")
	}
	if isWindows() {
		if ext != ".exe" {
			return "", fmt.Errorf("Windows 外部编辑器必须是 .exe 文件")
		}
		return execPath, nil
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("外部编辑器缺少执行权限")
	}
	return execPath, nil
}

func containsAvailableEditor(editors []Editor, editorID string) bool {
	for _, editor := range editors {
		if editor.ID == editorID && editor.Available {
			return true
		}
	}
	return false
}

func firstAvailableEditorID(editors []Editor) string {
	for _, editor := range editors {
		if editor.Available {
			return editor.ID
		}
	}
	return ""
}

func firstExistingPath(paths []string) string {
	for _, candidate := range paths {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if !filepath.IsAbs(candidate) {
			continue
		}
		if _, err := os.Stat(candidate); err == nil { //nolint:gosec // built-in candidates are static absolute paths
			return candidate
		}
	}
	return ""
}

func canonicalRemotePath(info *sftp_svc.RemoteFileInfo, fallback string) string {
	if info != nil && strings.TrimSpace(info.RealPath) != "" {
		return info.RealPath
	}
	return fallback
}

func isRemoteMissingError(err error) bool {
	if err == nil {
		return false
	}
	if os.IsNotExist(err) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "no such file") || strings.Contains(text, "not found")
}

func isLikelyText(filename string, data []byte) bool {
	if len(data) == 0 {
		return true
	}
	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	if bytes.HasPrefix(sample, []byte{0xff, 0xfe}) || bytes.HasPrefix(sample, []byte{0xfe, 0xff}) {
		return true
	}
	if bytes.IndexByte(sample, 0) >= 0 {
		return false
	}

	contentType := http.DetectContentType(sample)
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	if contentType == "application/json" || contentType == "application/xml" || contentType == "image/svg+xml" || contentType == "application/x-empty" {
		return true
	}
	if _, ok := textExtensions[strings.ToLower(path.Ext(filename))]; ok {
		return looksLikeText(sample)
	}
	return looksLikeText(sample)
}

func looksLikeText(sample []byte) bool {
	if utf8.Valid(sample) {
		return true
	}
	control := 0
	for _, b := range sample {
		if b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		if b < 0x20 {
			control++
		}
	}
	return control*20 < len(sample)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func marshalAuditPayload(payload any, limit int) string {
	if payload == nil {
		return ""
	}
	data, err := json.Marshal(sanitizeAuditPayload(payload))
	if err != nil {
		return ""
	}
	return truncateText(string(data), limit)
}

func sanitizeAuditPayload(payload any) any {
	// 审计脱敏发生在统一入口，而不是调用方各自删字段，
	// 这样新增审计场景时不会因为忘记过滤本地路径/哈希而把敏感信息写入库表。
	switch value := payload.(type) {
	case nil:
		return nil
	case OpenRequest:
		return sanitizeAuditOpenRequest(value)
	case *OpenRequest:
		if value == nil {
			return nil
		}
		return sanitizeAuditOpenRequest(*value)
	case SaveResult:
		return sanitizeAuditSaveResult(&value)
	case *SaveResult:
		return sanitizeAuditSaveResult(value)
	case Session:
		return sanitizeAuditSession(&value)
	case *Session:
		return sanitizeAuditSession(value)
	case map[string]any:
		return sanitizeAuditMap(value)
	case []any:
		items := make([]any, 0, len(value))
		for _, item := range value {
			items = append(items, sanitizeAuditPayload(item))
		}
		return items
	default:
		return payload
	}
}

func sanitizeAuditOpenRequest(req OpenRequest) map[string]any {
	return map[string]any{
		"assetId":    req.AssetID,
		"sessionId":  req.SessionID,
		"remotePath": req.RemotePath,
		"editorId":   req.EditorID,
	}
}

func sanitizeAuditSaveResult(result *SaveResult) *auditSaveResultPayload {
	if result == nil {
		return nil
	}
	return &auditSaveResultPayload{
		Status:  result.Status,
		Message: result.Message,
		Session: sanitizeAuditSession(result.Session),
	}
}

func sanitizeAuditSession(session *Session) *auditSessionPayload {
	if session == nil {
		return nil
	}
	return &auditSessionPayload{
		ID:                    session.ID,
		AssetID:               session.AssetID,
		AssetName:             session.AssetName,
		SessionID:             session.SessionID,
		RemotePath:            session.RemotePath,
		RemoteRealPath:        session.RemoteRealPath,
		EditorID:              session.EditorID,
		EditorName:            session.EditorName,
		OriginalSize:          session.OriginalSize,
		OriginalModTime:       session.OriginalModTime,
		OriginalEncoding:      session.OriginalEncoding,
		OriginalBOM:           session.OriginalBOM,
		Dirty:                 session.Dirty,
		State:                 session.State,
		Expired:               session.Expired,
		SourceSessionID:       session.SourceSessionID,
		SupersededBySessionID: session.SupersededBySessionID,
		CreatedAt:             session.CreatedAt,
		UpdatedAt:             session.UpdatedAt,
		LastLaunchedAt:        session.LastLaunchedAt,
		LastSyncedAt:          session.LastSyncedAt,
	}
}

func sanitizeAuditMap(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	sanitized := make(map[string]any, len(payload))
	for key, value := range payload {
		if isAuditSensitiveField(key) {
			continue
		}
		sanitized[key] = sanitizeAuditPayload(value)
	}
	return sanitized
}

func isAuditSensitiveField(key string) bool {
	switch key {
	case "localPath", "workspaceRoot", "workspaceDir", "editorPath", "editorArgs", "originalSha256", "originalByteSample", "lastLocalSha256":
		return true
	default:
		return false
	}
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

func cloneSession(session *Session) *Session {
	if session == nil {
		return nil
	}
	cloned := *session
	cloned.EditorArgs = cloneArgs(session.EditorArgs)
	return &cloned
}

func cloneArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	cloned := make([]string, len(args))
	copy(cloned, args)
	return cloned
}

func cloneCustomEditors(editors []bootstrap.ExternalEditorConfig) []bootstrap.ExternalEditorConfig {
	if len(editors) == 0 {
		return nil
	}
	cloned := make([]bootstrap.ExternalEditorConfig, len(editors))
	for i, editor := range editors {
		cloned[i] = bootstrap.ExternalEditorConfig{
			ID:   editor.ID,
			Name: editor.Name,
			Path: editor.Path,
			Args: cloneArgs(editor.Args),
		}
	}
	return cloned
}

func trimArgs(args []string) []string {
	trimmed := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		trimmed = append(trimmed, arg)
	}
	return trimmed
}

func truncateText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}

func boolToSuccess(success bool) int {
	if success {
		return 1
	}
	return 0
}

func isWindows() bool {
	return runtime.GOOS == "windows"
}
