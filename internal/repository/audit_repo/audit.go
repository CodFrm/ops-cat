package audit_repo

import (
	"context"

	"github.com/opskat/opskat/internal/model/entity/audit_entity"

	"github.com/cago-frame/cago/database/db"
)

// SessionInfo 会话摘要信息
type SessionInfo struct {
	SessionID string `json:"session_id"`
	FirstTime int64  `json:"first_time"`
	LastTime  int64  `json:"last_time"`
	Count     int64  `json:"count"`
}

// AuditRepo 审计日志数据访问接口
type AuditRepo interface {
	Create(ctx context.Context, log *audit_entity.AuditLog) error
	List(ctx context.Context, opts ListOptions) ([]*audit_entity.AuditLog, int64, error)
	ListSessions(ctx context.Context, startTime int64) ([]SessionInfo, error)
}

// ListOptions 列表查询选项
type ListOptions struct {
	Source         string
	AssetID        int64
	ConversationID int64
	SessionID      string
	StartTime      int64 // unix timestamp, 0 表示不限制
	EndTime        int64 // unix timestamp, 0 表示不限制
	Offset         int
	Limit          int
}

var defaultAudit AuditRepo

// Audit 获取 AuditRepo 实例
func Audit() AuditRepo {
	return defaultAudit
}

// RegisterAudit 注册 AuditRepo 实现
func RegisterAudit(i AuditRepo) {
	defaultAudit = i
}

// auditRepo 默认实现
type auditRepo struct{}

// NewAudit 创建默认实现
func NewAudit() AuditRepo {
	return &auditRepo{}
}

func (r *auditRepo) Create(ctx context.Context, log *audit_entity.AuditLog) error {
	return db.Ctx(ctx).Create(log).Error
}

func (r *auditRepo) List(ctx context.Context, opts ListOptions) ([]*audit_entity.AuditLog, int64, error) {
	var logs []*audit_entity.AuditLog
	var total int64

	query := db.Ctx(ctx).Model(&audit_entity.AuditLog{})
	if opts.Source != "" {
		query = query.Where("source = ?", opts.Source)
	}
	if opts.AssetID > 0 {
		query = query.Where("asset_id = ?", opts.AssetID)
	}
	if opts.ConversationID > 0 {
		query = query.Where("conversation_id = ?", opts.ConversationID)
	}
	if opts.SessionID != "" {
		query = query.Where("session_id = ?", opts.SessionID)
	}
	if opts.StartTime > 0 {
		query = query.Where("createtime >= ?", opts.StartTime)
	}
	if opts.EndTime > 0 {
		query = query.Where("createtime <= ?", opts.EndTime)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}

	if err := query.Order("id DESC").Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (r *auditRepo) ListSessions(ctx context.Context, startTime int64) ([]SessionInfo, error) {
	var sessions []SessionInfo
	query := db.Ctx(ctx).Model(&audit_entity.AuditLog{}).
		Select("session_id, MIN(createtime) as first_time, MAX(createtime) as last_time, COUNT(*) as count").
		Where("session_id != ''")
	if startTime > 0 {
		query = query.Where("createtime >= ?", startTime)
	}
	if err := query.Group("session_id").Order("last_time DESC").Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}
