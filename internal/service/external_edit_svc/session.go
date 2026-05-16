package external_edit_svc

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/google/uuid"
	"github.com/opskat/opskat/internal/service/sftp_svc"
	"go.uber.org/zap"
)

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
	documentKey := buildDocumentKey(req.AssetID, remoteRealPath)
	assetName := s.lookupAssetName(ctx, req.AssetID)
	nowUnix := s.now().Unix()
	remoteHash := remoteInfoHash(info)

	s.mu.Lock()
	var reusable *Session
	var reusableRank int
	// 这里优先复用已有主会话，而不是每次都重新拉一份本地副本：
	// 这样可以保留未保存的本地修改、watch 状态和审计上下文，避免双击同一文件时产生多份互相竞争的工作副本。
	for _, existing := range s.sessions {
		rank, ok := openSessionReuseRank(existing)
		if !ok || existing.DocumentKey != documentKey {
			continue
		}
		if _, statErr := os.Stat(existing.LocalPath); statErr != nil {
			s.removeSessionLocked(existing.ID)
			continue
		}
		if reusable == nil || rank < reusableRank || (rank == reusableRank && existing.UpdatedAt > reusable.UpdatedAt) {
			reusable = existing
			reusableRank = rank
		}
	}
	shouldRebuild := reusable != nil && remoteHash != "" && remoteHash != sessionBaseHash(reusable)
	var rebuildSource *Session
	if shouldRebuild {
		rebuildSource = cloneSession(reusable)
	}
	if reusable != nil {
		if shouldRebuild {
			s.mu.Unlock()
			return s.rebuildDocumentSessionFromRemote(req, *editor, assetName, rebuildSource, "external_edit_open")
		}
		if s.watchedDirs[reusable.WorkspaceDir] == 0 {
			if err := s.addWatchLocked(reusable.WorkspaceDir); err != nil {
				s.mu.Unlock()
				return nil, err
			}
		}
		reusable.SessionID = req.SessionID
		reusable.AssetName = assetName
		reusable.DocumentKey = documentKey
		reusable.RemotePath = req.RemotePath
		reusable.RemoteRealPath = remoteRealPath
		reusable.RecordState = recordStateActive
		reusable.SaveMode = saveModeAutoLive
		reusable.PendingReview = false
		reusable.Hidden = false
		reusable.LastError = nil
		reusable.ResumeRequired = false
		reusable.MergeRemoteSHA256 = ""
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
	localPath, workspaceDir, err := buildWorkspacePaths(workspaceRoot, req.AssetID, canonicalRemotePath(fileInfo, req.RemotePath))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建临时工作区失败: %w", err)
	}
	if err := os.WriteFile(localPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("写入临时副本失败: %w", err)
	}
	baseHash := hashBytes(data)
	sessionToken := uuid.NewString()

	session := &Session{
		ID:              sessionToken,
		AssetID:         req.AssetID,
		AssetName:       assetName,
		DocumentKey:     documentKey,
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
		OriginalSHA256:  baseHash,
		OriginalSize:    fileInfo.Size,
		OriginalModTime: fileInfo.ModTime,
		LastLocalSHA256: baseHash,
		State:           sessionStateClean,
		RecordState:     recordStateActive,
		SaveMode:        saveModeAutoLive,
		PendingReview:   false,
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
	var result *SaveResult
	err := s.withDocumentRunner(sessionID, func() error {
		var saveErr error
		result, saveErr = s.saveInternal(ctx, sessionID, "", false)
		return saveErr
	})
	return result, err
}

func (s *Service) Refresh(sessionID string) (*Session, error) {
	var result *Session
	err := s.withDocumentRunner(sessionID, func() error {
		var refreshErr error
		result, refreshErr = s.refreshInternal(sessionID)
		return refreshErr
	})
	return result, err
}

