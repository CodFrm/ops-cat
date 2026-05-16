package external_edit_svc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/audit_entity"
	"github.com/opskat/opskat/internal/repository/audit_repo"
	"github.com/opskat/opskat/internal/service/sftp_svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rebindRemoteFile struct {
	data     []byte
	realPath string
}

type rebindRemoteStub struct {
	mu          sync.Mutex
	files       map[string]map[string]rebindRemoteFile
	missing     map[string]map[string]error
	writeErrors map[string]map[string]error
	infoHashes  map[string]map[string]string
	writes      []string
}

func newRebindRemoteStub() *rebindRemoteStub {
	return &rebindRemoteStub{
		files:       make(map[string]map[string]rebindRemoteFile),
		missing:     make(map[string]map[string]error),
		writeErrors: make(map[string]map[string]error),
		infoHashes:  make(map[string]map[string]string),
	}
}

func (r *rebindRemoteStub) SetFile(sessionID, remotePath string, data []byte, realPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.files[sessionID] == nil {
		r.files[sessionID] = make(map[string]rebindRemoteFile)
	}
	r.files[sessionID][remotePath] = rebindRemoteFile{
		data:     append([]byte(nil), data...),
		realPath: realPath,
	}
	if r.missing[sessionID] != nil {
		delete(r.missing[sessionID], remotePath)
	}
}

func (r *rebindRemoteStub) SetError(sessionID, remotePath string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.missing[sessionID] == nil {
		r.missing[sessionID] = make(map[string]error)
	}
	r.missing[sessionID][remotePath] = err
}

func (r *rebindRemoteStub) SetWriteError(sessionID, remotePath string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.writeErrors[sessionID] == nil {
		r.writeErrors[sessionID] = make(map[string]error)
	}
	r.writeErrors[sessionID][remotePath] = err
}

func (r *rebindRemoteStub) SetInfoHash(sessionID, remotePath string, hash string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.infoHashes[sessionID] == nil {
		r.infoHashes[sessionID] = make(map[string]string)
	}
	r.infoHashes[sessionID][remotePath] = hash
}

func (r *rebindRemoteStub) ClearError(sessionID, remotePath string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if byPath := r.missing[sessionID]; byPath != nil {
		delete(byPath, remotePath)
	}
}

func (r *rebindRemoteStub) Stat(sessionID, remotePath string) (*sftp_svc.RemoteFileInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.lookupErrorLocked(sessionID, remotePath); err != nil {
		return nil, err
	}
	file, ok := r.lookupFileLocked(sessionID, remotePath)
	if !ok {
		return nil, os.ErrNotExist
	}
	sha := hashBytes(file.data)
	if byPath := r.infoHashes[sessionID]; byPath != nil {
		if override := strings.TrimSpace(byPath[remotePath]); override != "" {
			sha = override
		}
	}
	return &sftp_svc.RemoteFileInfo{
		Path:     remotePath,
		Size:     int64(len(file.data)),
		Mode:     uint32(0o600),
		ModTime:  1700000000,
		Regular:  true,
		RealPath: file.realPath,
		SHA256:   sha,
	}, nil
}

func (r *rebindRemoteStub) ReadFile(sessionID, remotePath string) ([]byte, *sftp_svc.RemoteFileInfo, error) {
	info, err := r.Stat(sessionID, remotePath)
	if err != nil {
		return nil, nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	file, _ := r.lookupFileLocked(sessionID, remotePath)
	return append([]byte(nil), file.data...), info, nil
}

func (r *rebindRemoteStub) WriteFile(sessionID, remotePath string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if byPath := r.writeErrors[sessionID]; byPath != nil {
		if err, ok := byPath[remotePath]; ok {
			return err
		}
	}
	if err := r.lookupErrorLocked(sessionID, remotePath); err != nil {
		return err
	}
	if r.files[sessionID] == nil {
		r.files[sessionID] = make(map[string]rebindRemoteFile)
	}
	existing := r.files[sessionID][remotePath]
	if strings.TrimSpace(existing.realPath) == "" {
		existing.realPath = remotePath
	}
	existing.data = append([]byte(nil), data...)
	r.files[sessionID][remotePath] = existing
	r.writes = append(r.writes, fmt.Sprintf("%s:%s", sessionID, remotePath))
	return nil
}

func (r *rebindRemoteStub) lookupErrorLocked(sessionID, remotePath string) error {
	if byPath := r.missing[sessionID]; byPath != nil {
		if err, ok := byPath[remotePath]; ok {
			return err
		}
	}
	return nil
}

func (r *rebindRemoteStub) lookupFileLocked(sessionID, remotePath string) (rebindRemoteFile, bool) {
	if byPath := r.files[sessionID]; byPath != nil {
		file, ok := byPath[remotePath]
		return file, ok
	}
	return rebindRemoteFile{}, false
}

type rebindAssetFinder struct{}

func (rebindAssetFinder) Find(context.Context, int64) (*asset_entity.Asset, error) {
	return &asset_entity.Asset{Name: "asset-101"}, nil
}

type rebindAuditRepo struct {
	mu   sync.Mutex
	logs []*audit_entity.AuditLog
}

func (r *rebindAuditRepo) Create(_ context.Context, log *audit_entity.AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cloned := *log
	r.logs = append(r.logs, &cloned)
	return nil
}

func (r *rebindAuditRepo) List(context.Context, audit_repo.ListOptions) ([]*audit_entity.AuditLog, int64, error) {
	return nil, 0, nil
}

func (r *rebindAuditRepo) ListSessions(context.Context, int64) ([]audit_repo.SessionInfo, error) {
	return nil, nil
}

func (r *rebindAuditRepo) lastTool() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.logs) == 0 {
		return ""
	}
	return r.logs[len(r.logs)-1].ToolName
}

func (r *rebindAuditRepo) lastLog() *audit_entity.AuditLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.logs) == 0 {
		return nil
	}
	cloned := *r.logs[len(r.logs)-1]
	return &cloned
}

type rebindHarness struct {
	svc      *Service
	remote   *rebindRemoteStub
	audit    *rebindAuditRepo
	cfg      *bootstrap.AppConfig
	manifest string
	now      time.Time
	events   []Event
	eventsMu sync.Mutex
}

func newRebindHarness(t *testing.T, finder func(int64) []string) *rebindHarness {
	t.Helper()

	dataDir := t.TempDir()
	h := &rebindHarness{}
	cfg := &bootstrap.AppConfig{
		ExternalEditDefaultEditorID: "system-text",
		ExternalEditWorkspaceRoot:   dataDir,
	}
	remote := newRebindRemoteStub()
	audit := &rebindAuditRepo{}
	currentTime := time.Unix(1700000000, 0)
	svc, err := NewService(Options{
		DataDir:        dataDir,
		ConfigProvider: func() *bootstrap.AppConfig { return cfg },
		ConfigSaver: func(next *bootstrap.AppConfig) error {
			*cfg = *next
			return nil
		},
		Remote:       remote,
		FindSessions: finder,
		Assets:       rebindAssetFinder{},
		Audit:        audit,
		Emit: func(event Event) {
			h.eventsMu.Lock()
			defer h.eventsMu.Unlock()
			h.events = append(h.events, event)
		},
		Launch: launcherFunc(func(string, []string) error { return nil }),
		Now: func() time.Time {
			return currentTime
		},
	})
	require.NoError(t, err)
	require.NoError(t, svc.Start(context.Background()))
	t.Cleanup(func() {
		_ = svc.Close()
	})

	h.svc = svc
	h.remote = remote
	h.audit = audit
	h.cfg = cfg
	h.manifest = dataDir
	h.now = currentTime
	return h
}

func (h *rebindHarness) snapshotEvents() []Event {
	h.eventsMu.Lock()
	defer h.eventsMu.Unlock()
	cloned := make([]Event, len(h.events))
	copy(cloned, h.events)
	return cloned
}

