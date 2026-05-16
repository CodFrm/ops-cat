package external_edit_svc

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/google/uuid"
	"github.com/opskat/opskat/internal/service/sftp_svc"
	"go.uber.org/zap"
)

func (s *Service) withDocumentRunner(sessionID string, fn func() error) error {
	session := s.getSession(sessionID)
	if session == nil {
		return fmt.Errorf("外部编辑会话不存在")
	}
	documentKey := strings.TrimSpace(session.DocumentKey)
	if documentKey == "" {
		documentKey = session.ID
	}

	s.mu.Lock()
	runner := s.documentRunners[documentKey]
	if runner == nil {
		runner = &sync.Mutex{}
		s.documentRunners[documentKey] = runner
	}
	s.mu.Unlock()

	runner.Lock()
	defer runner.Unlock()
	return fn()
}

func (s *Service) guardMutableSession(session *Session) error {
	if session == nil {
		return fmt.Errorf("外部编辑会话不存在")
	}
	if isSyncSuppressedRecord(session) {
		return fmt.Errorf("当前记录已归档，不再参与同步；请重新打开该远程文件后再继续编辑")
	}
	switch session.State {
	case sessionStateStale:
		return fmt.Errorf("当前副本已被新的远程版本替代，不能继续同步；%s", externalEditReconnectHint)
	case sessionStateExpired:
		return fmt.Errorf("当前副本已过期，不能继续同步；%s", externalEditReconnectHint)
	default:
		return nil
	}
}

func (s *Service) describeConflict(session *Session, snapshotSessionID string) *Conflict {
	if session == nil || strings.TrimSpace(session.DocumentKey) == "" {
		return nil
	}
	if session.State != sessionStateConflict && session.State != sessionStateStale && session.State != sessionStateRemoteMissing {
		return nil
	}

	primaryDraftID := session.ID
	latestSnapshotID := strings.TrimSpace(snapshotSessionID)

	if session.State == sessionStateStale && session.SourceSessionID != "" {
		primaryDraftID = session.ID
		if latestSnapshotID == "" {
			latestSnapshotID = strings.TrimSpace(session.SupersededBySessionID)
		}
	}

	if session.State == sessionStateConflict || session.State == sessionStateRemoteMissing {
		for _, candidate := range s.ListSessions() {
			if candidate == nil || candidate.DocumentKey != session.DocumentKey || candidate.ID == session.ID {
				continue
			}
			if candidate.SourceSessionID == session.ID && candidate.State == sessionStateClean {
				latestSnapshotID = candidate.ID
				break
			}
		}
	}

	return &Conflict{
		DocumentKey:             session.DocumentKey,
		PrimaryDraftSessionID:   primaryDraftID,
		LatestSnapshotSessionID: latestSnapshotID,
	}
}

func (s *Service) findSessionsByAsset(assetID int64) []string {
	if s.findSessions == nil {
		return nil
	}
	candidates := s.findSessions(assetID)
	if len(candidates) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func (s *Service) resolveDocumentTransport(session *Session) (*documentTransport, error) {
	if session == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}

	candidates := s.documentCandidateSessionIDs(session)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("当前远程文件已不可访问；%s", externalEditReconnectHint)
	}

	var firstMatch *documentTransport
	var missingMatch *documentTransport
	reachableDifferentDocument := false
	for _, candidateID := range candidates {
		transport, sameDocument, err := s.inspectDocumentTransport(session, candidateID)
		if err != nil {
			return nil, err
		}
		if transport == nil {
			if !sameDocument {
				reachableDifferentDocument = true
			}
			continue
		}
		if transport.Missing {
			if missingMatch == nil {
				missingMatch = transport
			}
			continue
		}
		if firstMatch == nil {
			firstMatch = transport
		}
	}

	if firstMatch != nil {
		return firstMatch, nil
	}
	if missingMatch != nil {
		return missingMatch, nil
	}
	if reachableDifferentDocument {
		return nil, fmt.Errorf("当前文件位置已变化，无法确认仍是同一份远程文件；%s", externalEditReconnectHint)
	}
	return nil, fmt.Errorf("当前远程文件已不可访问；%s", externalEditReconnectHint)
}

func (s *Service) validateOverwriteTransport(session *Session, info *sftp_svc.RemoteFileInfo) error {
	if session == nil {
		return fmt.Errorf("外部编辑会话不存在")
	}
	if info == nil {
		return fmt.Errorf("暂时无法确认当前远程文件状态，请稍后重试或重新打开该远程文件")
	}
	if info.IsDir || !info.Regular {
		return fmt.Errorf("远程路径已不是常规文件")
	}
	if !sameRemoteIdentity(session, info, session.RemotePath) {
		return fmt.Errorf("当前文件位置已变化，无法确认仍是同一份远程文件；%s", externalEditReconnectHint)
	}
	if os.FileMode(info.Mode).Perm()&0o200 == 0 {
		return fmt.Errorf("当前远程文件不可写，请先调整权限后再强制覆盖")
	}
	return nil
}