func (s *Service) refreshInternal(sessionID string) (*Session, error) {
	current := s.getSession(sessionID)
	if current == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	if err := s.guardMutableSession(current); err != nil {
		return nil, err
	}

	transport, transportErr := s.resolveDocumentTransport(current)
	if transportErr != nil {
		s.writeAudit(current, "external_edit_refresh", false, nil, nil, transportErr)
		return nil, transportErr
	}
	current, err := s.bindSessionTransport(sessionID, transport)
	if err != nil {
		return nil, err
	}

	localData, err := readLocalEditableFile(current.LocalPath, s.maxReadFileSizeBytes())
	if err != nil {
		return nil, fmt.Errorf("读取本地副本失败: %w", err)
	}
	localHash := hashBytes(localData)
	baseHash := sessionBaseHash(current)
	dirty := current.Dirty || localHash != baseHash

	if transport.Missing {
		refreshed := s.markSessionState(sessionID, sessionStateRemoteMissing, dirty, localHash)
		s.writeAudit(refreshed, "external_edit_refresh", true, map[string]any{"status": sessionStateRemoteMissing}, refreshed, nil)
		s.emit(Event{Type: eventSessionChanged, Session: refreshed})
		return refreshed, nil
	}

	remoteData, remoteInfo, err := readRemoteEditableFile(s.remote, current.SessionID, current.RemotePath, s.maxReadFileSizeBytes())
	if err != nil {
		if isRemoteMissingError(err) {
			refreshed := s.markSessionState(sessionID, sessionStateRemoteMissing, dirty, localHash)
			s.writeAudit(refreshed, "external_edit_refresh", true, map[string]any{"status": sessionStateRemoteMissing}, refreshed, nil)
			s.emit(Event{Type: eventSessionChanged, Session: refreshed})
			return refreshed, nil
		}
		refreshErr := fmt.Errorf("暂时无法确认当前远程文件状态，请稍后重试或重新打开该远程文件")
		s.writeAudit(current, "external_edit_refresh", false, nil, nil, refreshErr)
		return nil, refreshErr
	}
	if remoteInfo.IsDir || !remoteInfo.Regular {
		return nil, fmt.Errorf("远程路径已不是常规文件")
	}

	nextState := sessionStateClean
	remoteHash := hashBytes(remoteData)
	nextDirty := dirty
	switch {
	case remoteHash != baseHash:
		nextState = sessionStateConflict
		nextDirty = true
	case dirty:
		nextState = sessionStateDirty
	}
	refreshed := s.markSessionState(sessionID, nextState, nextDirty, localHash)
	s.writeAudit(refreshed, "external_edit_refresh", true, map[string]any{"status": nextState, "remoteBytes": len(remoteData)}, refreshed, nil)
	if nextState == sessionStateConflict {
		saveResult := &SaveResult{
			Status:   saveStatusConflict,
			Message:  "远程文件已有新版本，请先比对差异，再决定重新读取或强制覆盖",
			Session:  refreshed,
			Conflict: s.describeConflict(refreshed, ""),
		}
		s.pauseAutoSaveForDocument(refreshed.DocumentKey)
		s.emit(Event{Type: eventSessionConflict, Session: refreshed, SaveResult: saveResult})
		return refreshed, nil
	}
	s.emit(Event{Type: eventSessionChanged, Session: refreshed})
	return refreshed, nil
}

func (s *Service) Resolve(ctx context.Context, sessionID, resolution string) (*SaveResult, error) {
	var result *SaveResult
	err := s.withDocumentRunner(sessionID, func() error {
		switch resolution {
		case resolutionOverwrite, resolutionRecreate:
			var saveErr error
			result, saveErr = s.saveInternal(ctx, sessionID, resolution, false)
			return saveErr
		case resolutionReread:
			var rereadErr error
			result, rereadErr = s.rereadRemoteSession(sessionID)
			return rereadErr
		default:
			return fmt.Errorf("未知冲突处理动作: %s", resolution)
		}
	})
	return result, err
}

