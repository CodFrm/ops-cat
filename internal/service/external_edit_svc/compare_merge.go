package external_edit_svc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) rereadRemoteSession(sessionID string) (*SaveResult, error) {
	current := s.getSession(sessionID)
	if current == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	if err := s.guardMutableSession(current); err != nil {
		return nil, err
	}
	transport, transportErr := s.resolveDocumentTransport(current)
	if transportErr != nil {
		s.writeAudit(current, "external_edit_document_transport_blocked", false, map[string]any{"resolution": resolutionReread}, nil, transportErr)
		return nil, transportErr
	}
	current, err := s.bindSessionTransport(sessionID, transport)
	if err != nil {
		return nil, err
	}

	if _, _, err := readRemoteEditableFile(s.remote, current.SessionID, current.RemotePath); err != nil {
		if isRemoteMissingError(err) {
			result := s.markSessionState(sessionID, sessionStateRemoteMissing, true, sessionLocalHash(current))
			saveResult := &SaveResult{
				Status:   saveStatusRemoteMissing,
				Message:  "远程文件不存在，请先确认是否需要重新创建远程文件",
				Session:  result,
				Conflict: s.describeConflict(result, ""),
			}
			s.pauseAutoSaveForDocument(result.DocumentKey)
			s.writeAudit(result, "external_edit_conflict_remote_missing", true, map[string]any{"resolution": resolutionReread}, saveResult, nil)
			s.emit(Event{Type: eventSessionConflict, Session: result, SaveResult: saveResult})
			return saveResult, nil
		}
		return nil, fmt.Errorf("重新读取远程文件失败: %w", err)
	}

	rebuilt, err := s.rebuildDocumentSessionFromRemote(
		OpenRequest{
			AssetID:    current.AssetID,
			SessionID:  current.SessionID,
			RemotePath: current.RemotePath,
			EditorID:   current.EditorID,
		},
		Editor{
			ID:   current.EditorID,
			Name: current.EditorName,
			Path: current.EditorPath,
			Args: cloneArgs(current.EditorArgs),
		},
		current.AssetName,
		current,
		"external_edit_reread",
	)
	if err != nil {
		return nil, err
	}

	saveResult := &SaveResult{
		Status:   saveStatusReread,
		Message:  "已接收远程新版本，并以远程新基线重建当前草稿",
		Session:  rebuilt,
		Conflict: s.describeConflict(rebuilt, ""),
	}
	s.resumeAutoSaveForDocument(rebuilt.DocumentKey)
	return saveResult, nil
}

func (s *Service) Compare(sessionID string) (*CompareResult, error) {
	var result *CompareResult
	err := s.withDocumentRunner(sessionID, func() error {
		var compareErr error
		result, compareErr = s.compareInternal(sessionID)
		return compareErr
	})
	return result, err
}

