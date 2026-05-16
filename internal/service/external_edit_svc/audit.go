package external_edit_svc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/cago-frame/cago/pkg/logger"
	"github.com/opskat/opskat/internal/model/entity/audit_entity"
	"github.com/opskat/opskat/internal/repository/audit_repo"
	"go.uber.org/zap"
)

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
		SessionID:  session.ID,
		Createtime: s.now().Unix(),
	}
	// desktop 审计既要给 QA/SEC 还原状态机，又不能把本地工作区路径、编辑器安装路径等敏感环境信息带进数据库。
	if err := repo.Create(context.Background(), entry); err != nil {
		logger.Default().Warn("write external edit audit log", zap.Error(err))
	}
}

// recordAutoSaveAudit 对自动保存成功采样：
// 每 autoSaveAuditWindow 窗口只写一条汇总记录（含窗口内成功次数），
// 失败由调用方直接走 writeAudit 写详细记录，不经过此函数。
func (s *Service) recordAutoSaveAudit(session *Session, toolName string, result *SaveResult) {
	if session == nil {
		return
	}
	key := session.DocumentKey
	now := s.now().Unix()

	s.mu.Lock()
	c := s.autoSaveCounters[key]
	if c == nil {
		c = &autoSaveCounter{}
		s.autoSaveCounters[key] = c
	}
	c.count++
	windowExpired := now-c.lastAt >= int64(autoSaveAuditWindow.Seconds())
	count := c.count
	if windowExpired {
		c.count = 0
		c.lastAt = now
	}
	s.mu.Unlock()

	if !windowExpired {
		return
	}
	s.writeAudit(session, toolName, true,
		map[string]any{"auto": true, "windowSaves": count},
		result, nil)
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
		DocumentKey:           session.DocumentKey,
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
		RecordState:           session.RecordState,
		SaveMode:              session.SaveMode,
		Hidden:                session.Hidden,
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