func (h *rebindHarness) openSession(t *testing.T, sessionID, remotePath, realPath string, data []byte) *Session {
	t.Helper()
	h.remote.SetFile(sessionID, remotePath, data, realPath)
	session, err := h.svc.Open(context.Background(), OpenRequest{
		AssetID:    101,
		SessionID:  sessionID,
		RemotePath: remotePath,
		EditorID:   "system-text",
	})
	require.NoError(t, err)
	return session
}

func (h *rebindHarness) refreshSession(t *testing.T, sessionID string) *Session {
	t.Helper()
	session := h.svc.getSession(sessionID)
	require.NotNil(t, session)
	return session
}

func markDirtyLocalCopy(t *testing.T, session *Session, data []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(session.LocalPath, data, 0o600))
}

func readBakeupFiles(t *testing.T, workspaceDir string) [][]byte {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(workspaceDir, "bakeup"))
	require.NoError(t, err)
	files := make([][]byte, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(workspaceDir, "bakeup", entry.Name())
		data, readErr := os.ReadFile(path) //nolint:gosec // path comes from the test workspace bakeup directory
		require.NoError(t, readErr)
		files = append(files, data)
	}
	return files
}

func bakeupEntryPaths(t *testing.T, workspaceDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(workspaceDir, "bakeup"))
	require.NoError(t, err)
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(workspaceDir, "bakeup", entry.Name()))
	}
	return paths
}

func TestExternalEditOpenRejectsOversizedRemoteFile(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-a"} })
	oversized := []byte(strings.Repeat("a", int(sftp_svc.MaxReadFileSize)+1))
	h.remote.SetFile("ssh-a", "/srv/app/big.log", oversized, "/srv/app/big.log")

	_, err := h.svc.Open(context.Background(), OpenRequest{
		AssetID:    101,
		SessionID:  "ssh-a",
		RemotePath: "/srv/app/big.log",
		EditorID:   "system-text",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "远程文件过大")
}

func TestExternalEditSettingsExposeDefaultMaxReadFileSizeMB(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-a"} })

	settings, err := h.svc.GetSettings()
	require.NoError(t, err)
	require.NotNil(t, settings)
	assert.Equal(t, 10, settings.MaxReadFileSizeMB)
}

func TestExternalEditSaveSettingsNormalizesMaxReadFileSizeMB(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-a"} })

	settings, err := h.svc.SaveSettings(SettingsInput{
		DefaultEditorID:      "system-text",
		WorkspaceRoot:        h.manifest,
		CleanupRetentionDays: 7,
		MaxReadFileSizeMB:    0,
	})
	require.NoError(t, err)
	require.NotNil(t, settings)
	assert.Equal(t, 10, settings.MaxReadFileSizeMB)
	assert.Equal(t, 10, h.cfg.ExternalEditMaxReadFileSizeMB)
}

func TestExternalEditOpenRejectsRemoteFileOverConfiguredLimit(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-a"} })
	h.cfg.ExternalEditMaxReadFileSizeMB = 1
	oversized := []byte(strings.Repeat("a", 2*1024*1024))
	h.remote.SetFile("ssh-a", "/srv/app/big.log", oversized, "/srv/app/big.log")

	_, err := h.svc.Open(context.Background(), OpenRequest{
		AssetID:    101,
		SessionID:  "ssh-a",
		RemotePath: "/srv/app/big.log",
		EditorID:   "system-text",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "远程文件过大")
}

func TestExternalEditSaveRejectsOversizedLocalCopy(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-a"} })
	session := h.openSession(t, "ssh-a", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	markDirtyLocalCopy(t, session, []byte(strings.Repeat("a", int(sftp_svc.MaxReadFileSize)+1)))

	_, err := h.svc.Save(context.Background(), session.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "本地副本过大")
	require.Empty(t, h.remote.writes)
}

func TestExternalEditSaveRejectsLocalCopyOverConfiguredLimit(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-a"} })
	h.cfg.ExternalEditMaxReadFileSizeMB = 1
	session := h.openSession(t, "ssh-a", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	markDirtyLocalCopy(t, session, []byte(strings.Repeat("a", 2*1024*1024)))

	_, err := h.svc.Save(context.Background(), session.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "本地副本过大")
	require.Empty(t, h.remote.writes)
}

func TestExternalEditSaveRebindsToUniqueCandidateAndPersistsSessionID(t *testing.T) {
	h := newRebindHarness(t, func(assetID int64) []string {
		if assetID != 101 {
			return nil
		}
		return []string{"ssh-new"}
	})
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.remote.SetError("ssh-old", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-old"))
	h.remote.SetFile("ssh-new", "/srv/app/demo.txt", []byte("hello\n"), "/srv/app/demo.txt")
	markDirtyLocalCopy(t, session, []byte("hello saved\n"))

	result, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, result.Status)
	require.Equal(t, "ssh-new", result.Session.SessionID)

	manifest, err := os.ReadFile(filepath.Join(h.manifest, "storage", "manifest.json"))
	require.NoError(t, err)
	assert.Contains(t, string(manifest), "\"sessionId\": \"ssh-new\"")
	assert.Equal(t, "external_edit_save", h.audit.lastTool())
}

func TestExternalEditSaveRebindsWhenSessionMissingErrorHasNoSpace(t *testing.T) {
	h := newRebindHarness(t, func(assetID int64) []string {
		if assetID != 101 {
			return nil
		}
		return []string{"ssh-new"}
	})
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.remote.SetError("ssh-old", "/srv/app/demo.txt", errors.New("SSH会话不存在:ssh-old"))
	h.remote.SetFile("ssh-new", "/srv/app/demo.txt", []byte("hello\n"), "/srv/app/demo.txt")
	markDirtyLocalCopy(t, session, []byte("hello saved\n"))

	result, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, result.Status)
	require.Equal(t, "ssh-new", result.Session.SessionID)
	assert.NotContains(t, result.Message, "SSH会话不存在")

	manifest, err := os.ReadFile(filepath.Join(h.manifest, "storage", "manifest.json"))
	require.NoError(t, err)
	assert.Contains(t, string(manifest), "\"sessionId\": \"ssh-new\"")
	assert.Equal(t, "external_edit_save", h.audit.lastTool())
}

func TestExternalEditSaveBlocksWhenNoCandidate(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return nil })
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.remote.SetError("ssh-old", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-old"))
	markDirtyLocalCopy(t, session, []byte("hello dirty\n"))

	_, err := h.svc.Save(context.Background(), session.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "当前文件位置已变化")
	assert.Contains(t, err.Error(), externalEditReconnectHint)
	assert.Equal(t, "external_edit_document_transport_blocked", h.audit.lastTool())
}

func TestExternalEditSaveUsesAnyMatchingCandidateTransport(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-a", "ssh-b"} })
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.remote.SetError("ssh-old", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-old"))
	h.remote.SetFile("ssh-a", "/srv/app/demo.txt", []byte("hello\n"), "/srv/app/demo.txt")
	h.remote.SetFile("ssh-b", "/srv/app/demo.txt", []byte("hello\n"), "/srv/app/demo.txt")
	markDirtyLocalCopy(t, session, []byte("hello dirty\n"))

	result, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, result.Status)
	require.Equal(t, "ssh-a", result.Session.SessionID)
}

func TestExternalEditSaveBlocksWhenRemoteRealPathDiffers(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-new"} })
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.remote.SetError("ssh-old", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-old"))
	h.remote.SetFile("ssh-new", "/srv/app/demo.txt", []byte("hello\n"), "/srv/other/demo.txt")
	markDirtyLocalCopy(t, session, []byte("hello dirty\n"))

	_, err := h.svc.Save(context.Background(), session.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "无法确认仍是同一份远程文件")
}

func TestExternalEditSaveStillEntersConflictAfterRebind(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-new"} })
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.remote.SetError("ssh-old", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-old"))
	h.remote.SetFile("ssh-new", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	markDirtyLocalCopy(t, session, []byte("local dirty\n"))

	result, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, result.Status)
	require.Equal(t, "ssh-new", result.Session.SessionID)
	assert.Equal(t, "external_edit_conflict_remote_changed", h.audit.lastTool())
}

func TestExternalEditSaveDetectsRemoteChangeFromReadBytesWhenInfoHashIsStale(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	base := []byte("CASE68-C-BASE\n")
	session := h.openSession(t, "ssh-b", "/srv/app/case-c.txt", "/srv/app/case-c.txt", base)
	markDirtyLocalCopy(t, session, []byte("CASE68-C-LOCAL-EDIT-1\n"))

	h.remote.SetFile("ssh-b", "/srv/app/case-c.txt", []byte("CASE68-C-REMOTE-EDIT-1\n"), "/srv/app/case-c.txt")
	h.remote.SetInfoHash("ssh-b", "/srv/app/case-c.txt", sessionBaseHash(session))

	result, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, result.Status)
	require.NotNil(t, result.Conflict)
	require.Equal(t, session.ID, result.Conflict.PrimaryDraftSessionID)
	require.Equal(t, sessionStateConflict, result.Session.State)
	require.Equal(t, recordStateConflict, result.Session.RecordState)
	require.False(t, result.Session.Hidden)
	require.True(t, result.Session.Dirty)
	require.Empty(t, h.remote.writes)

	stored := h.refreshSession(t, session.ID)
	require.Equal(t, sessionStateConflict, stored.State)
	require.Equal(t, recordStateConflict, stored.RecordState)
	require.True(t, stored.Dirty)
	require.False(t, stored.Hidden)
}

func TestExternalEditSaveStillSupportsRecreateAfterRebind(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-new"} })
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.remote.SetError("ssh-old", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-old"))
	h.remote.SetError("ssh-new", "/srv/app/demo.txt", os.ErrNotExist)
	markDirtyLocalCopy(t, session, []byte("local dirty\n"))

	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusRemoteMissing, conflict.Status)
	require.Equal(t, "ssh-new", conflict.Session.SessionID)

	h.remote.ClearError("ssh-new", "/srv/app/demo.txt")
	recreated, err := h.svc.Resolve(context.Background(), session.ID, resolutionRecreate)
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, recreated.Status)
	require.Equal(t, "external_edit_recreate", h.audit.lastTool())
}

func TestExternalEditSaveWriteRemoteMissingStaysRecoverable(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("local dirty\n"))
	h.remote.SetWriteError("ssh-b", "/srv/app/demo.txt", os.ErrNotExist)

	result, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusRemoteMissing, result.Status)
	require.Equal(t, sessionStateRemoteMissing, result.Session.State)
	require.NotNil(t, result.Conflict)
	require.Equal(t, "external_edit_conflict_remote_missing", h.audit.lastTool())
}

