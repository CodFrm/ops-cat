package external_edit_svc

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

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
		if isSyncSuppressedRecord(session) {
			if timer, ok := s.reconcileTimers[id]; ok {
				timer.Stop()
				delete(s.reconcileTimers, id)
			}
			continue
		}
		if timer, ok := s.reconcileTimers[id]; ok {
			timer.Stop()
		}
		sessionID := id
		s.reconcileTimers[id] = time.AfterFunc(reconcileSettleDelay, func() {
			s.reconcileLocalCopy(sessionID)
		})
	}
}

func (s *Service) reconcileLocalCopy(sessionID string) {
	session := s.getSession(sessionID)
	if session == nil || isSyncSuppressedRecord(session) {
		return
	}
	if session.State == sessionStateConflict || session.State == sessionStateRemoteMissing {
		s.cancelAutoSaveForDocument(session.DocumentKey)
		return
	}

	data, err := readLocalEditableFile(session.LocalPath, s.maxReadFileSizeBytes())
	if err != nil {
		return
	}
	localHash := hashBytes(data)
	baseHash := sessionBaseHash(session)
	dirty := localHash != baseHash
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
	if isSyncSuppressedRecord(current) {
		s.mu.Unlock()
		s.cancelAutoSaveForDocument(session.DocumentKey)
		return
	}
	if current.State == sessionStateConflict || current.State == sessionStateRemoteMissing {
		s.mu.Unlock()
		s.cancelAutoSaveForDocument(session.DocumentKey)
		return
	}
	if sessionLocalHash(current) == localHash && current.Dirty == dirty && current.State == nextState {
		s.mu.Unlock()
		return
	}
	setSessionLocalHash(current, localHash)
	current.Dirty = dirty
	current.State = nextState
	current.PendingReview = nextState == sessionStateDirty && current.SaveMode == saveModeAutoLive
	if current.RecordState == "" || current.RecordState == recordStateCompleted || current.RecordState == recordStateAbandoned {
		current.RecordState = recordStateActive
	}
	current.Hidden = false
	current.UpdatedAt = s.now().Unix()
	err = s.saveManifestLocked()
	cloned := cloneSession(current)
	s.mu.Unlock()
	if err != nil {
		logger.Default().Warn("persist external edit manifest after local change", zap.Error(err))
		return
	}
	s.emit(Event{Type: eventSessionChanged, Session: cloned})
	if cloned.State == sessionStateDirty && cloned.SaveMode == saveModeAutoLive {
		s.scheduleAutoSave(cloned)
		return
	}
	s.cancelAutoSaveForDocument(cloned.DocumentKey)
}

func (s *Service) scheduleAutoSave(session *Session) {
	if session == nil || strings.TrimSpace(session.DocumentKey) == "" {
		return
	}
	attemptKey := session.DocumentKey + ":" + sessionLocalHash(session)

	s.mu.Lock()
	if s.autoSavePaused[session.DocumentKey] || s.autoSaveTried[session.DocumentKey] == attemptKey {
		s.mu.Unlock()
		return
	}
	if timer, ok := s.autoSaveTimers[session.DocumentKey]; ok {
		timer.Stop()
	}
	documentKey := session.DocumentKey
	primarySessionID := session.ID
	s.autoSaveTimers[documentKey] = time.AfterFunc(autoSaveDebounce, func() {
		s.runAutoSave(documentKey, primarySessionID, attemptKey)
	})
	s.mu.Unlock()
	s.emitAutoSavePhase(documentKey, primarySessionID, autoSavePhasePending, session)
}

func (s *Service) runAutoSave(documentKey, sessionID, attemptKey string) {
	if strings.TrimSpace(documentKey) == "" {
		return
	}
	defer s.emitAutoSavePhase(documentKey, sessionID, autoSavePhaseIdle, nil)

	session := s.getSession(sessionID)
	if session == nil || session.DocumentKey != documentKey {
		return
	}

	s.mu.Lock()
	if current, ok := s.autoSaveTimers[documentKey]; ok && current != nil {
		delete(s.autoSaveTimers, documentKey)
	}
	currentSession := s.sessions[sessionID]
	if currentSession == nil || isSyncSuppressedRecord(currentSession) {
		if !s.autoSavePaused[documentKey] {
			delete(s.autoSaveTried, documentKey)
		}
		s.mu.Unlock()
		return
	}
	if s.autoSavePaused[documentKey] || s.autoSaveTried[documentKey] == attemptKey {
		s.mu.Unlock()
		return
	}
	s.autoSaveTried[documentKey] = attemptKey
	runningSession := cloneSession(currentSession)
	s.mu.Unlock()

	s.emitAutoSavePhase(documentKey, sessionID, autoSavePhaseRunning, runningSession)
	var result *SaveResult
	err := s.withDocumentRunner(sessionID, func() error {
		var saveErr error
		result, saveErr = s.saveInternal(context.Background(), sessionID, "", true)
		return saveErr
	})
	if err != nil {
		logger.Default().Warn("auto save external edit document failed", zap.String("documentKey", documentKey), zap.Error(err))
		s.pauseAutoSaveForDocument(documentKey)
		return
	}
	if result == nil {
		return
	}
	if result.Status == saveStatusConflict || result.Status == saveStatusRemoteMissing {
		s.pauseAutoSaveForDocument(documentKey)
	}
}

func (s *Service) pauseAutoSaveForDocument(documentKey string) {
	documentKey = strings.TrimSpace(documentKey)
	if documentKey == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoSavePaused[documentKey] = true
	if timer, ok := s.autoSaveTimers[documentKey]; ok {
		timer.Stop()
		delete(s.autoSaveTimers, documentKey)
	}
	s.emitAutoSavePhase(documentKey, "", autoSavePhaseIdle, nil)
}

func (s *Service) resumeAutoSaveForDocument(documentKey string) {
	documentKey = strings.TrimSpace(documentKey)
	if documentKey == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.autoSavePaused, documentKey)
	delete(s.autoSaveTried, documentKey)
}

func (s *Service) cancelAutoSaveForDocument(documentKey string) {
	documentKey = strings.TrimSpace(documentKey)
	if documentKey == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if timer, ok := s.autoSaveTimers[documentKey]; ok {
		timer.Stop()
		delete(s.autoSaveTimers, documentKey)
	}
	if !s.autoSavePaused[documentKey] {
		delete(s.autoSaveTried, documentKey)
	}
	s.emitAutoSavePhase(documentKey, "", autoSavePhaseIdle, nil)
}

func (s *Service) emitAutoSavePhase(documentKey, sessionID, phase string, session *Session) {
	documentKey = strings.TrimSpace(documentKey)
	phase = strings.TrimSpace(phase)
	if documentKey == "" || phase == "" {
		return
	}

	event := Event{
		Type:    eventSessionAutoSave,
		Session: cloneSession(session),
		AutoSave: &AutoSaveStatus{
			DocumentKey: documentKey,
			SessionID:   strings.TrimSpace(sessionID),
			Phase:       phase,
		},
	}
	s.emit(event)
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
