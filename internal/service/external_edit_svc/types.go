package external_edit_svc

import (
	"context"
	"time"

	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/repository/audit_repo"
	"github.com/opskat/opskat/internal/service/sftp_svc"
)

const (
	// v4 is the first external-edit manifest schema persisted by this PR.
	// Any post-release schema change must add an explicit migration path.
	manifestVersion = 4

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
	eventSessionAutoSave = "session_auto_save"
)

const (
	recordStateActive    = "active"
	recordStateConflict  = "conflict"
	recordStateError     = "error"
	recordStateCompleted = "completed"
	recordStateAbandoned = "abandoned"

	saveModeAutoLive      = "auto_live"
	saveModeManualRestore = "manual_restored"
)

const (
	autoSavePhasePending = "pending"
	autoSavePhaseRunning = "running"
	autoSavePhaseIdle    = "idle"
)

const (
	textEncodingUTF8    = "utf-8"
	textEncodingUTF16LE = "utf-16le"
	textEncodingUTF16BE = "utf-16be"
	textEncodingGB18030 = "gb18030"
)

const (
	reconcileSettleDelay              = 100 * time.Millisecond
	autoSaveDebounce                  = 500 * time.Millisecond
	defaultCleanupRetentionDays       = 7
	minCleanupRetentionDays           = 1
	maxCleanupRetentionDays           = 365
	defaultMaxReadFileSizeMB          = 10
	minMaxReadFileSizeMB              = 1
	maxMaxReadFileSizeMB              = 1024
	bytesPerMB                  int64 = 1024 * 1024
)

const externalEditReconnectHint = "请在同一资产中重新打开该远程文件后再继续同步"

var externalEditClipboardResidueMarkers = []string{
	"clipboard-images",
	"folder/clipboard",
	"folder\\clipboard",
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
	DefaultEditorID      string                           `json:"defaultEditorId"`
	WorkspaceRoot        string                           `json:"workspaceRoot"`
	CleanupRetentionDays int                              `json:"cleanupRetentionDays"`
	MaxReadFileSizeMB    int                              `json:"maxReadFileSizeMB"`
	Editors              []Editor                         `json:"editors"`
	CustomEditors        []bootstrap.ExternalEditorConfig `json:"customEditors"`
}