func TestExternalEditOverwriteWriteRemoteMissingStaysRecoverable(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("local overwrite\n"))
	h.remote.SetFile("ssh-b", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	h.remote.SetWriteError("ssh-b", "/srv/app/demo.txt", os.ErrNotExist)
	result, err := h.svc.Resolve(context.Background(), session.ID, resolutionOverwrite)
	require.NoError(t, err)
	require.Equal(t, saveStatusRemoteMissing, result.Status)
	require.Equal(t, sessionStateRemoteMissing, result.Session.State)
	require.NotNil(t, result.Conflict)
	require.Equal(t, "external_edit_conflict_remote_missing", h.audit.lastTool())
}

func TestExternalEditOpenReuseKeepsBaseAndLocalHash(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	markDirtyLocalCopy(t, session, []byte("hello again\n"))
	h.svc.reconcileLocalCopy(session.ID)

	reopened, err := h.svc.Open(context.Background(), OpenRequest{
		AssetID:    session.AssetID,
		SessionID:  "ssh-b",
		RemotePath: session.RemotePath,
	})
	require.NoError(t, err)
	require.Equal(t, session.ID, reopened.ID)
	require.Equal(t, hashBytes([]byte("hello\n")), sessionBaseHash(reopened))
	require.Equal(t, hashBytes([]byte("hello again\n")), sessionLocalHash(reopened))
}

func TestExternalEditSaveAdvancesBaseHashAfterSuccessfulUpload(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	localData := []byte("hello saved\n")
	markDirtyLocalCopy(t, session, localData)

	result, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, result.Status)
	require.NotNil(t, result.Session)
	require.Equal(t, hashBytes(localData), sessionBaseHash(result.Session))
	require.Equal(t, hashBytes(localData), sessionLocalHash(result.Session))
}

func TestExternalEditResolveOverwriteRebindsBeforeContinuing(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-new"} })
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.remote.SetError("ssh-old", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-old"))
	h.remote.SetFile("ssh-new", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	markDirtyLocalCopy(t, session, []byte("local dirty\n"))

	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	overwrite, err := h.svc.Resolve(context.Background(), session.ID, resolutionOverwrite)
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, overwrite.Status)
	require.Equal(t, "ssh-new", overwrite.Session.SessionID)
	assert.Equal(t, "external_edit_overwrite", h.audit.lastTool())
}

func TestExternalEditResolveRereadRebindsBeforeContinuing(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-new"} })
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.remote.SetError("ssh-old", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-old"))
	h.remote.SetFile("ssh-new", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	markDirtyLocalCopy(t, session, []byte("local dirty\n"))

	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	h.remote.SetFile("ssh-new", "/srv/app/demo.txt", []byte("remote newer\n"), "/srv/app/demo.txt")
	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)
	require.Equal(t, "ssh-new", reread.Session.SessionID)
}

func TestExternalEditSaveBlocksStaleSession(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-new"} })
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.svc.mu.Lock()
	h.svc.sessions[session.ID].State = sessionStateStale
	h.svc.mu.Unlock()
	markDirtyLocalCopy(t, session, []byte("local dirty\n"))

	_, err := h.svc.Save(context.Background(), session.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已被新的远程版本替代")
}

func TestExternalEditSaveBlocksExpiredSessionWithReconnectHint(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-new"} })
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.svc.mu.Lock()
	h.svc.sessions[session.ID].State = sessionStateExpired
	h.svc.mu.Unlock()
	markDirtyLocalCopy(t, session, []byte("local dirty\n"))

	_, err := h.svc.Save(context.Background(), session.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "当前副本已过期")
	assert.Contains(t, err.Error(), externalEditReconnectHint)
}

func TestExternalEditSaveDoesNotMisclassifyConnectionFailureAsRemoteMissing(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-new"} })
	session := h.openSession(t, "ssh-old", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.remote.SetError("ssh-old", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-old"))
	h.remote.SetError("ssh-new", "/srv/app/demo.txt", errors.New("dial tcp timeout"))
	markDirtyLocalCopy(t, session, []byte("local dirty\n"))

	_, err := h.svc.Save(context.Background(), session.ID)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "远程文件不存在")

	log := h.audit.lastLog()
	require.NotNil(t, log)
	assert.Equal(t, "desktop", log.Source)
	assert.Equal(t, "external_edit_document_transport_blocked", log.ToolName)
}

func TestIsSSHSessionMissingErrorVariants(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "cn_with_space", err: errors.New("SSH 会话不存在: ssh-old"), want: true},
		{name: "cn_without_space", err: errors.New("SSH会话不存在:ssh-old"), want: true},
		{name: "en", err: errors.New("SSH session does not exist: ssh-old"), want: true},
		{name: "other", err: errors.New("dial tcp timeout"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isSSHSessionMissingError(tc.err))
		})
	}
}

func TestExternalEditDocumentSaveSucceedsAfterOriginalTransportClosed(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	markDirtyLocalCopy(t, session, []byte("hello from b\n"))
	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("hello\n"), "/srv/app/demo.txt")

	result, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, result.Status)
	require.Equal(t, "ssh-c", result.Session.SessionID)
	assert.Equal(t, "ssh-c:/srv/app/demo.txt", h.remote.writes[len(h.remote.writes)-1])
}