func (s *Service) saveInternal(ctx context.Context, sessionID, resolution string, automatic bool) (*SaveResult, error) {
	session := s.getSession(sessionID)
	if session == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	if err := s.guardMutableSession(session); err != nil {
		return nil, err
	}
	transport, transportErr := s.resolveDocumentTransport(session)
	if transportErr != nil {
		s.writeAudit(session, "external_edit_document_transport_blocked", false, map[string]any{"resolution": resolution}, nil, transportErr)
		failed := s.recordError(sessionID, "resolve_transport", transportErr)
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, transportErr
	}
	session, err := s.bindSessionTransport(sessionID, transport)
	if err != nil {
		return nil, err
	}
	s.clearRecordError(session)

	localData, err := readLocalEditableFile(session.LocalPath, s.maxReadFileSizeBytes())
	if err != nil {
		saveErr := fmt.Errorf("读取本地副本失败: %w", err)
		failed := s.recordError(sessionID, "read_local_copy", saveErr)
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, saveErr
	}
	if !isLikelyText(session.RemotePath, localData) {
		saveErr := fmt.Errorf("本地副本已不是可编辑文本文件")
		failed := s.recordError(sessionID, "validate_local_copy", saveErr)
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, saveErr
	}

	localHash := hashBytes(localData)
	baseHash := sessionBaseHash(session)
	if err := validateRoundTrip(session, localData); err != nil {
		s.writeAudit(session, "external_edit_save_validation_failed", false, map[string]any{"resolution": resolution}, nil, err)
		failed := s.recordError(sessionID, "validate_round_trip", err)
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, err
	}

	// 保存前永远重新读取远端状态。
	// overwrite / recreate 是显式用户决策；除此之外一旦发现远端内容漂移或文件缺失，就必须先停在冲突态，不能偷偷覆盖。
	currentInfo, err := s.remote.Stat(session.SessionID, session.RemotePath)
	if err != nil {
		if !isRemoteMissingError(err) {
			saveErr := fmt.Errorf("读取远程文件状态失败: %w", err)
			failed := s.recordError(sessionID, "stat_remote_file", saveErr)
			if failed != nil {
				s.emit(Event{Type: eventSessionChanged, Session: failed})
			}
			return nil, saveErr
		}
		if resolution != resolutionRecreate {
			result := s.markSessionState(sessionID, sessionStateRemoteMissing, true, localHash)
			saveResult := &SaveResult{
				Status:    saveStatusRemoteMissing,
				Message:   "远程文件不存在，请先确认是否需要重新创建远程文件",
				Session:   result,
				Conflict:  s.describeConflict(result, ""),
				Automatic: automatic,
			}
			s.pauseAutoSaveForDocument(result.DocumentKey)
			s.writeAudit(result, "external_edit_conflict_remote_missing", true, map[string]any{"resolution": resolution}, saveResult, nil)
			s.emit(Event{Type: eventSessionConflict, Session: result, SaveResult: saveResult})
			return saveResult, nil
		}
	} else {
		if currentInfo.IsDir || !currentInfo.Regular {
			return nil, fmt.Errorf("远程路径已不是常规文件")
		}

		if resolution != resolutionOverwrite {
			remoteData, _, readErr := readRemoteEditableFile(s.remote, session.SessionID, session.RemotePath, s.maxReadFileSizeBytes())
			if readErr != nil {
				if isRemoteMissingError(readErr) {
					result := s.markSessionState(sessionID, sessionStateRemoteMissing, true, localHash)
					saveResult := &SaveResult{
						Status:    saveStatusRemoteMissing,
						Message:   "远程文件不存在，请先确认是否需要重新创建远程文件",
						Session:   result,
						Conflict:  s.describeConflict(result, ""),
						Automatic: automatic,
					}
					s.pauseAutoSaveForDocument(result.DocumentKey)
					s.writeAudit(result, "external_edit_conflict_remote_missing", true, map[string]any{"resolution": resolution}, saveResult, nil)
					s.emit(Event{Type: eventSessionConflict, Session: result, SaveResult: saveResult})
					return saveResult, nil
				}
				saveErr := fmt.Errorf("读取远程文件失败: %w", readErr)
				failed := s.recordError(sessionID, "read_remote_file", saveErr)
				if failed != nil {
					s.emit(Event{Type: eventSessionChanged, Session: failed})
				}
				return nil, saveErr
			}
			remoteHash := hashBytes(remoteData)
			if remoteHash != baseHash {
				result := s.markSessionState(sessionID, sessionStateConflict, true, localHash)
				saveResult := &SaveResult{
					Status:    saveStatusConflict,
					Message:   "远程文件已有新版本，请先比对差异，再决定重新读取或强制覆盖",
					Session:   result,
					Conflict:  s.describeConflict(result, ""),
					Automatic: automatic,
				}
				s.pauseAutoSaveForDocument(result.DocumentKey)
				s.writeAudit(result, "external_edit_conflict_remote_changed", true, map[string]any{"resolution": resolution, "remoteSha256": remoteHash, "remoteBytes": len(remoteData)}, saveResult, nil)
				s.emit(Event{Type: eventSessionConflict, Session: result, SaveResult: saveResult})
				return saveResult, nil
			}
		}
	}

	// dirty 标记来自 watcher，hash 则来自当前磁盘内容。
	// 即使本地未变，也必须先完成远端漂移检测；reread 后的新 active draft 可能在用户再次保存前遇到远端并发改写，
	// 此时不能提前 noop 或被后续链路收敛为 clean/completed。
	if resolution == "" && localHash == baseHash && !session.Dirty {
		result := &SaveResult{
			Status:    saveStatusNoop,
			Message:   "本地副本没有新的变更",
			Session:   cloneSession(session),
			Automatic: automatic,
		}
		return result, nil
	}

	if resolution == resolutionOverwrite {
		if err := s.validateOverwriteTransport(session, currentInfo); err != nil {
			s.writeAudit(session, "external_edit_overwrite_validation_failed", false, map[string]any{"resolution": resolution}, nil, err)
			failed := s.recordError(sessionID, "validate_overwrite", err)
			if failed != nil {
				s.emit(Event{Type: eventSessionChanged, Session: failed})
			}
			return nil, err
		}
	}

	// SFTP 没有 compare-and-swap 原语，前面的 hash 检查与回写仍是两次远端操作。
	// 若远端在这个窗口内再次变化，只能由 overwrite/recreate/conflict 决策流继续兜底。
	if err := s.remote.WriteFile(session.SessionID, session.RemotePath, localData); err != nil {
		if isRemoteMissingError(err) {
			saveResult := s.markRemoteMissingConflict(sessionID, session, localHash, automatic, resolution, "write_remote_file")
			return saveResult, nil
		}
		s.writeAudit(session, "external_edit_save", false, map[string]any{"resolution": resolution}, nil, err)
		saveErr := fmt.Errorf("保存远程文件失败: %w", err)
		failed := s.recordError(sessionID, "write_remote_file", saveErr)
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, saveErr
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
		Status:    saveStatusSaved,
		Message:   "远程文件已保存",
		Session:   savedSession,
		Automatic: automatic,
	}
	toolName := "external_edit_save"
	if resolution == resolutionOverwrite {
		toolName = "external_edit_overwrite"
	}
	if resolution == resolutionRecreate {
		toolName = "external_edit_recreate"
	}
	if automatic {
		s.recordAutoSaveAudit(savedSession, toolName, saveResult)
	} else {
		s.writeAudit(savedSession, toolName, true, map[string]any{"resolution": resolution, "bytes": len(localData)}, saveResult, nil)
	}
	s.emit(Event{Type: eventSessionSaved, Session: savedSession, SaveResult: saveResult})
	s.resumeAutoSaveForDocument(savedSession.DocumentKey)
	return saveResult, nil
}