func (s *Service) compareInternal(sessionID string) (*CompareResult, error) {
	current := s.getSession(sessionID)
	if current == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	conflict := s.describeConflict(current, "")
	if conflict == nil {
		return nil, fmt.Errorf("当前文件没有待比对的冲突版本")
	}

	primary := s.getSession(conflict.PrimaryDraftSessionID)
	if primary == nil {
		return nil, fmt.Errorf("冲突草稿不存在")
	}
	if primary.State != sessionStateConflict && primary.State != sessionStateStale {
		return nil, fmt.Errorf("当前文件没有待比对的冲突版本")
	}

	var snapshot *Session
	if conflict.LatestSnapshotSessionID != "" {
		snapshot = s.getSession(conflict.LatestSnapshotSessionID)
	}
	if snapshot == nil {
		snapshot = primary
	}

	transport, err := s.resolveDocumentTransport(primary)
	if err != nil {
		s.writeAudit(primary, "external_edit_compare", false, nil, nil, err)
		return nil, err
	}
	primary, err = s.bindSessionTransport(primary.ID, transport)
	if err != nil {
		return nil, err
	}

	remoteData, remoteInfo, err := readRemoteEditableFile(s.remote, primary.SessionID, primary.RemotePath)
	if err != nil {
		if isRemoteMissingError(err) {
			saveResult := s.markRemoteMissingConflict(primary.ID, primary, sessionLocalHash(primary), false, "", "compare")
			return &CompareResult{
				DocumentKey:           primary.DocumentKey,
				PrimaryDraftSessionID: primary.ID,
				FileName:              filepath.Base(primary.RemotePath),
				RemotePath:            primary.RemotePath,
				ReadOnly:              true,
				Status:                saveResult.Status,
				Message:               saveResult.Message,
				Session:               saveResult.Session,
				Conflict:              saveResult.Conflict,
			}, nil
		}
		return nil, fmt.Errorf("读取远程文件失败: %w", err)
	}
	if remoteInfo.IsDir || !remoteInfo.Regular {
		return nil, fmt.Errorf("远程路径已不是常规文件")
	}
	if !sameRemoteIdentity(primary, remoteInfo, primary.RemotePath) {
		return nil, fmt.Errorf("当前文件位置已变化，无法确认仍是同一份远程文件；%s", externalEditReconnectHint)
	}
	if !isLikelyText(primary.RemotePath, remoteData) {
		return nil, fmt.Errorf("当前远程文件不是可编辑文本文件")
	}
	if _, err := detectTextEncoding(remoteData); err != nil {
		return nil, fmt.Errorf("当前远程文件编码暂不支持比对: %w", err)
	}
	if err := validateRoundTrip(primary, remoteData); err != nil {
		return nil, err
	}

	localData, err := readLocalEditableFile(primary.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("读取本地副本失败: %w", err)
	}
	if !isLikelyText(primary.RemotePath, localData) {
		return nil, fmt.Errorf("本地副本已不是可编辑文本文件")
	}
	if err := validateRoundTrip(primary, localData); err != nil {
		return nil, err
	}

	result := &CompareResult{
		DocumentKey:             primary.DocumentKey,
		PrimaryDraftSessionID:   primary.ID,
		LatestSnapshotSessionID: snapshot.ID,
		FileName:                filepath.Base(primary.RemotePath),
		RemotePath:              primary.RemotePath,
		LocalContent:            string(localData),
		RemoteContent:           string(remoteData),
		ReadOnly:                true,
	}
	s.writeAudit(primary, "external_edit_compare", true, nil, map[string]any{"documentKey": primary.DocumentKey, "readOnly": true}, nil)
	return result, nil
}

func (s *Service) PrepareMerge(sessionID string) (*MergePrepareResult, error) {
	var result *MergePrepareResult
	err := s.withDocumentRunner(sessionID, func() error {
		var prepareErr error
		result, prepareErr = s.prepareMergeInternal(sessionID)
		return prepareErr
	})
	return result, err
}