func TestExternalEditDocumentOpenFromAnotherTransportReusesDirtyCopy(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	sessionB := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, sessionB, []byte("hello from b\n"))
	h.svc.reconcileLocalCopy(sessionB.ID)

	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("hello\n"), "/srv/app/demo.txt")
	sessionC := h.openSession(t, "ssh-c", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	require.Equal(t, sessionB.ID, sessionC.ID)
	require.Equal(t, sessionB.DocumentKey, sessionC.DocumentKey)
	require.True(t, sessionC.Dirty)
	require.Equal(t, "ssh-c", sessionC.SessionID)
}

func TestExternalEditDocumentRefreshShowsRemoteMissingAfterDelete(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("hello from b\n"))

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetError("ssh-c", "/srv/app/demo.txt", os.ErrNotExist)
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusRemoteMissing, conflict.Status)
	require.Equal(t, "ssh-c", conflict.Session.SessionID)

	refreshed := h.refreshSession(t, session.ID)
	require.Equal(t, sessionStateRemoteMissing, refreshed.State)
	require.True(t, refreshed.Dirty)
}

func TestExternalEditDocumentStillConflictsWhenRemoteChangedOnAnotherTransport(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("hello from b\n"))

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	result, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, result.Status)
	require.Equal(t, "ssh-c", result.Session.SessionID)
}

func TestExternalEditDocumentBlocksWhenCanonicalFileCannotBeConfirmed(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("hello from b\n"))

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("hello\n"), "/srv/other/demo.txt")

	_, err := h.svc.Save(context.Background(), session.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "无法确认仍是同一份远程文件")
	assert.Empty(t, h.remote.writes)
}

func TestExternalEditDocumentRereadUsesAnotherTransportAfterConflict(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("hello from b\n"))

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote newer\n"), "/srv/app/demo.txt")
	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)
	require.Equal(t, "ssh-c", reread.Session.SessionID)
	require.Equal(t, session.DocumentKey, reread.Session.DocumentKey)
	require.Equal(t, saveModeAutoLive, reread.Session.SaveMode)
}

func TestExternalEditRereadNewDraftAutoSavesAfterFurtherEdit(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("hello from b\n"))

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote newer\n"), "/srv/app/demo.txt")
	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)

	markDirtyLocalCopy(t, reread.Session, []byte("remote newer\nlocal follow-up\n"))
	h.svc.reconcileLocalCopy(reread.Session.ID)
	require.Eventually(t, func() bool {
		return len(h.remote.writes) > 0
	}, autoSaveDebounce+time.Second, 50*time.Millisecond)

	lastWrite := h.remote.writes[len(h.remote.writes)-1]
	assert.Equal(t, "ssh-c:/srv/app/demo.txt", lastWrite)

	stored := h.refreshSession(t, reread.Session.ID)
	require.Equal(t, recordStateActive, stored.RecordState)
	require.False(t, stored.Hidden)
	require.Equal(t, sessionStateClean, stored.State)
	require.Equal(t, hashBytes([]byte("remote newer\nlocal follow-up\n")), sessionBaseHash(stored))
	require.Equal(t, hashBytes([]byte("remote newer\nlocal follow-up\n")), sessionLocalHash(stored))
}

func TestExternalEditRereadNewDraftReentersConflictAfterFurtherEdit(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("hello from b\n"))

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote newer\n"), "/srv/app/demo.txt")
	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)

	markDirtyLocalCopy(t, reread.Session, []byte("remote newer\nlocal follow-up\n"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote changed again\n"), "/srv/app/demo.txt")

	nextConflict, err := h.svc.Save(context.Background(), reread.Session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, nextConflict.Status)
	require.NotNil(t, nextConflict.Conflict)
	require.Equal(t, reread.Session.ID, nextConflict.Conflict.PrimaryDraftSessionID)
}

func TestExternalEditRereadNewDraftDetectsRemoteChangeWithoutLocalDirty(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/ee68_c_conflict.txt", "/srv/app/ee68_c_conflict.txt", []byte("CASE68-C-BASE\n"))
	markDirtyLocalCopy(t, session, []byte("CASE68-C-LOCAL-EDIT-1\n"))

	h.remote.SetError("ssh-b", "/srv/app/ee68_c_conflict.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/ee68_c_conflict.txt", []byte("CASE68-C-REMOTE-EDIT-1\n"), "/srv/app/ee68_c_conflict.txt")
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	h.remote.SetInfoHash("ssh-c", "/srv/app/ee68_c_conflict.txt", sessionBaseHash(session))
	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)
	require.False(t, reread.Session.Dirty)
	require.Equal(t, sessionStateClean, reread.Session.State)
	require.Equal(t, hashBytes([]byte("CASE68-C-REMOTE-EDIT-1\n")), sessionBaseHash(reread.Session))

	h.remote.SetFile("ssh-c", "/srv/app/ee68_c_conflict.txt", []byte("CASE68-C-REMOTE-EDIT-2\n"), "/srv/app/ee68_c_conflict.txt")
	nextConflict, err := h.svc.Save(context.Background(), reread.Session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, nextConflict.Status)
	require.NotNil(t, nextConflict.Conflict)
	require.Equal(t, reread.Session.ID, nextConflict.Conflict.PrimaryDraftSessionID)
	require.Equal(t, sessionStateConflict, nextConflict.Session.State)
	require.Equal(t, recordStateConflict, nextConflict.Session.RecordState)
	require.False(t, nextConflict.Session.Hidden)
	require.True(t, nextConflict.Session.Dirty)
	require.Empty(t, h.remote.writes)

	stored := h.refreshSession(t, reread.Session.ID)
	require.Equal(t, sessionStateConflict, stored.State)
	require.Equal(t, recordStateConflict, stored.RecordState)
	require.False(t, stored.Hidden)
}

func TestExternalEditRefreshRereadNewDraftDetectsRemoteChangeWithoutLocalDirty(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/ee68_c_conflict.txt", "/srv/app/ee68_c_conflict.txt", []byte("CASE68-C-BASE\n"))
	markDirtyLocalCopy(t, session, []byte("CASE68-C-LOCAL-EDIT-1\n"))

	h.remote.SetError("ssh-b", "/srv/app/ee68_c_conflict.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/ee68_c_conflict.txt", []byte("CASE68-C-REMOTE-EDIT-1\n"), "/srv/app/ee68_c_conflict.txt")
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)
	require.False(t, reread.Session.Dirty)
	require.Equal(t, sessionStateClean, reread.Session.State)

	h.remote.SetFile("ssh-c", "/srv/app/ee68_c_conflict.txt", []byte("X"), "/srv/app/ee68_c_conflict.txt")
	h.remote.SetInfoHash("ssh-c", "/srv/app/ee68_c_conflict.txt", sessionBaseHash(reread.Session))
	refreshed, err := h.svc.Refresh(reread.Session.ID)
	require.NoError(t, err)
	require.Equal(t, sessionStateConflict, refreshed.State)
	require.Equal(t, recordStateConflict, refreshed.RecordState)
	require.False(t, refreshed.Hidden)
	require.True(t, refreshed.Dirty)
	require.Equal(t, hashBytes([]byte("CASE68-C-REMOTE-EDIT-1\n")), sessionLocalHash(refreshed))
	require.Empty(t, h.remote.writes)

	stored := h.refreshSession(t, reread.Session.ID)
	require.Equal(t, sessionStateConflict, stored.State)
	require.Equal(t, recordStateConflict, stored.RecordState)
	require.False(t, stored.Hidden)
	require.True(t, stored.Dirty)

	events := h.snapshotEvents()
	require.NotEmpty(t, events)
	last := events[len(events)-1]
	require.Equal(t, eventSessionConflict, last.Type)
	require.NotNil(t, last.SaveResult)
	require.Equal(t, saveStatusConflict, last.SaveResult.Status)
	require.Equal(t, refreshed.ID, last.SaveResult.Conflict.PrimaryDraftSessionID)
}