func (s *Service) documentCandidateSessionIDs(session *Session) []string {
	if session == nil {
		return nil
	}

	seen := make(map[string]struct{}, 4)
	candidates := make([]string, 0, 4)
	push := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		candidates = append(candidates, id)
	}

	push(session.SessionID)
	for _, id := range s.findSessionsByAsset(session.AssetID) {
		push(id)
	}
	return candidates
}

func (s *Service) inspectDocumentTransport(session *Session, candidateID string) (*documentTransport, bool, error) {
	if session == nil || candidateID == "" {
		return nil, false, nil
	}

	info, err := s.remote.Stat(candidateID, session.RemotePath)
	if err != nil {
		if isRemoteMissingError(err) {
			if !canConfirmRemotePathWithoutStat(session) {
				return nil, false, fmt.Errorf("当前远程文件位置已变化，无法确认是否仍是同一份文件；%s", externalEditReconnectHint)
			}
			return &documentTransport{
				SessionID:     candidateID,
				RemotePath:    session.RemotePath,
				CanonicalPath: session.RemoteRealPath,
				Missing:       true,
			}, true, nil
		}
		if isSSHSessionMissingError(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("验证当前远程文件失败: %w", err)
	}
	if info.IsDir || !info.Regular {
		return nil, false, fmt.Errorf("当前远程路径已不是常规文件")
	}

	canonicalPath := canonicalRemotePath(info, session.RemotePath)
	if buildDocumentKey(session.AssetID, canonicalPath) != session.DocumentKey {
		return nil, false, nil
	}
	return &documentTransport{
		SessionID:     candidateID,
		RemotePath:    session.RemotePath,
		CanonicalPath: canonicalPath,
		Info:          info,
	}, true, nil
}

func (s *Service) bindSessionTransport(sessionID string, transport *documentTransport) (*Session, error) {
	if transport == nil {
		return nil, fmt.Errorf("缺少可用的远程文件连接")
	}
	return s.updateSessionBinding(sessionID, transport.SessionID, transport.RemotePath, transport.CanonicalPath)
}

func (s *Service) updateSessionBinding(sessionID, nextSessionID, remotePath, remoteRealPath string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.sessions[sessionID]
	if session == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	session.SessionID = nextSessionID
	session.RemotePath = remotePath
	session.RemoteRealPath = remoteRealPath
	session.DocumentKey = buildDocumentKey(session.AssetID, remoteRealPath)
	session.LastError = nil
	session.UpdatedAt = s.now().Unix()
	if err := s.saveManifestLocked(); err != nil {
		return nil, err
	}
	return cloneSession(session), nil
}

func sameRemoteIdentity(session *Session, info *sftp_svc.RemoteFileInfo, fallbackPath string) bool {
	if session == nil || info == nil {
		return false
	}
	currentRealPath := strings.TrimSpace(session.RemoteRealPath)
	nextRealPath := strings.TrimSpace(canonicalRemotePath(info, fallbackPath))
	currentPath := strings.TrimSpace(session.RemotePath)
	fallbackPath = strings.TrimSpace(fallbackPath)

	if currentRealPath != "" && nextRealPath != "" {
		return currentRealPath == nextRealPath
	}
	if currentPath != "" && nextRealPath != "" {
		return currentPath == nextRealPath
	}
	if currentRealPath != "" && fallbackPath != "" {
		return currentRealPath == fallbackPath
	}
	return currentPath != "" && currentPath == fallbackPath
}

func canConfirmRemotePathWithoutStat(session *Session) bool {
	if session == nil {
		return false
	}
	currentPath := strings.TrimSpace(session.RemotePath)
	currentRealPath := strings.TrimSpace(session.RemoteRealPath)
	if currentPath == "" {
		return false
	}
	return currentRealPath == "" || currentRealPath == currentPath
}

func isSSHSessionMissingError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "ssh会话不存在") ||
		strings.Contains(text, "ssh 会话不存在") ||
		strings.Contains(text, "ssh session does not exist")
}