func (s *Service) markRemoteMissingConflict(sessionID string, session *Session, localHash string, automatic bool, resolution string, source string) *SaveResult {
	if sessionID == "" && session != nil {
		sessionID = session.ID
	}
	result := s.markSessionState(sessionID, sessionStateRemoteMissing, true, localHash)
	saveResult := &SaveResult{
		Status:    saveStatusRemoteMissing,
		Message:   "远程文件不存在，请先确认是否需要重新创建远程文件",
		Session:   result,
		Conflict:  s.describeConflict(result, ""),
		Automatic: automatic,
	}
	if result != nil {
		s.pauseAutoSaveForDocument(result.DocumentKey)
	}
	request := map[string]any{"resolution": resolution}
	if source != "" {
		request["source"] = source
	}
	s.writeAudit(result, "external_edit_conflict_remote_missing", true, request, saveResult, nil)
	s.emit(Event{Type: eventSessionConflict, Session: result, SaveResult: saveResult})
	return saveResult
}

func (s *Service) getSession(sessionID string) *Session {
	s.mu.RLock()
	session := cloneSession(s.sessions[sessionID])
	s.mu.RUnlock()
	if isExternalEditClipboardResidueSession(session) {
		s.cleanupClipboardResidueSession(session.ID)
		return nil
	}
	return session
}

func (s *Service) cleanupClipboardResidueSession(sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}

	s.mu.Lock()
	session := s.sessions[sessionID]
	if session == nil || !isExternalEditClipboardResidueSession(session) {
		s.mu.Unlock()
		return
	}
	s.removeSessionLocked(sessionID)
	if err := s.saveManifestLocked(); err != nil {
		logger.Default().Warn("persist external edit manifest after clipboard residue cleanup", zap.Error(err))
	}
	s.mu.Unlock()

	s.emit(Event{Type: eventSessionCleaned, Session: &Session{ID: sessionID}})
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
		session.DocumentKey = buildDocumentKey(session.AssetID, session.RemoteRealPath)
	} else {
		session.OriginalSize = int64(len(localData))
	}
	setSessionBaseHash(session, localHash)
	session.OriginalByteSample = byteSampleHex(localData)
	setSessionLocalHash(session, localHash)
	session.Dirty = false
	session.State = sessionStateClean
	session.RecordState = recordStateActive
	session.PendingReview = false
	session.Hidden = false
	session.Expired = false
	session.LastError = nil
	session.ResumeRequired = false
	session.MergeRemoteSHA256 = ""
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
	switch state {
	case sessionStateConflict, sessionStateRemoteMissing, sessionStateStale:
		session.RecordState = recordStateConflict
		session.PendingReview = false
		session.Hidden = false
	case sessionStateClean, sessionStateDirty, sessionStateExpired:
		session.PendingReview = state == sessionStateDirty && session.SaveMode == saveModeAutoLive
		if session.RecordState == "" || session.RecordState == recordStateCompleted || session.RecordState == recordStateAbandoned {
			session.RecordState = recordStateActive
		}
	}
	if localHash != "" {
		setSessionLocalHash(session, localHash)
	}
	session.UpdatedAt = s.now().Unix()
	if err := s.saveManifestLocked(); err != nil {
		logger.Default().Warn("persist external edit manifest after state change", zap.Error(err))
	}
	return cloneSession(session)
}

