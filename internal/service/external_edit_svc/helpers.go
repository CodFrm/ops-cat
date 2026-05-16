package external_edit_svc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/opskat/opskat/internal/bootstrap"
	"github.com/opskat/opskat/internal/service/sftp_svc"
	"go.uber.org/zap"
)

func pickVisibleFamilyPrimarySession(family []*Session) *Session {
	var primary *Session
	var primaryRank int
	for _, session := range family {
		rank, ok := familyVisibleSessionRank(session)
		if !ok {
			continue
		}
		if primary == nil || rank < primaryRank || (rank == primaryRank && session.UpdatedAt > primary.UpdatedAt) {
			primary = session
			primaryRank = rank
		}
	}
	return primary
}

func familyVisibleSessionRank(session *Session) (int, bool) {
	if session == nil || session.Hidden {
		return 0, false
	}
	if session.RecordState == recordStateCompleted || session.RecordState == recordStateAbandoned {
		return 0, false
	}
	switch session.State {
	case sessionStateConflict, sessionStateRemoteMissing:
		return 0, true
	case sessionStateDirty:
		if session.RecordState == recordStateError {
			return 1, true
		}
		if session.ResumeRequired {
			return 2, true
		}
		return 3, true
	case sessionStateClean, sessionStateExpired:
		if session.RecordState == recordStateError {
			return 1, true
		}
		if session.ResumeRequired {
			return 2, true
		}
		return 3, true
	default:
		return 3, true
	}
}

func openSessionReuseRank(session *Session) (int, bool) {
	if session == nil || isExternalEditClipboardResidueSession(session) {
		return 0, false
	}
	if session.State == sessionStateStale {
		return 0, false
	}
	if session.Hidden {
		if isReusableClosedMainSession(session) {
			return 4, true
		}
		return 0, false
	}
	switch session.State {
	case sessionStateConflict, sessionStateRemoteMissing:
		return 0, true
	case sessionStateDirty:
		if session.RecordState == recordStateError {
			return 1, true
		}
		if session.ResumeRequired {
			return 2, true
		}
		return 3, true
	case sessionStateClean, sessionStateExpired:
		if session.RecordState == recordStateError {
			return 1, true
		}
		if session.ResumeRequired {
			return 2, true
		}
		return 3, true
	default:
		return 3, true
	}
}

