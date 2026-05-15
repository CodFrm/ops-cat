package external_edit_svc

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
)

func (s *Service) startCleanupLoop() {
	s.mu.Lock()
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}
	s.cleanupTicker = time.NewTicker(24 * time.Hour)
	ticker := s.cleanupTicker
	s.mu.Unlock()

	go func() {
		for {
			select {
			case <-s.closeCh:
				return
			case <-ticker.C:
				s.runRetentionCleanup()
			}
		}
	}()
}

func (s *Service) runRetentionCleanup() {
	retentionDays := s.cleanupRetentionDays()
	retentionCutoff := s.now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	cleanupBefore := s.now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
	var cleaned []string
	var retentionTargets []workspaceTarget

	s.mu.Lock()
	for id, session := range s.sessions {
		if session == nil {
			delete(s.sessions, id)
			continue
		}
		if session.UpdatedAt > cleanupBefore {
			continue
		}
		if !canCleanupRetainedSession(session) {
			continue
		}
		cleaned = append(cleaned, id)
		s.removeSessionLocked(id)
	}
	retentionTargets = s.collectWorkspaceTargetsLocked()
	if err := s.saveManifestLocked(); err != nil {
		logger.Default().Warn("cleanup external edit retention manifest", zap.Error(err))
	}
	s.mu.Unlock()
	s.cleanupBakeupRetention(retentionTargets, retentionCutoff)

	for _, id := range cleaned {
		s.emit(Event{Type: eventSessionCleaned, Session: &Session{ID: id}})
	}
}