func (s *Service) recordError(sessionID, step string, err error) *Session {
	snapshot := buildErrorSnapshot(step, err, s.now().Unix())
	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.sessions[sessionID]
	if session == nil {
		return nil
	}
	session.RecordState = recordStateError
	session.Hidden = false
	session.LastError = snapshot
	session.UpdatedAt = s.now().Unix()
	if err := s.saveManifestLocked(); err != nil {
		logger.Default().Warn("persist external edit manifest after error snapshot", zap.Error(err))
	}
	return cloneSession(session)
}

func (s *Service) clearRecordError(session *Session) {
	if session == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.sessions[session.ID]
	if current == nil {
		return
	}
	current.LastError = nil
	if current.RecordState == recordStateError {
		current.RecordState = recordStateActive
	}
	if err := s.saveManifestLocked(); err != nil {
		logger.Default().Warn("persist external edit manifest after clearing error snapshot", zap.Error(err))
	}
}

func (s *Service) setMergeRemoteHash(sessionID, remoteHash string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.sessions[sessionID]
	if session == nil {
		return nil
	}
	session.MergeRemoteSHA256 = strings.TrimSpace(remoteHash)
	session.UpdatedAt = s.now().Unix()
	if err := s.saveManifestLocked(); err != nil {
		logger.Default().Warn("persist external edit manifest after merge prepare", zap.Error(err))
	}
	return cloneSession(session)
}

func (s *Service) clearMergeRemoteHash(sessionID string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.sessions[sessionID]
	if session == nil {
		return nil
	}
	session.MergeRemoteSHA256 = ""
	session.UpdatedAt = s.now().Unix()
	if err := s.saveManifestLocked(); err != nil {
		logger.Default().Warn("persist external edit manifest after merge apply", zap.Error(err))
	}
	return cloneSession(session)
}

func (s *Service) markResumeRequired(sessionID string, required bool) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := s.sessions[sessionID]
	if session == nil {
		return nil
	}
	session.ResumeRequired = required
	if required {
		session.PendingReview = false
	}
	session.Hidden = false
	if session.RecordState == "" || session.RecordState == recordStateCompleted || session.RecordState == recordStateAbandoned {
		session.RecordState = recordStateActive
	}
	session.UpdatedAt = s.now().Unix()
	if err := s.saveManifestLocked(); err != nil {
		logger.Default().Warn("persist external edit manifest after recovery marker", zap.Error(err))
	}
	return cloneSession(session)
}

func (s *Service) removeSessionLocked(sessionID string) {
	session := s.sessions[sessionID]
	if session == nil {
		return
	}
	delete(s.sessions, sessionID)
	if timer, ok := s.reconcileTimers[sessionID]; ok {
		timer.Stop()
		delete(s.reconcileTimers, sessionID)
	}
	if s.workspaceDirInUseLocked(session.WorkspaceDir) {
		return
	}
	s.removeWatchLocked(session.WorkspaceDir)
	if err := cleanupWorkspace(session.WorkspaceRoot, session.WorkspaceDir); err != nil {
		logger.Default().Warn("cleanup external edit workspace", zap.String("path", session.WorkspaceDir), zap.Error(err))
	}
}

func isExternalEditClipboardResidueSession(session *Session) bool {
	if session == nil {
		return false
	}
	return isExternalEditClipboardResidueText(session.DocumentKey) ||
		isExternalEditClipboardResidueText(session.RemotePath) ||
		isExternalEditClipboardResidueText(session.RemoteRealPath) ||
		isExternalEditClipboardResidueText(session.LocalPath) ||
		isExternalEditClipboardResidueText(session.WorkspaceDir)
}

func isExternalEditClipboardResidueText(value string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"))
	if normalized == "" {
		return false
	}
	for _, marker := range externalEditClipboardResidueMarkers {
		if strings.Contains(normalized, strings.ToLower(strings.ReplaceAll(marker, "\\", "/"))) {
			return true
		}
	}
	return false
}

func (s *Service) saveManifestLocked() error {
	s.normalizeDocumentFamiliesLocked()
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