func TestExternalEditRefreshConflictSurvivesLocalReconcile(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/ee68_c_conflict.txt", "/srv/app/ee68_c_conflict.txt", []byte("CASE68-C-BASE\n"))
	markDirtyLocalCopy(t, session, []byte("CASE68-C-LOCAL-EDIT-1\n"))

	h.remote.SetError("ssh-b", "/srv/app/ee68_c_conflict.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/ee68_c_conflict.txt", []byte("CASE68-C-REMOTE-EDIT-1\n"), "/srv/app/ee68_c_conflict.txt")
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)
	require.False(t, reread.Session.Dirty)
	require.Equal(t, sessionStateClean, reread.Session.State)

	h.remote.SetFile("ssh-c", "/srv/app/ee68_c_conflict.txt", []byte("X"), "/srv/app/ee68_c_conflict.txt")
	refreshed, err := h.svc.Refresh(reread.Session.ID)
	require.NoError(t, err)
	require.Equal(t, sessionStateConflict, refreshed.State)
	require.Equal(t, recordStateConflict, refreshed.RecordState)
	require.True(t, refreshed.Dirty)

	h.svc.reconcileLocalCopy(reread.Session.ID)
	stored := h.refreshSession(t, reread.Session.ID)
	require.Equal(t, sessionStateConflict, stored.State)
	require.Equal(t, recordStateConflict, stored.RecordState)
	require.True(t, stored.Dirty)
	require.False(t, stored.Hidden)
	require.Empty(t, h.remote.writes)
}

func TestExternalEditReconnectRefreshRereadNewDraftDetectsRemoteChangeWithoutLocalDirty(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/ee68_c_conflict.txt", "/srv/app/ee68_c_conflict.txt", []byte("CASE68-C-BASE\n"))
	markDirtyLocalCopy(t, session, []byte("CASE68-C-LOCAL-EDIT-1\n"))

	h.remote.SetError("ssh-b", "/srv/app/ee68_c_conflict.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/ee68_c_conflict.txt", []byte("CASE68-C-REMOTE-EDIT-1\n"), "/srv/app/ee68_c_conflict.txt")
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	h.remote.SetInfoHash("ssh-c", "/srv/app/ee68_c_conflict.txt", sessionBaseHash(session))
	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)
	require.Equal(t, hashBytes([]byte("CASE68-C-REMOTE-EDIT-1\n")), sessionBaseHash(reread.Session))
	require.NoError(t, h.svc.Close())

	cfg := &bootstrap.AppConfig{
		ExternalEditDefaultEditorID: "system-text",
		ExternalEditWorkspaceRoot:   h.manifest,
	}
	reopened, err := NewService(Options{
		DataDir:        h.manifest,
		ConfigProvider: func() *bootstrap.AppConfig { return cfg },
		ConfigSaver:    func(next *bootstrap.AppConfig) error { *cfg = *next; return nil },
		Remote:         h.remote,
		FindSessions:   func(int64) []string { return []string{"ssh-b", "ssh-c"} },
		Assets:         rebindAssetFinder{},
		Audit:          h.audit,
		Emit:           func(Event) {},
		Launch:         launcherFunc(func(string, []string) error { return nil }),
		Now:            h.svc.now,
	})
	require.NoError(t, err)
	require.NoError(t, reopened.Start(context.Background()))
	defer func() { _ = reopened.Close() }()

	restored := reopened.getSession(reread.Session.ID)
	require.NotNil(t, restored)
	require.Equal(t, sessionStateClean, restored.State)
	require.Equal(t, hashBytes([]byte("CASE68-C-REMOTE-EDIT-1\n")), sessionBaseHash(restored))

	h.remote.SetFile("ssh-c", "/srv/app/ee68_c_conflict.txt", []byte("X"), "/srv/app/ee68_c_conflict.txt")
	h.remote.SetInfoHash("ssh-c", "/srv/app/ee68_c_conflict.txt", sessionBaseHash(restored))
	refreshed, err := reopened.Refresh(reread.Session.ID)
	require.NoError(t, err)
	require.Equal(t, sessionStateConflict, refreshed.State)
	require.Equal(t, recordStateConflict, refreshed.RecordState)
	require.False(t, refreshed.Hidden)
	require.True(t, refreshed.Dirty)
	require.Empty(t, h.remote.writes)
}

func TestExternalEditAutoSaveOnlyAttemptsOneStableHashOnce(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	markDirtyLocalCopy(t, session, []byte("hello autosave\n"))
	h.svc.reconcileLocalCopy(session.ID)

	require.Eventually(t, func() bool {
		return len(h.remote.writes) == 1
	}, 3*time.Second, 50*time.Millisecond)

	h.svc.reconcileLocalCopy(session.ID)
	time.Sleep(autoSaveDebounce + 200*time.Millisecond)
	require.Len(t, h.remote.writes, 1)

	saved := h.refreshSession(t, session.ID)
	require.Equal(t, sessionStateClean, saved.State)
	require.False(t, saved.Dirty)
}

func TestExternalEditCompareReturnsReadOnlyDiffWithoutWritingRemote(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("local draft\n"))

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")

	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	diff, err := h.svc.Compare(session.ID)
	require.NoError(t, err)
	require.True(t, diff.ReadOnly)
	require.Equal(t, "local draft\n", diff.LocalContent)
	require.Equal(t, "remote changed\n", diff.RemoteContent)
	require.Empty(t, h.remote.writes)
	assert.Equal(t, "external_edit_compare", h.audit.lastTool())
}

func TestExternalEditCompareRemoteMissingKeepsRecoverableState(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("local draft\n"))

	h.remote.SetFile("ssh-b", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", os.ErrNotExist)
	diff, err := h.svc.Compare(session.ID)
	require.NoError(t, err)
	require.NotNil(t, diff)
	require.Equal(t, saveStatusRemoteMissing, diff.Status)
	require.NotNil(t, diff.Session)
	require.NotNil(t, diff.Conflict)

	current := h.refreshSession(t, session.ID)
	require.Equal(t, sessionStateRemoteMissing, current.State)
	require.Equal(t, recordStateConflict, current.RecordState)

	events := h.snapshotEvents()
	require.NotEmpty(t, events)
	last := events[len(events)-1]
	require.Equal(t, eventSessionConflict, last.Type)
	require.Equal(t, saveStatusRemoteMissing, last.SaveResult.Status)
	require.Equal(t, sessionStateRemoteMissing, last.Session.State)
	assert.Equal(t, "external_edit_conflict_remote_missing", h.audit.lastTool())
}

func TestExternalEditRereadKeepsPrimaryDraftAndTracksLatestSnapshot(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("local draft\n"))

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")

	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)
	require.NotNil(t, conflict.Conflict)
	require.Equal(t, session.ID, conflict.Conflict.PrimaryDraftSessionID)

	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote newer\n"), "/srv/app/demo.txt")
	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)
	require.Nil(t, reread.Conflict)
	require.Equal(t, session.ID, reread.Session.ID)
	require.Equal(t, sessionStateClean, reread.Session.State)
	require.Equal(t, recordStateActive, reread.Session.RecordState)
	require.False(t, reread.Session.Hidden)
	require.Equal(t, hashBytes([]byte("remote newer\n")), sessionBaseHash(reread.Session))
	require.Equal(t, hashBytes([]byte("remote newer\n")), sessionLocalHash(reread.Session))

	currentData, readErr := os.ReadFile(reread.Session.LocalPath)
	require.NoError(t, readErr)
	require.Equal(t, []byte("remote newer\n"), currentData)
	bakeupFiles := readBakeupFiles(t, reread.Session.WorkspaceDir)
	require.Len(t, bakeupFiles, 1)
	require.Equal(t, []byte("local draft\n"), bakeupFiles[0])
}