func (s *Service) prepareMergeInternal(sessionID string) (*MergePrepareResult, error) {
	current := s.getSession(sessionID)
	if current == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	if current.State != sessionStateConflict {
		return nil, fmt.Errorf("当前文件没有可合并的远程冲突")
	}

	transport, err := s.resolveDocumentTransport(current)
	if err != nil {
		s.writeAudit(current, "external_edit_merge_prepare", false, nil, nil, err)
		return nil, err
	}
	current, err = s.bindSessionTransport(sessionID, transport)
	if err != nil {
		return nil, err
	}

	remoteData, remoteInfo, err := readRemoteEditableFile(s.remote, current.SessionID, current.RemotePath)
	if err != nil {
		if isRemoteMissingError(err) {
			saveResult := s.markRemoteMissingConflict(current.ID, current, sessionLocalHash(current), false, "", "merge_prepare")
			return nil, errors.New(saveResult.Message)
		}
		return nil, fmt.Errorf("读取远程文件失败: %w", err)
	}
	if remoteInfo.IsDir || !remoteInfo.Regular {
		return nil, fmt.Errorf("远程路径已不是常规文件")
	}
	if !sameRemoteIdentity(current, remoteInfo, current.RemotePath) {
		return nil, fmt.Errorf("当前文件位置已变化，无法确认仍是同一份远程文件；%s", externalEditReconnectHint)
	}
	if !isLikelyText(current.RemotePath, remoteData) {
		return nil, fmt.Errorf("当前远程文件不是可编辑文本文件")
	}
	if _, err := detectTextEncoding(remoteData); err != nil {
		return nil, fmt.Errorf("当前远程文件编码暂不支持合并: %w", err)
	}
	if err := validateRoundTrip(current, remoteData); err != nil {
		return nil, err
	}

	localData, err := readLocalEditableFile(current.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("读取本地副本失败: %w", err)
	}
	if !isLikelyText(current.RemotePath, localData) {
		return nil, fmt.Errorf("本地副本已不是可编辑文本文件")
	}
	if err := validateRoundTrip(current, localData); err != nil {
		return nil, err
	}

	remoteHash := hashBytes(remoteData)
	updated := s.setMergeRemoteHash(sessionID, remoteHash)
	result := &MergePrepareResult{
		DocumentKey:           current.DocumentKey,
		PrimaryDraftSessionID: current.ID,
		FileName:              filepath.Base(current.RemotePath),
		RemotePath:            current.RemotePath,
		LocalContent:          string(localData),
		RemoteContent:         string(remoteData),
		FinalContent:          string(localData),
		RemoteHash:            remoteHash,
		Session:               updated,
	}
	s.writeAudit(current, "external_edit_merge_prepare", true, nil, map[string]any{"documentKey": current.DocumentKey}, nil)
	return result, nil
}

func (s *Service) ApplyMerge(ctx context.Context, req MergeApplyRequest) (*SaveResult, error) {
	var result *SaveResult
	err := s.withDocumentRunner(req.SessionID, func() error {
		var applyErr error
		result, applyErr = s.applyMergeInternal(ctx, req)
		return applyErr
	})
	return result, err
}

func (s *Service) applyMergeInternal(ctx context.Context, req MergeApplyRequest) (*SaveResult, error) {
	session := s.getSession(req.SessionID)
	if session == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	if session.State != sessionStateConflict {
		return nil, fmt.Errorf("当前文件没有可合并的远程冲突")
	}
	if strings.TrimSpace(req.RemoteHash) == "" {
		return nil, fmt.Errorf("缺少合并基线，请重新打开合并窗口")
	}
	if session.MergeRemoteSHA256 != "" && session.MergeRemoteSHA256 != req.RemoteHash {
		return nil, fmt.Errorf("合并基线已过期，请重新比对远程新版本")
	}
	finalData := []byte(req.FinalContent)
	if !isLikelyText(session.RemotePath, finalData) {
		mergeErr := fmt.Errorf("最终稿已不是可编辑文本文件")
		failed := s.recordError(req.SessionID, "merge_validate_final", mergeErr)
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, mergeErr
	}
	if err := validateRoundTrip(session, finalData); err != nil {
		failed := s.recordError(req.SessionID, "merge_validate_final", err)
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, err
	}

	transport, transportErr := s.resolveDocumentTransport(session)
	if transportErr != nil {
		s.writeAudit(session, "external_edit_merge_apply", false, nil, nil, transportErr)
		failed := s.recordError(req.SessionID, "merge_resolve_transport", transportErr)
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, transportErr
	}
	session, err := s.bindSessionTransport(req.SessionID, transport)
	if err != nil {
		return nil, err
	}

	remoteData, remoteInfo, err := readRemoteEditableFile(s.remote, session.SessionID, session.RemotePath)
	if err != nil {
		if isRemoteMissingError(err) {
			return s.markRemoteMissingConflict(req.SessionID, session, sessionLocalHash(session), false, "", "merge_apply"), nil
		}
		mergeErr := fmt.Errorf("读取远程文件失败: %w", err)
		failed := s.recordError(req.SessionID, "merge_read_remote", mergeErr)
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, mergeErr
	}
	if remoteInfo.IsDir || !remoteInfo.Regular {
		return nil, fmt.Errorf("远程路径已不是常规文件")
	}
	if hashBytes(remoteData) != req.RemoteHash {
		result := s.markSessionState(req.SessionID, sessionStateConflict, true, sessionLocalHash(session))
		saveResult := &SaveResult{
			Status:   saveStatusConflict,
			Message:  "远程文件在合并期间再次变化，请重新比对后再合并",
			Session:  result,
			Conflict: s.describeConflict(result, ""),
		}
		s.pauseAutoSaveForDocument(result.DocumentKey)
		s.writeAudit(result, "external_edit_merge_stale_remote", true, map[string]any{"remoteBytes": len(remoteData)}, saveResult, nil)
		s.emit(Event{Type: eventSessionConflict, Session: result, SaveResult: saveResult})
		return saveResult, nil
	}

	if err := os.WriteFile(session.LocalPath, finalData, 0o600); err != nil {
		mergeErr := fmt.Errorf("写入本地最终稿失败: %w", err)
		failed := s.recordError(req.SessionID, "merge_write_local", mergeErr)
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, mergeErr
	}
	updatedLocalHash := hashBytes(finalData)
	s.markSessionState(req.SessionID, sessionStateDirty, true, updatedLocalHash)
	result, err := s.saveInternal(ctx, req.SessionID, resolutionOverwrite, false)
	if err != nil {
		return nil, err
	}
	if result != nil && result.Status == saveStatusSaved {
		s.clearMergeRemoteHash(req.SessionID)
		s.writeAudit(result.Session, "external_edit_merge_apply", true, map[string]any{"bytes": len(finalData)}, result, nil)
	}
	return result, nil
}

