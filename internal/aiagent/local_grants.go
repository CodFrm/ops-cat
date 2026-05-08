package aiagent

import (
	"context"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"github.com/opskat/opskat/internal/repository/local_tool_grant_repo"
)

// repoLocalGrantStore 是 LocalGrantStore 的生产实现：直接读 / 写
// local_tool_grant_repo。Has 错误时按 false 处理，让用户重新看到弹卡而不是
// 因为读库失败被静默 deny。
type repoLocalGrantStore struct{}

// NewRepoLocalGrantStore 返回基于 local_tool_grant_repo.LocalToolGrant() 的
// LocalGrantStore。bootstrap 必须先调用 local_tool_grant_repo.Register。
func NewRepoLocalGrantStore() LocalGrantStore { return &repoLocalGrantStore{} }

func (s *repoLocalGrantStore) Has(ctx context.Context, sessionID, toolName string) bool {
	r := local_tool_grant_repo.LocalToolGrant()
	if r == nil {
		return false
	}
	ok, err := r.Has(ctx, sessionID, toolName)
	if err != nil {
		logger.Default().Warn("local-tool grant lookup failed",
			zap.String("sessionID", sessionID), zap.String("tool", toolName), zap.Error(err))
		return false
	}
	return ok
}

func (s *repoLocalGrantStore) Save(ctx context.Context, sessionID, toolName string) {
	r := local_tool_grant_repo.LocalToolGrant()
	if r == nil {
		return
	}
	if err := r.Save(ctx, sessionID, toolName); err != nil {
		logger.Default().Warn("local-tool grant save failed",
			zap.String("sessionID", sessionID), zap.String("tool", toolName), zap.Error(err))
	}
}
