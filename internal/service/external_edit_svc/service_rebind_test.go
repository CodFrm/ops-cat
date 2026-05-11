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
	writes      []string
}

func newRebindRemoteStub() *rebindRemoteStub {
	return &rebindRemoteStub{
		files:       make(map[string]map[string]rebindRemoteFile),
		missing:     make(map[string]map[string]error),
		writeErrors: make(map[string]map[string]error),
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
	return &sftp_svc.RemoteFileInfo{
		Path:     remotePath,
		Size:     int64(len(file.data)),
		Mode:     uint32(0o600),
		ModTime:  1700000000,
		Regular:  true,
		RealPath: file.realPath,
		SHA256:   hashBytes(file.data),
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

func (h *rebindHarness) advanceNow(delta time.Duration) {
	h.now = h.now.Add(delta)
	h.svc.now = func() time.Time {
		return h.now
	}
}

func markDirtyLocalCopy(t *testing.T, session *Session, data []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(session.LocalPath, data, 0o600))
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
	require.Equal(t, recordStateCompleted, stored.RecordState)
	require.True(t, stored.Hidden)
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
	require.NotNil(t, reread.Conflict)
	require.Equal(t, session.ID, reread.Conflict.PrimaryDraftSessionID)
	require.Equal(t, reread.Session.ID, reread.Conflict.LatestSnapshotSessionID)

	original := h.refreshSession(t, session.ID)
	require.Equal(t, sessionStateStale, original.State)
	require.Equal(t, reread.Session.ID, original.SupersededBySessionID)
}

func TestExternalEditCompletedRecordBecomesHiddenAfterSave(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, session, []byte("hello saved\n"))

	result, err := h.svc.Save(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, saveStatusSaved, result.Status)
	require.NotNil(t, result.Session)
	require.Equal(t, recordStateCompleted, result.Session.RecordState)
	require.True(t, result.Session.Hidden)
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
	require.Equal(t, recordStateCompleted, manual.Session.RecordState)
}

func TestExternalEditDeleteRecordOnlyMarksAbandonedAndHidden(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	result, err := h.svc.DeleteSession(session.ID, false)
	require.NoError(t, err)
	require.Equal(t, "deleted_record_only", result.Status)
	require.NotNil(t, result.Session)
	require.Equal(t, recordStateAbandoned, result.Session.RecordState)
	require.True(t, result.Session.Hidden)

	stored := h.refreshSession(t, session.ID)
	require.Equal(t, recordStateAbandoned, stored.RecordState)
	require.True(t, stored.Hidden)
}

func TestExternalEditDeleteWithLocalFailureFallsBackToError(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	require.NoError(t, os.MkdirAll(filepath.Join(session.WorkspaceDir, "nested"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(session.WorkspaceDir, "nested", "child.txt"), []byte("busy"), 0o600))
	// 删除目录失败不容易在测试环境稳定复现，直接制造越界路径来触发 cleanup 保护分支。
	h.svc.mu.Lock()
	h.svc.sessions[session.ID].WorkspaceDir = filepath.Join(h.manifest, "..", "escape")
	h.svc.mu.Unlock()

	_, err := h.svc.DeleteSession(session.ID, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "删除本地副本失败")

	stored := h.refreshSession(t, session.ID)
	require.Equal(t, recordStateError, stored.RecordState)
	require.NotNil(t, stored.LastError)
	require.Equal(t, "delete_local_copy", stored.LastError.Step)
}

func TestExternalEditRestoreKeepsCompletedAndAbandonedHidden(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	completed := h.openSession(t, "ssh-b", "/srv/app/completed.txt", "/srv/app/completed.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, completed, []byte("saved\n"))
	saved, err := h.svc.Save(context.Background(), completed.ID)
	require.NoError(t, err)
	require.True(t, saved.Session.Hidden)

	abandoned := h.openSession(t, "ssh-b", "/srv/app/abandoned.txt", "/srv/app/abandoned.txt", []byte("draft\n"))
	_, err = h.svc.DeleteSession(abandoned.ID, false)
	require.NoError(t, err)

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
	require.Equal(t, recordStateCompleted, completedRestored.RecordState)
	require.True(t, completedRestored.Hidden)
	require.Equal(t, saveModeManualRestore, completedRestored.SaveMode)

	abandonedRestored := reopened.getSession(abandoned.ID)
	require.NotNil(t, abandonedRestored)
	require.Equal(t, recordStateAbandoned, abandonedRestored.RecordState)
	require.True(t, abandonedRestored.Hidden)
	require.Equal(t, saveModeManualRestore, abandonedRestored.SaveMode)
}

func TestExternalEditRestoreHiddenRecordsDoNotReactivateOnLocalChange(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	completed := h.openSession(t, "ssh-b", "/srv/app/completed.txt", "/srv/app/completed.txt", []byte("hello\n"))
	markDirtyLocalCopy(t, completed, []byte("saved\n"))
	_, err := h.svc.Save(context.Background(), completed.ID)
	require.NoError(t, err)

	abandoned := h.openSession(t, "ssh-b", "/srv/app/abandoned.txt", "/srv/app/abandoned.txt", []byte("draft\n"))
	_, err = h.svc.DeleteSession(abandoned.ID, false)
	require.NoError(t, err)

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
	markDirtyLocalCopy(t, completedRestored, []byte("changed after restore\n"))
	reopened.reconcileLocalCopy(completed.ID)

	abandonedRestored := reopened.getSession(abandoned.ID)
	require.NotNil(t, abandonedRestored)
	markDirtyLocalCopy(t, abandonedRestored, []byte("draft changed after restore\n"))
	reopened.reconcileLocalCopy(abandoned.ID)

	completedCurrent := reopened.getSession(completed.ID)
	require.Equal(t, recordStateCompleted, completedCurrent.RecordState)
	require.True(t, completedCurrent.Hidden)

	abandonedCurrent := reopened.getSession(abandoned.ID)
	require.Equal(t, recordStateAbandoned, abandonedCurrent.RecordState)
	require.True(t, abandonedCurrent.Hidden)
}

func TestExternalEditDeleteRecordOnlyCancelsPendingAutoSave(t *testing.T) {
	h := newRebindHarness(t, func(int64) []string { return []string{"ssh-b"} })
	session := h.openSession(t, "ssh-b", "/srv/app/demo.txt", "/srv/app/demo.txt", []byte("hello\n"))

	markDirtyLocalCopy(t, session, []byte("autosave pending\n"))
	h.svc.reconcileLocalCopy(session.ID)

	_, err := h.svc.DeleteSession(session.ID, false)
	require.NoError(t, err)

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
