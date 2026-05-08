package local_tool_grant_repo

//go:generate mockgen -source=local_tool_grant.go -destination=mock_local_tool_grant_repo/local_tool_grant.go -package=mock_local_tool_grant_repo

import (
	"context"
	"errors"
	"time"

	"github.com/cago-frame/cago/database/db"
	"gorm.io/gorm"

	"github.com/opskat/opskat/internal/model/entity/local_tool_grant_entity"
)

// LocalToolGrantRepo 暴露会话级"始终放行"开关的查/写。
// 调用语义：Has 命中即视为允许；Save 是 upsert。
type LocalToolGrantRepo interface {
	Has(ctx context.Context, sessionID, toolName string) (bool, error)
	Save(ctx context.Context, sessionID, toolName string) error
	ListBySession(ctx context.Context, sessionID string) ([]string, error)
}

var defaultRepo LocalToolGrantRepo

// LocalToolGrant 获取已注册实现。bootstrap 阶段未注册时返回 nil。
func LocalToolGrant() LocalToolGrantRepo { return defaultRepo }

// Register 注册实现。
func Register(r LocalToolGrantRepo) { defaultRepo = r }

type localToolGrantRepo struct{}

// New 默认实现。
func New() LocalToolGrantRepo { return &localToolGrantRepo{} }

func (r *localToolGrantRepo) Has(ctx context.Context, sessionID, toolName string) (bool, error) {
	if sessionID == "" || toolName == "" {
		return false, nil
	}
	var row local_tool_grant_entity.LocalToolGrant
	err := db.Ctx(ctx).Where("session_id = ? AND tool_name = ?", sessionID, toolName).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *localToolGrantRepo) Save(ctx context.Context, sessionID, toolName string) error {
	if sessionID == "" || toolName == "" {
		return nil
	}
	row := &local_tool_grant_entity.LocalToolGrant{
		SessionID:  sessionID,
		ToolName:   toolName,
		Createtime: time.Now().Unix(),
	}
	// SQLite 拿不到 ON CONFLICT helper，直接靠唯一索引 + IgnoreDuplicate。
	err := db.Ctx(ctx).Create(row).Error
	if err == nil {
		return nil
	}
	// 唯一键冲突就当成功——本来就是幂等放行。
	exists, hasErr := r.Has(ctx, sessionID, toolName)
	if hasErr == nil && exists {
		return nil
	}
	return err
}

func (r *localToolGrantRepo) ListBySession(ctx context.Context, sessionID string) ([]string, error) {
	if sessionID == "" {
		return nil, nil
	}
	var rows []local_tool_grant_entity.LocalToolGrant
	if err := db.Ctx(ctx).
		Where("session_id = ?", sessionID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ToolName)
	}
	return out, nil
}