func TestExternalEditReopenAfterCloseRebuildsFromRemoteAndMovesPriorBaseToBakeup(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("local draft\n"))

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")

	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)
	require.Equal(t, session.ID, reread.Session.ID)

	// 关闭会话：直接操作 manifest 将 session 标记为 abandoned+hidden，
	// 模拟 retireDocumentFamilyRecord 的效果（该方法已随 Delete 链一起移除）。
	h.svc.mu.Lock()
	s := h.svc.sessions[reread.Session.ID]
	if s != nil {
		s.RecordState = recordStateAbandoned
		s.Hidden = true
	}
	_ = h.svc.saveManifestLocked()
	h.svc.mu.Unlock()

	main := h.refreshSession(t, reread.Session.ID)
	require.Equal(t, recordStateAbandoned, main.RecordState)
	require.True(t, main.Hidden)
	require.True(t, isReusableClosedMainSession(main))

	h.remote.ClearError("ssh-b", "/srv/app/demo.txt")
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote latest\n"), "/srv/app/demo.txt")
	reopened, err := h.svc.Open(context.Background(), OpenRequest{
		AssetID:    101,
		SessionID:  "ssh-c",
		RemotePath: "/srv/app/demo.txt",
		EditorID:   "system-text",
	})
	require.NoError(t, err)
	require.Equal(t, reread.Session.ID, reopened.ID)
	require.Equal(t, recordStateActive, reopened.RecordState)
	require.False(t, reopened.Hidden)
	require.Equal(t, hashBytes([]byte("remote latest\n")), sessionBaseHash(reopened))
	require.Equal(t, hashBytes([]byte("remote latest\n")), sessionLocalHash(reopened))

	currentData, readErr := os.ReadFile(reopened.LocalPath)
	require.NoError(t, readErr)
	require.Equal(t, []byte("remote latest\n"), currentData)
	bakeupFiles := readBakeupFiles(t, reopened.WorkspaceDir)
	require.Len(t, bakeupFiles, 2)
}

func TestExternalEditRetentionCleanupRemovesExpiredBakeupEntries(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	h.cfg.ExternalEditCleanupRetentionDays = 1
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("local draft\n"))

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote newer\n"), "/srv/app/demo.txt")
	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)

	paths := bakeupEntryPaths(t, reread.Session.WorkspaceDir)
	require.Len(t, paths, 1)
	oldTime := h.now.Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(paths[0], oldTime, oldTime))

	h.svc.runRetentionCleanup()

	entries, err := os.ReadDir(filepath.Join(reread.Session.WorkspaceDir, "bakeup"))
	require.NoError(t, err)
	require.Len(t, entries, 0)
}

func TestExternalEditRestoreCleanupRemovesExpiredBakeupEntries(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b", "ssh-c"} })
	h.cfg.ExternalEditCleanupRetentionDays = 1
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("local draft\n"))

	h.remote.SetError("ssh-b", "/srv/app/demo.txt", errors.New("SSH 会话不存在: ssh-b"))
	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	h.remote.SetFile("ssh-c", "/srv/app/demo.txt", []byte("remote newer\n"), "/srv/app/demo.txt")
	reread, err := h.svc.Resolve(context.Background(), session.ID, resolutionReread)
	require.NoError(t, err)
	require.Equal(t, saveStatusReread, reread.Status)

	paths := bakeupEntryPaths(t, reread.Session.WorkspaceDir)
	require.Len(t, paths, 1)
	oldTime := h.now.Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(paths[0], oldTime, oldTime))

	require.NoError(t, h.svc.Close())

	cfg := &bootstrap.AppConfig{
		ExternalEditDefaultEditorID:      "system-text",
		ExternalEditWorkspaceRoot:        h.manifest,
		ExternalEditCleanupRetentionDays: 1,
	}
	reopened, err := NewService(Options{
		DataDir:        h.manifest,
		ConfigProvider: func() *bootstrap.AppConfig { return cfg },
		ConfigSaver:    func(next *bootstrap.AppConfig) error { *cfg = *next; return nil },
		Remote:         h.remote,
		FindSessions:   func(int64) []string { return []string{"ssh-b", "ssh-c"} },
		Assets:         rebindAssetFinder{},
		Audit:          h.audit,
		Emit:           func(Event) {},
		Launch:         launcherFunc(func(string, []string) error { return nil }),
		Now:            h.svc.now,
	})
	require.NoError(t, err)
	require.NoError(t, reopened.Start(context.Background()))
	defer func() { _ = reopened.Close() }()

	entries, err := os.ReadDir(filepath.Join(reread.Session.WorkspaceDir, "bakeup"))
	require.NoError(t, err)
	require.Len(t, entries, 0)
}

func TestExternalEditSavedRecordStaysActiveAfterSave(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("hello saved\n"))

	result, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, result.Status)
	require.NotNil(t, result.Session)
	require.Equal(t, recordStateActive, result.Session.RecordState)
	require.False(t, result.Session.Hidden)
	require.Equal(t, sessionStateClean, result.Session.State)
}

func TestExternalEditManualRestoredDraftDoesNotAutoSave(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	h.svc.mu.Lock()
	h.svc.sessions[session.ID].SaveMode = saveModeManualRestore
	h.svc.mu.Unlock()

	markDirtyLocalCopy(t, session, []byte("restored manual\n"))
	h.svc.reconcileLocalCopy(session.ID)
	time.Sleep(autoSaveDebounce + 200*time.Millisecond)
	require.Empty(t, h.remote.writes)

	manual, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, manual.Status)
	require.Equal(t, recordStateActive, manual.Session.RecordState)
	require.False(t, manual.Session.Hidden)
}


