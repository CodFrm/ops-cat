package aiagent

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/opskat/opskat/internal/repository/local_tool_grant_repo"
	"github.com/opskat/opskat/internal/repository/local_tool_grant_repo/mock_local_tool_grant_repo"
)

// resetGrantRepo 复位全局 defaultRepo，避免污染同包后续 test。
func resetGrantRepo() func() {
	prev := local_tool_grant_repo.LocalToolGrant()
	return func() { local_tool_grant_repo.Register(prev) }
}

// TestRepoLocalGrantStore_HasReturnsFalseWhenRepoUnregistered 守住 bootstrap 顺序失败
// （Register 还没调）的兜底：repo 为 nil 时 Has 必须返回 false，绝不能 panic。
// 返 false 的语义是"重新走审批流程"，比 true（静默放行）安全。
func TestRepoLocalGrantStore_HasReturnsFalseWhenRepoUnregistered(t *testing.T) {
	defer resetGrantRepo()()
	local_tool_grant_repo.Register(nil)

	s := NewRepoLocalGrantStore()
	if got := s.Has(context.Background(), "conv_1", "write"); got {
		t.Errorf("Has on nil repo: got true, want false")
	}
}

func TestRepoLocalGrantStore_HasReturnsTrueOnHit(t *testing.T) {
	defer resetGrantRepo()()
	ctrl := gomock.NewController(t)
	mock := mock_local_tool_grant_repo.NewMockLocalToolGrantRepo(ctrl)
	local_tool_grant_repo.Register(mock)

	mock.EXPECT().Has(gomock.Any(), "conv_1", "write").Return(true, nil)
	s := NewRepoLocalGrantStore()
	if got := s.Has(context.Background(), "conv_1", "write"); !got {
		t.Errorf("Has hit: got false, want true")
	}
}

// TestRepoLocalGrantStore_HasDegradesOnRepoErrorToFalse 是核心安全约束：
// repo 读失败时不能让模型 / 工具静默放行。返 false → 走审批流程，符合
// local_grants.go 注释里"按 false 处理，让用户重新看到弹卡而不是因为读库失败被静默 deny"。
func TestRepoLocalGrantStore_HasDegradesOnRepoErrorToFalse(t *testing.T) {
	defer resetGrantRepo()()
	ctrl := gomock.NewController(t)
	mock := mock_local_tool_grant_repo.NewMockLocalToolGrantRepo(ctrl)
	local_tool_grant_repo.Register(mock)

	mock.EXPECT().Has(gomock.Any(), "conv_2", "edit").Return(false, errors.New("db down"))
	s := NewRepoLocalGrantStore()
	if got := s.Has(context.Background(), "conv_2", "edit"); got {
		t.Errorf("Has on repo error: got true, want false (must degrade)")
	}
}

func TestRepoLocalGrantStore_SaveNilRepoIsNoop(t *testing.T) {
	defer resetGrantRepo()()
	local_tool_grant_repo.Register(nil)

	s := NewRepoLocalGrantStore()
	// 不返错、不 panic 即可——Save 是 fire-and-forget。
	s.Save(context.Background(), "conv_1", "write")
}

func TestRepoLocalGrantStore_SaveSwallowsRepoError(t *testing.T) {
	defer resetGrantRepo()()
	ctrl := gomock.NewController(t)
	mock := mock_local_tool_grant_repo.NewMockLocalToolGrantRepo(ctrl)
	local_tool_grant_repo.Register(mock)

	mock.EXPECT().Save(gomock.Any(), "conv_3", "edit").Return(errors.New("disk full"))
	s := NewRepoLocalGrantStore()
	// Save 必须吞错（仅 warn 日志）——否则 hook 会把"无法持久化"放大成"deny"，
	// 用户体验是"我点了 allowAll 但仍 deny"。
	s.Save(context.Background(), "conv_3", "edit")
}

func TestRepoLocalGrantStore_SaveSucceeds(t *testing.T) {
	defer resetGrantRepo()()
	ctrl := gomock.NewController(t)
	mock := mock_local_tool_grant_repo.NewMockLocalToolGrantRepo(ctrl)
	local_tool_grant_repo.Register(mock)

	mock.EXPECT().Save(gomock.Any(), "conv_4", "write").Return(nil)
	s := NewRepoLocalGrantStore()
	s.Save(context.Background(), "conv_4", "write")
}