func (s *Service) rebuildDocumentSessionFromRemote(
	req OpenRequest,
	editor Editor,
	assetName string,
	source *Session,
	auditTool string,
) (*Session, error) {
	data, fileInfo, err := readRemoteEditableFile(s.remote, req.SessionID, req.RemotePath, s.maxReadFileSizeBytes())
	if err != nil {
		return nil, fmt.Errorf("读取远程文件失败: %w", err)
	}
	if fileInfo.IsDir || !fileInfo.Regular {
		return nil, fmt.Errorf("仅支持打开常规文本文件")
	}
	if !isLikelyText(req.RemotePath, data) {
		return nil, fmt.Errorf("当前文件不是可编辑文本文件")
	}
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
	localPath, workspaceDir, err := buildWorkspacePaths(workspaceRoot, req.AssetID, canonicalRemotePath(fileInfo, req.RemotePath))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建临时工作区失败: %w", err)
	}

	var bakeupPath string
	if source != nil {
		bakeupPath, err = s.moveFileToBakeup(source.WorkspaceRoot, source.WorkspaceDir, source.LocalPath)
		if err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(localPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("写入临时副本失败: %w", err)
	}
	baseHash := hashBytes(data)
	nowUnix := s.now().Unix()
	remoteHash := baseHash

	s.mu.Lock()
	var session *Session
	if source != nil {
		current := s.sessions[source.ID]
		if current == nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("外部编辑会话不存在")
		}
		if s.watchedDirs[workspaceDir] == 0 {
			if err := s.addWatchLocked(workspaceDir); err != nil {
				s.mu.Unlock()
				return nil, err
			}
		}
		current.AssetName = assetName
		current.DocumentKey = buildDocumentKey(req.AssetID, canonicalRemotePath(fileInfo, req.RemotePath))
		current.SessionID = req.SessionID
		current.RemotePath = req.RemotePath
		current.RemoteRealPath = canonicalRemotePath(fileInfo, req.RemotePath)
		current.LocalPath = localPath
		current.WorkspaceRoot = workspaceRoot
		current.WorkspaceDir = workspaceDir
		current.EditorID = editor.ID
		current.EditorName = editor.Name
		current.EditorPath = editor.Path
		current.EditorArgs = cloneArgs(editor.Args)
		current.OriginalSHA256 = remoteHash
		current.OriginalSize = fileInfo.Size
		current.OriginalModTime = fileInfo.ModTime
		current.LastLocalSHA256 = baseHash
		current.Dirty = false
		current.State = sessionStateClean
		current.RecordState = recordStateActive
		current.SaveMode = saveModeAutoLive
		current.Hidden = false
		current.Expired = false
		current.LastError = nil
		current.ResumeRequired = false
		current.MergeRemoteSHA256 = ""
		current.SourceSessionID = ""
		current.SupersededBySessionID = ""
		current.UpdatedAt = nowUnix
		current.LastLaunchedAt = nowUnix
		current.LastSyncedAt = nowUnix
		applyEncodingSnapshot(current, encodingSnapshot)
		session = current
	} else {
		session = &Session{
			ID:              uuid.NewString(),
			AssetID:         req.AssetID,
			AssetName:       assetName,
			DocumentKey:     buildDocumentKey(req.AssetID, canonicalRemotePath(fileInfo, req.RemotePath)),
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
			OriginalSHA256:  remoteHash,
			OriginalSize:    fileInfo.Size,
			OriginalModTime: fileInfo.ModTime,
			LastLocalSHA256: baseHash,
			State:           sessionStateClean,
			RecordState:     recordStateActive,
			SaveMode:        saveModeAutoLive,
			CreatedAt:       nowUnix,
			UpdatedAt:       nowUnix,
			LastLaunchedAt:  nowUnix,
			LastSyncedAt:    nowUnix,
		}
		applyEncodingSnapshot(session, encodingSnapshot)
		s.sessions[session.ID] = session
		if err := s.addWatchLocked(workspaceDir); err != nil {
			delete(s.sessions, session.ID)
			s.mu.Unlock()
			return nil, err
		}
	}
	if err := s.saveManifestLocked(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	cloned := cloneSession(session)
	s.mu.Unlock()

	if err := s.launch.Launch(editor.Path, append(cloneArgs(editor.Args), localPath)); err != nil {
		if source == nil {
			s.cleanupSessionAfterLaunchFailure(session.ID)
		}
		s.writeAudit(cloned, auditTool, false, map[string]any{"rebuild": true}, map[string]any{"bakeupPath": bakeupPath}, err)
		return nil, fmt.Errorf("启动外部编辑器失败: %w", err)
	}
	s.writeAudit(cloned, auditTool, true, map[string]any{"rebuild": true}, map[string]any{"bakeupPath": bakeupPath}, nil)
	s.emit(Event{Type: eventSessionOpened, Session: cloned})
	return cloned, nil
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