type SettingsInput struct {
	DefaultEditorID      string                           `json:"defaultEditorId"`
	WorkspaceRoot        string                           `json:"workspaceRoot"`
	CleanupRetentionDays int                              `json:"cleanupRetentionDays"`
	MaxReadFileSizeMB    int                              `json:"maxReadFileSizeMB"`
	CustomEditors        []bootstrap.ExternalEditorConfig `json:"customEditors"`
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

// ErrorSnapshot 只保留用户能理解、且不泄露 transport / 本地路径细节的失败摘要。
// 记录层会把最近一次失败沉淀到这里，前端再按文件态展示失败步骤和恢复建议。
type ErrorSnapshot struct {
	Step       string `json:"step"`
	Summary    string `json:"summary"`
	Suggestion string `json:"suggestion"`
	At         int64  `json:"at"`
}

// Session 是桌面端外部编辑的单一事实记录：
// 它同时串起远端基线、本地副本、编辑器选择、冲突状态和恢复信息，前后端都围绕这份记录推进状态。
type Session struct {
	ID             string   `json:"id"`
	AssetID        int64    `json:"assetId"`
	AssetName      string   `json:"assetName"`
	DocumentKey    string   `json:"documentKey"`
	SessionID      string   `json:"sessionId"`
	RemotePath     string   `json:"remotePath"`
	RemoteRealPath string   `json:"remoteRealPath"`
	LocalPath      string   `json:"localPath"`
	WorkspaceRoot  string   `json:"workspaceRoot"`
	WorkspaceDir   string   `json:"workspaceDir"`
	EditorID       string   `json:"editorId"`
	EditorName     string   `json:"editorName"`
	EditorPath     string   `json:"editorPath"`
	EditorArgs     []string `json:"editorArgs,omitempty"`
	// OriginalSHA256 保留旧字段名以兼容现有 manifest / IPC，语义上等同于当前 document 的 baseHash。
	OriginalSHA256     string `json:"originalSha256"`
	OriginalSize       int64  `json:"originalSize"`
	OriginalModTime    int64  `json:"originalModTime"`
	OriginalEncoding   string `json:"originalEncoding"`
	OriginalBOM        string `json:"originalBom,omitempty"`
	OriginalByteSample string `json:"originalByteSample,omitempty"`
	// LastLocalSHA256 同样保留兼容字段名，语义上等同于最近一次落盘的 localHash。
	LastLocalSHA256       string         `json:"lastLocalSha256"`
	Dirty                 bool           `json:"dirty"`
	State                 string         `json:"state"`
	RecordState           string         `json:"recordState,omitempty"`
	SaveMode              string         `json:"saveMode,omitempty"`
	PendingReview         bool           `json:"pendingReview,omitempty"`
	Hidden                bool           `json:"hidden"`
	Expired               bool           `json:"expired"`
	LastError             *ErrorSnapshot `json:"lastError,omitempty"`
	ResumeRequired        bool           `json:"resumeRequired,omitempty"`
	MergeRemoteSHA256     string         `json:"mergeRemoteSha256,omitempty"`
	SourceSessionID       string         `json:"sourceSessionId,omitempty"`
	SupersededBySessionID string         `json:"supersededBySessionId,omitempty"`
	CreatedAt             int64          `json:"createdAt"`
	UpdatedAt             int64          `json:"updatedAt"`
	LastLaunchedAt        int64          `json:"lastLaunchedAt"`
	LastSyncedAt          int64          `json:"lastSyncedAt"`
}

// Conflict 描述 document 级冲突关系：
// primaryDraftSessionId 永远指向用户正在保留的原始草稿；
// latestSnapshotSessionId 只在执行 reread 后出现，用来标记最新远端快照副本。
type Conflict struct {
	DocumentKey             string `json:"documentKey"`
	PrimaryDraftSessionID   string `json:"primaryDraftSessionId"`
	LatestSnapshotSessionID string `json:"latestSnapshotSessionId,omitempty"`
}

func sessionBaseHash(session *Session) string {
	if session == nil {
		return ""
	}
	return session.OriginalSHA256
}

func setSessionBaseHash(session *Session, hash string) {
	if session == nil {
		return
	}
	session.OriginalSHA256 = hash
}

func sessionLocalHash(session *Session) string {
	if session == nil {
		return ""
	}
	if session.LastLocalSHA256 != "" {
		return session.LastLocalSHA256
	}
	return sessionBaseHash(session)
}

func setSessionLocalHash(session *Session, hash string) {
	if session == nil {
		return
	}
	session.LastLocalSHA256 = hash
}

type SaveResult struct {
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Session   *Session  `json:"session,omitempty"`
	Conflict  *Conflict `json:"conflict,omitempty"`
	Automatic bool      `json:"automatic,omitempty"`
}

type DeleteResult struct {
	Status  string   `json:"status"`
	Message string   `json:"message,omitempty"`
	Session *Session `json:"session,omitempty"`
}

type CompareResult struct {
	DocumentKey             string    `json:"documentKey"`
	PrimaryDraftSessionID   string    `json:"primaryDraftSessionId"`
	LatestSnapshotSessionID string    `json:"latestSnapshotSessionId,omitempty"`
	FileName                string    `json:"fileName"`
	RemotePath              string    `json:"remotePath"`
	LocalContent            string    `json:"localContent"`
	RemoteContent           string    `json:"remoteContent"`
	ReadOnly                bool      `json:"readOnly"`
	Status                  string    `json:"status,omitempty"`
	Message                 string    `json:"message,omitempty"`
	Session                 *Session  `json:"session,omitempty"`
	Conflict                *Conflict `json:"conflict,omitempty"`
}

type MergePrepareResult struct {
	DocumentKey           string   `json:"documentKey"`
	PrimaryDraftSessionID string   `json:"primaryDraftSessionId"`
	FileName              string   `json:"fileName"`
	RemotePath            string   `json:"remotePath"`
	LocalContent          string   `json:"localContent"`
	RemoteContent         string   `json:"remoteContent"`
	FinalContent          string   `json:"finalContent"`
	RemoteHash            string   `json:"remoteHash"`
	Session               *Session `json:"session,omitempty"`
}

type MergeApplyRequest struct {
	SessionID    string `json:"sessionId"`
	FinalContent string `json:"finalContent"`
	RemoteHash   string `json:"remoteHash"`
}

// AutoSaveStatus 只描述运行期的自动保存瞬时阶段。
// 它通过 runtime event 给前端做反馈，不会落到 manifest / Session 持久状态中。
type AutoSaveStatus struct {
	DocumentKey string `json:"documentKey"`
	SessionID   string `json:"sessionId,omitempty"`
	Phase       string `json:"phase"`
}

type auditSessionPayload struct {
	ID                    string `json:"id,omitempty"`
	AssetID               int64  `json:"assetId,omitempty"`
	AssetName             string `json:"assetName,omitempty"`
	DocumentKey           string `json:"documentKey,omitempty"`
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
	RecordState           string `json:"recordState,omitempty"`
	SaveMode              string `json:"saveMode,omitempty"`
	PendingReview         bool   `json:"pendingReview,omitempty"`
	Hidden                bool   `json:"hidden"`
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
	Type       string          `json:"type"`
	Session    *Session        `json:"session,omitempty"`
	SaveResult *SaveResult     `json:"saveResult,omitempty"`
	AutoSave   *AutoSaveStatus `json:"autoSave,omitempty"`
}

type documentTransport struct {
	SessionID     string
	RemotePath    string
	CanonicalPath string
	Info          *sftp_svc.RemoteFileInfo
	Missing       bool
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
	FindSessions   func(assetID int64) []string
	Assets         AssetFinder
	Audit          audit_repo.AuditRepo
	Emit           func(Event)
	Launch         Launcher
	Now            func() time.Time
}