func (s *Service) Recover(sessionID string) (*Session, error) {
	var result *Session
	err := s.withDocumentRunner(sessionID, func() error {
		var recoverErr error
		result, recoverErr = s.recoverInternal(sessionID)
		return recoverErr
	})
	return result, err
}

func (s *Service) Continue(sessionID string) (*Session, error) {
	var result *Session
	err := s.withDocumentRunner(sessionID, func() error {
		var continueErr error
		result, continueErr = s.continueInternal(sessionID)
		return continueErr
	})
	return result, err
}

func (s *Service) recoverInternal(sessionID string) (*Session, error) {
	session := s.getSession(sessionID)
	if session == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	if session.Hidden || session.RecordState == recordStateCompleted || session.RecordState == recordStateAbandoned {
		return nil, fmt.Errorf("当前记录已归档，不能恢复")
	}
	if _, err := os.Stat(session.LocalPath); err != nil {
		failed := s.recordError(sessionID, "recover_local_copy", fmt.Errorf("本地恢复副本不存在: %w", err))
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, fmt.Errorf("本地恢复副本不存在，请重新打开远程文件")
	}
	if err := s.launch.Launch(session.EditorPath, append(cloneArgs(session.EditorArgs), session.LocalPath)); err != nil {
		failed := s.recordError(sessionID, "recover_launch_editor", err)
		if failed != nil {
			s.emit(Event{Type: eventSessionChanged, Session: failed})
		}
		return nil, fmt.Errorf("启动外部编辑器失败: %w", err)
	}
	updated := s.markResumeRequired(sessionID, true)
	s.writeAudit(updated, "external_edit_recover", true, nil, updated, nil)
	s.emit(Event{Type: eventSessionRestored, Session: updated})
	return updated, nil
}

func (s *Service) continueInternal(sessionID string) (*Session, error) {
	session := s.getSession(sessionID)
	if session == nil {
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	if session.Hidden || session.RecordState == recordStateCompleted || session.RecordState == recordStateAbandoned {
		return nil, fmt.Errorf("当前记录已归档，不能继续编辑")
	}

	s.mu.Lock()
	current := s.sessions[sessionID]
	if current == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("外部编辑会话不存在")
	}
	current.PendingReview = false
	current.UpdatedAt = s.now().Unix()
	if err := s.saveManifestLocked(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	updated := cloneSession(current)
	s.mu.Unlock()

	s.writeAudit(updated, "external_edit_continue", true, nil, updated, nil)
	s.emit(Event{Type: eventSessionChanged, Session: updated})
	return updated, nil
}