func isReusableClosedMainSession(session *Session) bool {
	if session == nil {
		return false
	}
	return session.Hidden &&
		session.RecordState == recordStateAbandoned &&
		session.State != sessionStateStale &&
		strings.TrimSpace(session.SourceSessionID) == "" &&
		strings.TrimSpace(session.SupersededBySessionID) == ""
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

func canonicalRemotePath(info *sftp_svc.RemoteFileInfo, fallback string) string {
	if info != nil && strings.TrimSpace(info.RealPath) != "" {
		return info.RealPath
	}
	return fallback
}

func normalizeMaxReadFileSizeMB(value int) int {
	if value < minMaxReadFileSizeMB || value > maxMaxReadFileSizeMB {
		return defaultMaxReadFileSizeMB
	}
	return value
}

func MinMaxReadFileSizeMBForConfig() int {
	return minMaxReadFileSizeMB
}

func MaxMaxReadFileSizeMBForConfig() int {
	return maxMaxReadFileSizeMB
}

func MaxReadFileSizeBytesForConfig(cfg *bootstrap.AppConfig) int64 {
	if cfg == nil {
		return int64(defaultMaxReadFileSizeMB) * bytesPerMB
	}
	return int64(normalizeMaxReadFileSizeMB(cfg.ExternalEditMaxReadFileSizeMB)) * bytesPerMB
}

func (s *Service) maxReadFileSizeBytes() int64 {
	return MaxReadFileSizeBytesForConfig(s.configProvider())
}

func buildDocumentKey(assetID int64, canonicalRemoteFile string) string {
	return fmt.Sprintf("%d:%s", assetID, strings.TrimSpace(canonicalRemoteFile))
}

func isSyncSuppressedRecord(session *Session) bool {
	if session == nil {
		return false
	}
	if session.Hidden {
		return true
	}
	return session.RecordState == recordStateCompleted || session.RecordState == recordStateAbandoned
}

func remoteInfoHash(info *sftp_svc.RemoteFileInfo) string {
	if info == nil {
		return ""
	}
	return strings.TrimSpace(info.SHA256)
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

func readRemoteEditableFile(remote RemoteFileService, sessionID, remotePath string, maxReadBytes int64) ([]byte, *sftp_svc.RemoteFileInfo, error) {
	data, info, err := remote.ReadFile(sessionID, remotePath)
	if err != nil {
		return nil, nil, err
	}
	if info != nil && info.Size > maxReadBytes {
		return nil, nil, fmt.Errorf("远程文件过大，无法完整读取: %s (%d bytes > %d bytes)", remotePath, info.Size, maxReadBytes)
	}
	if int64(len(data)) > maxReadBytes {
		return nil, nil, fmt.Errorf("远程文件过大，无法完整读取: %s (%d bytes > %d bytes)", remotePath, len(data), maxReadBytes)
	}
	return data, info, nil
}

func readLocalEditableFile(localPath string, maxReadBytes int64) ([]byte, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("本地副本不是常规文件: %s (mode=%s, perm=%#o, isDir=%t)", localPath, info.Mode(), info.Mode().Perm(), info.IsDir())
	}
	if info.Size() > maxReadBytes {
		return nil, fmt.Errorf("本地副本过大，无法完整读取: %s (%d bytes > %d bytes)", localPath, info.Size(), maxReadBytes)
	}

	file, err := os.Open(localPath) //nolint:gosec // localPath is a managed external-edit workspace copy
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Default().Warn("close external edit local copy", zap.String("path", localPath), zap.Error(err))
		}
	}()

	data, err := io.ReadAll(io.LimitReader(file, maxReadBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxReadBytes {
		return nil, fmt.Errorf("本地副本读取过程中超过大小上限: %s (%d bytes > %d bytes)", localPath, len(data), maxReadBytes)
	}
	return data, nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func cloneSession(session *Session) *Session {
	if session == nil {
		return nil
	}
	cloned := *session
	cloned.EditorArgs = cloneArgs(session.EditorArgs)
	cloned.LastError = cloneErrorSnapshot(session.LastError)
	return &cloned
}

func cloneErrorSnapshot(snapshot *ErrorSnapshot) *ErrorSnapshot {
	if snapshot == nil {
		return nil
	}
	cloned := *snapshot
	return &cloned
}

func buildErrorSnapshot(step string, err error, nowUnix int64) *ErrorSnapshot {
	summary := "同步失败，请稍后重试"
	suggestion := externalEditReconnectHint
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "当前远程文件已不可访问"),
			strings.Contains(err.Error(), "无法确认仍是同一份远程文件"),
			strings.Contains(err.Error(), "当前副本已过期"):
			summary = "当前文件暂时无法继续同步"
			suggestion = externalEditReconnectHint
		case strings.Contains(err.Error(), "不可写"):
			summary = "远程文件暂时不可写"
			suggestion = "请先确认远程文件权限后再重试"
		case strings.Contains(err.Error(), "编码"),
			strings.Contains(err.Error(), "BOM"),
			strings.Contains(err.Error(), "文本文件"):
			summary = "当前本地副本已不满足安全同步条件"
			suggestion = "请恢复原始编码或重新打开该远程文件后再同步"
		case strings.Contains(err.Error(), "过大"),
			strings.Contains(err.Error(), "大小上限"),
			strings.Contains(err.Error(), "完整读取"):
			summary = "当前文件超过最大读取阈值，无法继续完整读取"
			suggestion = "请前往 设置 > External Edit 调整最大读取大小后再重试"
		case strings.Contains(err.Error(), "删除本地副本失败"):
			summary = "删除本地副本失败"
			suggestion = "请先关闭占用该文件的程序后再重试"
		}
	}
	return &ErrorSnapshot{
		Step:       step,
		Summary:    summary,
		Suggestion: suggestion,
		At:         nowUnix,
	}
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
