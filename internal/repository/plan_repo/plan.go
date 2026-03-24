package plan_repo

import (
	"context"
	"time"

	"github.com/opskat/opskat/internal/model/entity/plan_entity"

	"github.com/cago-frame/cago/database/db"
)

// PlanRepo 计划审批数据访问接口
type PlanRepo interface {
	CreateSession(ctx context.Context, session *plan_entity.PlanSession) error
	GetSession(ctx context.Context, id string) (*plan_entity.PlanSession, error)
	UpdateSessionStatus(ctx context.Context, id string, status int) error
	CreateItems(ctx context.Context, items []*plan_entity.PlanItem) error
	UpdateItems(ctx context.Context, sessionID string, items []*plan_entity.PlanItem) error
	ListItems(ctx context.Context, sessionID string) ([]*plan_entity.PlanItem, error)
	// ListApprovedItems 获取某个会话下所有已批准 plan 的 items
	ListApprovedItems(ctx context.Context, sessionID string) ([]*plan_entity.PlanItem, error)
	// ConsumeItem 原子消费一个匹配的 plan item，返回是否成功消费
	ConsumeItem(ctx context.Context, sessionID, toolName string, assetID int64, command string, auditLogID int64) (bool, error)
}

var defaultPlan PlanRepo

// Plan 获取 PlanRepo 实例
func Plan() PlanRepo {
	return defaultPlan
}

// RegisterPlan 注册 PlanRepo 实现
func RegisterPlan(i PlanRepo) {
	defaultPlan = i
}

// planRepo 默认实现
type planRepo struct{}

// NewPlan 创建默认实现
func NewPlan() PlanRepo {
	return &planRepo{}
}

func (r *planRepo) CreateSession(ctx context.Context, session *plan_entity.PlanSession) error {
	return db.Ctx(ctx).Create(session).Error
}

func (r *planRepo) GetSession(ctx context.Context, id string) (*plan_entity.PlanSession, error) {
	var session plan_entity.PlanSession
	if err := db.Ctx(ctx).Where("id = ?", id).First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *planRepo) UpdateSessionStatus(ctx context.Context, id string, status int) error {
	return db.Ctx(ctx).Model(&plan_entity.PlanSession{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     status,
			"updatetime": time.Now().Unix(),
		}).Error
}

func (r *planRepo) CreateItems(ctx context.Context, items []*plan_entity.PlanItem) error {
	if len(items) == 0 {
		return nil
	}
	return db.Ctx(ctx).Create(items).Error
}

func (r *planRepo) ListItems(ctx context.Context, sessionID string) ([]*plan_entity.PlanItem, error) {
	var items []*plan_entity.PlanItem
	if err := db.Ctx(ctx).Where("plan_session_id = ?", sessionID).
		Order("item_index ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *planRepo) UpdateItems(ctx context.Context, sessionID string, items []*plan_entity.PlanItem) error {
	// 删除旧 items 并重建
	if err := db.Ctx(ctx).Where("plan_session_id = ?", sessionID).Delete(&plan_entity.PlanItem{}).Error; err != nil {
		return err
	}
	if len(items) > 0 {
		return db.Ctx(ctx).Create(items).Error
	}
	return nil
}

func (r *planRepo) ListApprovedItems(ctx context.Context, sessionID string) ([]*plan_entity.PlanItem, error) {
	var items []*plan_entity.PlanItem
	// 查找该 sessionID 关联的所有已批准 plan 的 items
	if err := db.Ctx(ctx).
		Joins("JOIN plan_sessions ON plan_sessions.id = plan_items.plan_session_id").
		Where("plan_sessions.status = ? AND plan_items.plan_session_id = ?",
			plan_entity.PlanStatusApproved, sessionID).
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *planRepo) ConsumeItem(ctx context.Context, sessionID, toolName string, assetID int64, command string, auditLogID int64) (bool, error) {
	result := db.Ctx(ctx).Model(&plan_entity.PlanItem{}).
		Where("plan_session_id = ? AND tool_name = ? AND asset_id = ? AND command = ? AND consumed = 0",
			sessionID, toolName, assetID, command).
		Limit(1).
		Updates(map[string]any{
			"consumed":     1,
			"consumed_at":  time.Now().Unix(),
			"audit_log_id": auditLogID,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