func (s *Service) deleteDocumentFamilyAndWorkspace(documentKey string) error {
	s.mu.Lock()
	documentKey = strings.TrimSpace(documentKey)
	if documentKey == "" {
		s.mu.Unlock()
		return fmt.Errorf("外部编辑会话不存在")
	}
	family := s.documentFamilySessionsLocked(documentKey)
	if len(family) == 0 {
		s.mu.Unlock()
		return fmt.Errorf("外部编辑会话不存在")
	}
	type workspaceTarget struct {
		root string
		dir  string
	}
	targets := make([]workspaceTarget, 0, len(family))
	seen := make(map[string]struct{})
	for _, session := range family {
		if session == nil || session.WorkspaceDir == "" {
			continue
		}
		if _, ok := seen[session.WorkspaceDir]; ok {
			continue
		}
		seen[session.WorkspaceDir] = struct{}{}
		targets = append(targets, workspaceTarget{root: session.WorkspaceRoot, dir: session.WorkspaceDir})
	}
	s.mu.Unlock()

	for _, target := range targets {
		if err := cleanupWorkspace(target.root, target.dir); err != nil {
			return fmt.Errorf("删除本地副本失败，请先关闭编辑器或手动清理后再重试")
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range family {
		if session == nil {
			continue
		}
		s.removeWatchLocked(session.WorkspaceDir)
		delete(s.sessions, session.ID)
		if timer, ok := s.reconcileTimers[session.ID]; ok {
			timer.Stop()
			delete(s.reconcileTimers, session.ID)
		}
	}
	if err := s.saveManifestLocked(); err != nil {
		return err
	}
	return nil
}

func (s *Service) workspaceDirInUseLocked(workspaceDir string) bool {
	workspaceDir = strings.TrimSpace(workspaceDir)
	if workspaceDir == "" {
		return false
	}
	for _, session := range s.sessions {
		if session == nil {
			continue
		}
		if strings.TrimSpace(session.WorkspaceDir) == workspaceDir {
			return true
		}
	}
	return false
}

type workspaceTarget struct {
	root string
	dir  string
}

func (s *Service) collectWorkspaceTargetsLocked() []workspaceTarget {
	targets := make([]workspaceTarget, 0, len(s.sessions))
	seen := make(map[string]struct{}, len(s.sessions))
	for _, session := range s.sessions {
		if session == nil {
			continue
		}
		workspaceDir := strings.TrimSpace(session.WorkspaceDir)
		if workspaceDir == "" {
			continue
		}
		if _, ok := seen[workspaceDir]; ok {
			continue
		}
		seen[workspaceDir] = struct{}{}
		targets = append(targets, workspaceTarget{
			root: strings.TrimSpace(session.WorkspaceRoot),
			dir:  workspaceDir,
		})
	}
	return targets
}

func (s *Service) cleanupBakeupRetention(targets []workspaceTarget, cutoff time.Time) {
	for _, target := range targets {
		if err := cleanupBakeupEntries(target.root, target.dir, cutoff); err != nil {
			logger.Default().Warn(
				"cleanup external edit bakeup retention",
				zap.String("workspaceDir", target.dir),
				zap.Error(err),
			)
		}
	}
}

func (s *Service) documentFamilySessionsLocked(documentKey string) []*Session {
	documentKey = strings.TrimSpace(documentKey)
	if documentKey == "" {
		return nil
	}
	sessions := make([]*Session, 0, 4)
	for _, session := range s.sessions {
		if session == nil || session.DocumentKey != documentKey || isExternalEditClipboardResidueSession(session) {
			continue
		}
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt > sessions[j].UpdatedAt
	})
	return sessions
}

func buildWorkspacePaths(workspaceRoot string, assetID int64, remotePath string) (string, string, error) {
	safeRemote := sanitizeRemotePath(remotePath)
	if safeRemote == "" {
		return "", "", fmt.Errorf("无法构建本地临时副本路径")
	}
	hashPrefix := shortHash(remotePath)
	workspaceDir := filepath.Join(workspaceRoot, "workspaces", fmt.Sprintf("asset-%d", assetID), hashPrefix, filepath.Dir(safeRemote))
	localPath := filepath.Join(workspaceDir, filepath.Base(safeRemote))
	return localPath, workspaceDir, nil
}

func bakeupDirForWorkspace(workspaceDir string) string {
	return filepath.Join(workspaceDir, "bakeup")
}

func cleanupBakeupEntries(workspaceRoot, workspaceDir string, cutoff time.Time) error {
	if strings.TrimSpace(workspaceRoot) == "" || strings.TrimSpace(workspaceDir) == "" {
		return nil
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return err
	}
	workspace, err := filepath.Abs(workspaceDir)
	if err != nil {
		return err
	}
	bakeupDir, err := filepath.Abs(bakeupDirForWorkspace(workspaceDir))
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	workspace = filepath.Clean(workspace)
	bakeupDir = filepath.Clean(bakeupDir)
	if workspace != root && !strings.HasPrefix(workspace, root+string(os.PathSeparator)) {
		return fmt.Errorf("workspace dir escapes workspace root")
	}
	if bakeupDir != workspace && !strings.HasPrefix(bakeupDir, workspace+string(os.PathSeparator)) {
		return fmt.Errorf("bakeup dir escapes workspace dir")
	}
	entries, err := os.ReadDir(bakeupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(bakeupDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) moveFileToBakeup(workspaceRoot, workspaceDir, localPath string) (string, error) {
	if strings.TrimSpace(localPath) == "" {
		return "", nil
	}
	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	bakeupDir := bakeupDirForWorkspace(workspaceDir)
	if err := os.MkdirAll(bakeupDir, 0o700); err != nil {
		return "", fmt.Errorf("创建 bakeup 目录失败: %w", err)
	}
	baseName := filepath.Base(localPath)
	ext := filepath.Ext(baseName)
	nameOnly := strings.TrimSuffix(baseName, ext)
	timePart := s.now().Format("20060102-150405")
	for index := 0; index < 1000; index++ {
		candidate := filepath.Join(bakeupDir, fmt.Sprintf("%s-%s-%03d%s", nameOnly, timePart, index, ext))
		if _, err := os.Stat(candidate); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.Rename(localPath, candidate); err != nil {
			return "", fmt.Errorf("移动旧本地副本到 bakeup 失败: %w", err)
		}
		return candidate, nil
	}
	return "", fmt.Errorf("bakeup 目录中候选文件已耗尽")
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

func normalizeCleanupRetentionDays(days int) int {
	if days < minCleanupRetentionDays || days > maxCleanupRetentionDays {
		return defaultCleanupRetentionDays
	}
	return days
}

func (s *Service) cleanupRetentionDays() int {
	cfg := s.configProvider()
	if cfg == nil {
		return defaultCleanupRetentionDays
	}
	return normalizeCleanupRetentionDays(cfg.ExternalEditCleanupRetentionDays)
}

func canCleanupRetainedSession(session *Session) bool {
	if session == nil {
		return false
	}
	if session.State == sessionStateConflict || session.State == sessionStateRemoteMissing {
		return false
	}
	if session.RecordState == recordStateError {
		return false
	}
	if session.ResumeRequired {
		return false
	}
	return true
}