func TestExternalEditRestoreKeepsSavedDraftVisibleAndAbandonedHidden(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	completed := h.openSession(t, "ssh-b", "/srv/app/completed.txt", "/srv/app/completed.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, completed, []byte("saved\n"))
	saved, err := h.svc.Save(context.Background(), completed.ID)
	require.NoError(t, err)
	require.False(t, saved.Session.Hidden)

	abandoned := h.openSession(t, "ssh-b", "/srv/app/abandoned.txt", "/srv/app/abandoned.txt", []byte("draft\n"))
	// 直接操作 manifest 将 session 标记为 abandoned+hidden
	h.svc.mu.Lock()
	if s := h.svc.sessions[abandoned.ID]; s != nil {
		s.RecordState = recordStateAbandoned
		s.Hidden = true
	}
	_ = h.svc.saveManifestLocked()
	h.svc.mu.Unlock()

	require.NoError(t, h.svc.Close())

	cfg := &bootstrap.AppConfig{
		ExternalEditDefaultEditorID: "system-text",
		ExternalEditWorkspaceRoot:   h.manifest,
	}
	reopened, err := NewService(Options{
		DataDir:        h.manifest,
		ConfigProvider: func() *bootstrap.AppConfig { return cfg },
		ConfigSaver:    func(next *bootstrap.AppConfig) error { *cfg = *next; return nil },
		Remote:         h.remote,
		FindSessions:   func(int64) []string { return []string{"ssh-b"} },
		Assets:         rebindAssetFinder{},
		Audit:          h.audit,
		Emit:           func(Event) {},
		Launch:         launcherFunc(func(string, []string) error { return nil }),
		Now:            h.svc.now,
	})
	require.NoError(t, err)
	require.NoError(t, reopened.Start(context.Background()))
	defer func() { _ = reopened.Close() }()

	completedRestored := reopened.getSession(completed.ID)
	require.NotNil(t, completedRestored)
	require.Equal(t, recordStateActive, completedRestored.RecordState)
	require.False(t, completedRestored.Hidden)
	require.Equal(t, saveModeManualRestore, completedRestored.SaveMode)
	require.True(t, completedRestored.ResumeRequired)

	abandonedRestored := reopened.getSession(abandoned.ID)
	require.NotNil(t, abandonedRestored)
	require.Equal(t, recordStateAbandoned, abandonedRestored.RecordState)
	require.True(t, abandonedRestored.Hidden)
	require.Equal(t, saveModeManualRestore, abandonedRestored.SaveMode)
}

func TestExternalEditRestoreAbandonedRecordDoesNotReactivateOnLocalChange(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	abandoned := h.openSession(t, "ssh-b", "/srv/app/abandoned.txt", "/srv/app/abandoned.txt", []byte("draft\n"))
	// 直接操作 manifest 将 session 标记为 abandoned+hidden
	h.svc.mu.Lock()
	if s := h.svc.sessions[abandoned.ID]; s != nil {
		s.RecordState = recordStateAbandoned
		s.Hidden = true
	}
	_ = h.svc.saveManifestLocked()
	h.svc.mu.Unlock()

	require.NoError(t, h.svc.Close())

	cfg := &bootstrap.AppConfig{
		ExternalEditDefaultEditorID: "system-text",
		ExternalEditWorkspaceRoot:   h.manifest,
	}
	reopened, err := NewService(Options{
		DataDir:        h.manifest,
		ConfigProvider: func() *bootstrap.AppConfig { return cfg },
		ConfigSaver:    func(next *bootstrap.AppConfig) error { *cfg = *next; return nil },
		Remote:         h.remote,
		FindSessions:   func(int64) []string { return []string{"ssh-b"} },
		Assets:         rebindAssetFinder{},
		Audit:          h.audit,
		Emit:           func(Event) {},
		Launch:         launcherFunc(func(string, []string) error { return nil }),
		Now:            h.svc.now,
	})
	require.NoError(t, err)
	require.NoError(t, reopened.Start(context.Background()))
	defer func() { _ = reopened.Close() }()

	abandonedRestored := reopened.getSession(abandoned.ID)
	require.NotNil(t, abandonedRestored)
	markDirtyLocalCopy(t, abandonedRestored, []byte("draft changed after restore\n"))
	reopened.reconcileLocalCopy(abandoned.ID)

	abandonedCurrent := reopened.getSession(abandoned.ID)
	require.Equal(t, recordStateAbandoned, abandonedCurrent.RecordState)
	require.True(t, abandonedCurrent.Hidden)
}

func TestExternalEditAbandonedSessionCancelsPendingAutoSave(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	markDirtyLocalCopy(t, session, []byte("autosave pending\n"))
	h.svc.reconcileLocalCopy(session.ID)

	// 直接操作 manifest 将 session 标记为 abandoned+hidden，模拟 UI 关闭会话
	h.svc.mu.Lock()
	if s := h.svc.sessions[session.ID]; s != nil {
		s.RecordState = recordStateAbandoned
		s.Hidden = true
	}
	_ = h.svc.saveManifestLocked()
	h.svc.mu.Unlock()

	time.Sleep(autoSaveDebounce + 200*time.Millisecond)
	require.Empty(t, h.remote.writes)

	stored := h.refreshSession(t, session.ID)
	require.Equal(t, recordStateAbandoned, stored.RecordState)
	require.True(t, stored.Hidden)
}

func TestExternalEditAutoSaveEmitsPendingRunningAndSavedTimeline(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	markDirtyLocalCopy(t, session, []byte("autosave timeline\n"))
	h.svc.reconcileLocalCopy(session.ID)

	require.Eventually(t, func() bool {
		events := h.snapshotEvents()
		hasPending := false
		hasRunning := false
		hasSaved := false
		for _, event := range events {
			if event.Type == eventSessionAutoSave && event.AutoSave != nil && event.AutoSave.DocumentKey == session.DocumentKey {
				hasPending = hasPending || event.AutoSave.Phase == autoSavePhasePending
				hasRunning = hasRunning || event.AutoSave.Phase == autoSavePhaseRunning
			}
			if event.Type == eventSessionSaved && event.Session != nil && event.Session.ID == session.ID && event.SaveResult != nil && event.SaveResult.Automatic {
				hasSaved = true
			}
		}
		return hasPending && hasRunning && hasSaved
	}, 2*time.Second, 20*time.Millisecond)
}

func TestExternalEditAutoSaveStillEntersConflictAfterRemoteCompare(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	markDirtyLocalCopy(t, session, []byte("autosave local\n"))
	h.remote.SetFile("ssh-b", "/srv/app/demo.txt", []byte("remote changed\n"), "/srv/app/demo.txt")
	h.svc.reconcileLocalCopy(session.ID)

	require.Eventually(t, func() bool {
		current := h.refreshSession(t, session.ID)
		return current.State == sessionStateConflict
	}, 2*time.Second, 20*time.Millisecond)

	current := h.refreshSession(t, session.ID)
	require.Equal(t, sessionStateConflict, current.State)

	events := h.snapshotEvents()
	foundConflict := false
	for _, event := range events {
		if event.Type == eventSessionConflict && event.Session != nil && event.Session.ID == session.ID && event.SaveResult != nil {
			require.True(t, event.SaveResult.Automatic)
			require.Equal(t, saveStatusConflict, event.SaveResult.Status)
			foundConflict = true
		}
	}
	require.True(t, foundConflict)
	require.Empty(t, h.remote.writes)
}

func TestExternalEditMergeApplySavesFinalDraftAndAdvancesHashes(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("base\n"))
	markDirtyLocalCopy(t, session, []byte("local draft\n"))
	h.remote.SetFile("ssh-b", "/srv/app/demo.txt", []byte("remote draft\n"), "/srv/app/demo.txt")

	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	prepare, err := h.svc.PrepareMerge(session.ID)
	require.NoError(t, err)
	require.Equal(t, "local draft\n", prepare.LocalContent)
	require.Equal(t, "remote draft\n", prepare.RemoteContent)
	require.Equal(t, hashBytes([]byte("remote draft\n")), prepare.RemoteHash)
	h.remote.SetInfoHash("ssh-b", "/srv/app/demo.txt", hashBytes([]byte("remote draft\n")))

	finalData := []byte("merged final\n")
	applied, err := h.svc.ApplyMerge(context.Background(), MergeApplyRequest{
		SessionID:    session.ID,
		FinalContent: string(finalData),
		RemoteHash:   prepare.RemoteHash,
	})
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, applied.Status)
	require.Equal(t, hashBytes(finalData), sessionBaseHash(applied.Session))
	require.Equal(t, hashBytes(finalData), sessionLocalHash(applied.Session))
	require.False(t, applied.Session.Hidden)
	require.Equal(t, recordStateActive, applied.Session.RecordState)
	require.Empty(t, applied.Session.MergeRemoteSHA256)
	assert.Equal(t, "external_edit_merge_apply", h.audit.lastTool())
}

func TestExternalEditMergeApplyBlocksStaleRemoteSnapshot(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("base\n"))
	markDirtyLocalCopy(t, session, []byte("local draft\n"))
	h.remote.SetFile("ssh-b", "/srv/app/demo.txt", []byte("remote draft\n"), "/srv/app/demo.txt")

	conflict, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, conflict.Status)

	prepare, err := h.svc.PrepareMerge(session.ID)
	require.NoError(t, err)
	h.remote.SetFile("ssh-b", "/srv/app/demo.txt", []byte("remote changed again\n"), "/srv/app/demo.txt")
	h.remote.SetInfoHash("ssh-b", "/srv/app/demo.txt", prepare.RemoteHash)

	applied, err := h.svc.ApplyMerge(context.Background(), MergeApplyRequest{
		SessionID:    session.ID,
		FinalContent: "merged final\n",
		RemoteHash:   prepare.RemoteHash,
	})
	require.NoError(t, err)
	require.Equal(t, saveStatusConflict, applied.Status)
	require.Contains(t, applied.Message, "合并期间再次变化")
	require.NotNil(t, applied.Conflict)
	require.Empty(t, h.remote.writes)
}

