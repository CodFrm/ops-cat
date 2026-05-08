package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/model/entity/audit_entity"
	"github.com/opskat/opskat/internal/repository/asset_repo"
	"github.com/opskat/opskat/internal/repository/audit_repo"
)

// --- Context keys ---

type auditSourceKey struct{}
type conversationIDKey struct{}
type grantSessionIDKey struct{}
type sessionIDKey struct{}

// WithAuditSource 注入审计来源
func WithAuditSource(ctx context.Context, source string) context.Context {
	return context.WithValue(ctx, auditSourceKey{}, source)
}

// GetAuditSource 获取审计来源
func GetAuditSource(ctx context.Context) string {
	if v, ok := ctx.Value(auditSourceKey{}).(string); ok {
		return v
	}
	return ""
}

// WithConversationID 注入会话 ID
func WithConversationID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, conversationIDKey{}, id)
}

// GetConversationID 获取会话 ID
func GetConversationID(ctx context.Context) int64 {
	if v, ok := ctx.Value(conversationIDKey{}).(int64); ok {
		return v
	}
	return 0
}

// WithGrantSessionID 注入授权会话 ID
func WithGrantSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, grantSessionIDKey{}, id)
}

// GetGrantSessionID 获取授权会话 ID
func GetGrantSessionID(ctx context.Context) string {
	if v, ok := ctx.Value(grantSessionIDKey{}).(string); ok {
		return v
	}
	return ""
}

// WithSessionID 注入会话 ID（opsctl session 或 AI session）
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, id)
}

// GetSessionID 获取会话 ID
func GetSessionID(ctx context.Context) string {
	if v, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return v
	}
	return ""
}

// --- AuditWriter 接口 ---

// ToolCallInfo 一次工具调用的完整信息
type ToolCallInfo struct {
	ToolName string
	ArgsJSON string
	Result   string
	Error    error
	Decision *CheckResult // 可选，权限检查结果
}

// AuditWriter 审计日志写入接口
type AuditWriter interface {
	WriteToolCall(ctx context.Context, info ToolCallInfo)
}

// DefaultAuditWriter 默认审计日志写入实现
type DefaultAuditWriter struct{}

// NewDefaultAuditWriter 创建默认审计写入器
func NewDefaultAuditWriter() *DefaultAuditWriter {
	return &DefaultAuditWriter{}
}

// WriteToolCall 写入一次工具调用的审计日志
func (w *DefaultAuditWriter) WriteToolCall(ctx context.Context, info ToolCallInfo) {
	var args map[string]any
	if err := json.Unmarshal([]byte(info.ArgsJSON), &args); err != nil {
		logger.Default().Warn("unmarshal audit args", zap.Error(err))
	}

	assetID := argInt64(args, "asset_id")
	if assetID == 0 {
		assetID = argInt64(args, "id")
	}

	assetName := ""
	if assetID > 0 && asset_repo.Asset() != nil {
		if a, err := asset_repo.Asset().Find(context.Background(), assetID); err == nil {
			assetName = a.Name
		}
	}

	command := ExtractCommandForAudit(info.ToolName, args)

	success := 1
	errMsg := ""
	if info.Error != nil {
		success = 0
		errMsg = info.Error.Error()
	}

	entry := &audit_entity.AuditLog{
		Source:         GetAuditSource(ctx),
		ToolName:       info.ToolName,
		AssetID:        assetID,
		AssetName:      assetName,
		Command:        command,
		Request:        truncateString(info.ArgsJSON, 4096),
		Result:         truncateString(info.Result, 32768),
		Error:          errMsg,
		Success:        success,
		ConversationID: GetConversationID(ctx),
		GrantSessionID: GetGrantSessionID(ctx),
		SessionID:      GetSessionID(ctx),
		Createtime:     time.Now().Unix(),
	}

	// 填充决策信息
	if info.Decision != nil && info.Decision.DecisionSource != "" {
		entry.Decision = info.Decision.DecisionString()
		entry.DecisionSource = info.Decision.DecisionSource
		entry.MatchedPattern = info.Decision.MatchedPattern
	}

	if repo := audit_repo.Audit(); repo != nil {
		if err := repo.Create(context.Background(), entry); err != nil {
			logger.Default().Error("audit log write failed", zap.Error(err))
		}
	}
}

// --- 会话模式审计 ---

// writeGrantSubmitAudit 记录会话级"始终允许"模式变更（内部使用）
func writeGrantSubmitAudit(ctx context.Context, assetID int64, assetName string, patterns []string) {
	if repo := audit_repo.Audit(); repo != nil {
		entry := &audit_entity.AuditLog{
			Source:     GetAuditSource(ctx),
			ToolName:   "grant_submit",
			AssetID:    assetID,
			AssetName:  assetName,
			Command:    strings.Join(patterns, ", "),
			SessionID:  GetSessionID(ctx),
			Decision:   "allow",
			Success:    1,
			Createtime: time.Now().Unix(),
		}
		if err := repo.Create(context.Background(), entry); err != nil {
			logger.Default().Error("write grant submit audit", zap.Error(err))
		}
	}
}

// WriteGrantSubmitAudit 对外暴露的 grant 审计写入（供桌面端 approval handler 使用）
func WriteGrantSubmitAudit(ctx context.Context, assetID int64, assetName string, patterns []string, sessionID string) {
	if repo := audit_repo.Audit(); repo != nil {
		entry := &audit_entity.AuditLog{
			Source:     "opsctl",
			ToolName:   "grant_submit",
			AssetID:    assetID,
			AssetName:  assetName,
			Command:    strings.Join(patterns, ", "),
			SessionID:  sessionID,
			Decision:   "allow",
			Success:    1,
			Createtime: time.Now().Unix(),
		}
		if err := repo.Create(context.Background(), entry); err != nil {
			logger.Default().Error("write grant submit audit", zap.Error(err))
		}
	}
}

// --- 命令提取 ---

// commandExtractors 从 AllToolDefs 构建的命令提取器映射（延迟初始化）
var commandExtractors map[string]CommandExtractorFunc

// getCommandExtractors 获取命令提取器映射（线程安全，AllToolDefs 返回固定值）
func getCommandExtractors() map[string]CommandExtractorFunc {
	if commandExtractors == nil {
		m := make(map[string]CommandExtractorFunc)
		for _, def := range AllToolDefs() {
			if def.CommandExtractor != nil {
				m[def.Name] = def.CommandExtractor
			}
		}
		commandExtractors = m
	}
	return commandExtractors
}

// ExtractCommandForAudit 从工具参数中提取命令信息
func ExtractCommandForAudit(toolName string, args map[string]any) string {
	extractors := getCommandExtractors()
	// 支持 opsctl 使用 "exec" 作为 tool name
	if toolName == "exec" {
		toolName = "run_command"
	}
	if fn, ok := extractors[toolName]; ok {
		return fn(args)
	}
	return ""
}

// --- 辅助函数 ---

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n...[truncated]"
}

// LimitedBuffer 限制大小的缓冲区，用于审计日志捕获输出
type LimitedBuffer struct {
	buf   bytes.Buffer
	limit int
}

// NewLimitedBuffer 创建限制大小的缓冲区
func NewLimitedBuffer(limit int) *LimitedBuffer {
	return &LimitedBuffer{limit: limit}
}

func (b *LimitedBuffer) Write(p []byte) (int, error) {
	n := len(p) // 始终返回原始长度，避免 io.MultiWriter 报 ErrShortWrite
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		return n, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	b.buf.Write(p)
	return n, nil
}

// String 返回缓冲区内容
func (b *LimitedBuffer) String() string {
	return b.buf.String()
}