func TestExternalEditRestartRecoveryMarksResumeRequiredWithoutAutoUpload(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("base\n"))
	markDirtyLocalCopy(t, session, []byte("local after crash\n"))
	h.svc.reconcileLocalCopy(session.ID)
	require.NoError(t, h.svc.Close())

	cfg := &bootstrap.AppConfig{
		ExternalEditDefaultEditorID: "system-text",
		ExternalEditWorkspaceRoot:   h.manifest,
	}
	reopened, err := NewService(Options{
		DataDir:        h.manifest,
		ConfigProvider: func() *bootstrap.AppConfig { return cfg },
		ConfigSaver:    func(next *bootstrap.AppConfig) error { *cfg = *next; return nil },
		Remote:         h.remote,
		FindSessions:   func(int64) []string { return []string{"ssh-b"} },
		Assets:         rebindAssetFinder{},
		Audit:          h.audit,
		Emit:           func(Event) {},
		Launch:         launcherFunc(func(string, []string) error { return nil }),
		Now:            h.svc.now,
	})
	require.NoError(t, err)
	require.NoError(t, reopened.Start(context.Background()))
	defer func() { _ = reopened.Close() }()

	restored := reopened.getSession(session.ID)
	require.NotNil(t, restored)
	require.True(t, restored.ResumeRequired)
	require.Equal(t, saveModeManualRestore, restored.SaveMode)

	reopened.reconcileLocalCopy(session.ID)
	time.Sleep(autoSaveDebounce + 200*time.Millisecond)
	require.Empty(t, h.remote.writes)

	saved, err := reopened.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, saved.Status)
	require.False(t, saved.Session.ResumeRequired)
}

func TestExternalEditRuntimeDirtyDraftMarksPendingReview(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/runtime-pending.txt", "/srv/app/runtime-pending.txt", []byte("base\n"))

	markDirtyLocalCopy(t, session, []byte("runtime dirty\n"))
	h.svc.reconcileLocalCopy(session.ID)

	stored := h.refreshSession(t, session.ID)
	require.True(t, stored.PendingReview)
	require.Equal(t, sessionStateDirty, stored.State)
	require.Equal(t, saveModeAutoLive, stored.SaveMode)

	events := h.snapshotEvents()
	require.NotEmpty(t, events)
	require.Condition(t, func() bool {
		for _, event := range events {
			if event.Type == eventSessionChanged && event.Session != nil && event.Session.ID == session.ID && event.Session.PendingReview {
				return true
			}
		}
		return false
	})
}

func TestExternalEditContinueClearsRuntimePendingReview(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/runtime-pending.txt", "/srv/app/runtime-pending.txt", []byte("base\n"))

	markDirtyLocalCopy(t, session, []byte("runtime dirty\n"))
	h.svc.reconcileLocalCopy(session.ID)

	continued, err := h.svc.Continue(session.ID)
	require.NoError(t, err)
	require.NotNil(t, continued)
	require.False(t, continued.PendingReview)
	require.Equal(t, sessionStateDirty, continued.State)

	stored := h.refreshSession(t, session.ID)
	require.False(t, stored.PendingReview)
	require.Equal(t, sessionStateDirty, stored.State)

	events := h.snapshotEvents()
	require.NotEmpty(t, events)
	require.Condition(t, func() bool {
		for _, event := range events {
			if event.Type == eventSessionChanged && event.Session != nil && event.Session.ID == session.ID && !event.Session.PendingReview {
				return true
			}
		}
		return false
	})
	assert.Equal(t, "external_edit_continue", h.audit.lastTool())
}

func TestExternalEditClipboardResidueIsFilteredFromManifestRestoreAndList(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	valid := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("base\n"))
	residue := h.openSession(
		t,
		"ssh-b",
		"/srv/app/folder/clipboard-d29e2e94d3cae23119571647cf236bee83860f702e384e36d17305631c609c88.png",
		"/srv/app/folder/clipboard-d29e2e94d3cae23119571647cf236bee83860f702e384e36d17305631c609c88.png",
		[]byte("clipboard residue\n"),
	)
	h.svc.mu.Lock()
	h.svc.sessions[residue.ID].LocalPath = filepath.Join(
		h.manifest,
		"clipboard-images",
		"clipboard-6607ba08467079f385199f18c460e71b33008a531841e07a90f9b4b613629f88.png",
	)
	h.svc.sessions[residue.ID].WorkspaceDir = filepath.Join(h.manifest, "clipboard-images")
	require.NoError(t, os.MkdirAll(h.svc.sessions[residue.ID].WorkspaceDir, 0o700))
	require.NoError(t, os.WriteFile(h.svc.sessions[residue.ID].LocalPath, []byte("clipboard residue\n"), 0o600))
	require.NoError(t, h.svc.saveManifestLocked())
	h.svc.mu.Unlock()
	require.NoError(t, h.svc.Close())

	cfg := &bootstrap.AppConfig{
		ExternalEditDefaultEditorID: "system-text",
		ExternalEditWorkspaceRoot:   h.manifest,
	}
	reopened, err := NewService(Options{
		DataDir:        h.manifest,
		ConfigProvider: func() *bootstrap.AppConfig { return cfg },
		ConfigSaver:    func(next *bootstrap.AppConfig) error { *cfg = *next; return nil },
		Remote:         h.remote,
		FindSessions:   func(int64) []string { return []string{"ssh-b"} },
		Assets:         rebindAssetFinder{},
		Audit:          h.audit,
		Emit:           func(Event) {},
		Launch:         launcherFunc(func(string, []string) error { return nil }),
		Now:            h.svc.now,
	})
	require.NoError(t, err)
	require.NoError(t, reopened.Start(context.Background()))
	defer func() { _ = reopened.Close() }()

	require.NotNil(t, reopened.getSession(valid.ID))
	require.Nil(t, reopened.getSession(residue.ID))
	require.Empty(t, reopened.documentRunners)
	for _, session := range reopened.ListSessions() {
		require.NotContains(t, session.RemotePath, "folder/clipboard")
		require.NotContains(t, session.LocalPath, "clipboard-images")
	}

	manifest, err := os.ReadFile(filepath.Join(h.manifest, "storage", "manifest.json"))
	require.NoError(t, err)
	require.NotContains(t, string(manifest), "folder/clipboard")
	require.NotContains(t, string(manifest), "clipboard-images")
}

func TestExternalEditClipboardResidueRuntimeEntryPointsCleanWithoutRunner(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	residue := h.openSession(
		t,
		"ssh-b",
		"/srv/app/folder/clipboard-d29e2e94d3cae23119571647cf236bee83860f702e384e36d17305631c609c88.png",
		"/srv/app/folder/clipboard-d29e2e94d3cae23119571647cf236bee83860f702e384e36d17305631c609c88.png",
		[]byte("clipboard residue\n"),
	)
	h.svc.mu.Lock()
	h.svc.sessions[residue.ID].LocalPath = filepath.Join(
		h.manifest,
		"clipboard-images",
		"clipboard-6607ba08467079f385199f18c460e71b33008a531841e07a90f9b4b613629f88.png",
	)
	h.svc.sessions[residue.ID].WorkspaceDir = filepath.Join(h.manifest, "clipboard-images")
	require.NoError(t, os.MkdirAll(h.svc.sessions[residue.ID].WorkspaceDir, 0o700))
	require.NoError(t, os.WriteFile(h.svc.sessions[residue.ID].LocalPath, []byte("clipboard residue\n"), 0o600))
	require.NoError(t, h.svc.saveManifestLocked())
	h.svc.mu.Unlock()

	_, err := h.svc.Save(context.Background(), residue.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "外部编辑会话不存在")
	require.Nil(t, h.svc.getSession(residue.ID))
	require.Empty(t, h.svc.ListSessions())
	require.Empty(t, h.svc.documentRunners)
	require.Empty(t, h.remote.writes)

	manifest, err := os.ReadFile(filepath.Join(h.manifest, "storage", "manifest.json"))
	require.NoError(t, err)
	require.NotContains(t, string(manifest), "folder/clipboard")
	require.NotContains(t, string(manifest), "clipboard-images")
}
