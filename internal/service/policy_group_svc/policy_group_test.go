package policy_group_svc

import (
	"context"
	"errors"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/opskat/opskat/internal/model/entity/policy_group_entity"
	"github.com/opskat/opskat/internal/repository/policy_group_repo"
	"github.com/opskat/opskat/internal/repository/policy_group_repo/mock_policy_group_repo"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func setupTest(t *testing.T) (context.Context, *mock_policy_group_repo.MockPolicyGroupRepo) {
	mockCtrl := gomock.NewController(t)
	t.Cleanup(func() { mockCtrl.Finish() })
	ctx := context.Background()
	mockRepo := mock_policy_group_repo.NewMockPolicyGroupRepo(mockCtrl)
	policy_group_repo.RegisterPolicyGroup(mockRepo)
	return ctx, mockRepo
}

func TestPolicyGroupSvc_List(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("列出权限组", t, func() {
		convey.Convey("不指定类型时返回所有内置组和用户自定义组", func() {
			userGroups := []*policy_group_entity.PolicyGroup{
				{ID: 1, Name: "自定义组", PolicyType: policy_group_entity.PolicyTypeCommand, Policy: "{}"},
			}
			mockRepo.EXPECT().List(gomock.Any()).Return(userGroups, nil)

			items, err := PolicyGroup().List(ctx, "")
			assert.NoError(t, err)

			// 应包含所有内置组 + 1 个用户自定义组
			builtinCount := len(policy_group_entity.BuiltinGroups())
			assert.Len(t, items, builtinCount+1)

			// 内置组标记 source=builtin
			for i := 0; i < builtinCount; i++ {
				assert.Equal(t, policy_group_entity.SourceBuiltin, items[i].Source)
				assert.True(t, policy.IsBuiltinID(items[i].ID))
			}
			// 用户自定义组标记 source=user
			assert.Equal(t, policy_group_entity.SourceUser, items[builtinCount].Source)
			assert.Equal(t, "自定义组", items[builtinCount].Name)
		})

		convey.Convey("按类型过滤仅返回对应类型的组", func() {
			userGroups := []*policy_group_entity.PolicyGroup{
				{ID: 2, Name: "SQL自定义", PolicyType: policy_group_entity.PolicyTypeQuery, Policy: "{}"},
			}
			mockRepo.EXPECT().ListByType(gomock.Any(), policy_group_entity.PolicyTypeQuery).Return(userGroups, nil)

			items, err := PolicyGroup().List(ctx, policy_group_entity.PolicyTypeQuery)
			assert.NoError(t, err)

			// 所有返回项的类型都应为 query
			for _, item := range items {
				assert.Equal(t, policy_group_entity.PolicyTypeQuery, item.PolicyType)
			}
			// 应包含内置 query 组 + 用户自定义组
			assert.True(t, len(items) >= 2) // 至少有 BuiltinSQLReadOnly + BuiltinSQLDangerousDeny
		})

		convey.Convey("DB查询失败时仍返回内置组", func() {
			mockRepo.EXPECT().List(gomock.Any()).Return(nil, errors.New("db error"))

			items, err := PolicyGroup().List(ctx, "")
			assert.NoError(t, err) // 不返回错误
			assert.Len(t, items, len(policy_group_entity.BuiltinGroups()))
		})
	})
}

func TestPolicyGroupSvc_Get(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("获取权限组", t, func() {
		convey.Convey("获取内置权限组成功", func() {
			item, err := PolicyGroup().Get(ctx, policy.BuiltinLinuxReadOnly)
			assert.NoError(t, err)
			assert.Equal(t, policy.BuiltinLinuxReadOnly, item.ID)
			assert.Equal(t, policy_group_entity.SourceBuiltin, item.Source)
			assert.Equal(t, "Linux 常用只读", item.Name)
		})

		convey.Convey("获取不存在的内置权限组返回错误", func() {
			_, err := PolicyGroup().Get(ctx, "builtin.nonexistent")
			assert.Error(t, err)
		})

		convey.Convey("获取用户自定义权限组成功", func() {
			expected := &policy_group_entity.PolicyGroup{
				ID: 10, Name: "我的组", PolicyType: policy_group_entity.PolicyTypeCommand, Policy: "{}",
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(10)).Return(expected, nil)

			item, err := PolicyGroup().Get(ctx, "10")
			assert.NoError(t, err)
			assert.Equal(t, policy_group_entity.SourceUser, item.Source)
			assert.Equal(t, "我的组", item.Name)
		})
	})
}

func TestPolicyGroupSvc_Create(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("创建权限组", t, func() {
		convey.Convey("创建合法权限组成功", func() {
			pg := &policy_group_entity.PolicyGroup{
				Name:       "测试组",
				PolicyType: policy_group_entity.PolicyTypeCommand,
				Policy:     `{"allow_list":["ls *"]}`,
			}
			mockRepo.EXPECT().Create(gomock.Any(), pg).Return(nil)

			err := PolicyGroup().Create(ctx, pg)
			assert.NoError(t, err)
			assert.Greater(t, pg.Createtime, int64(0))
			assert.Greater(t, pg.Updatetime, int64(0))
		})

		convey.Convey("名称为空时Validate拦截", func() {
			pg := &policy_group_entity.PolicyGroup{
				Name:       "",
				PolicyType: policy_group_entity.PolicyTypeCommand,
			}

			err := PolicyGroup().Create(ctx, pg)
			assert.Error(t, err)
		})

		convey.Convey("无效策略类型时Validate拦截", func() {
			pg := &policy_group_entity.PolicyGroup{
				Name:       "测试组",
				PolicyType: "invalid",
			}

			err := PolicyGroup().Create(ctx, pg)
			assert.Error(t, err)
		})
	})
}

func TestPolicyGroupSvc_Update(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("更新权限组", t, func() {
		convey.Convey("更新用户自定义组成功", func() {
			pg := &policy_group_entity.PolicyGroup{
				ID: 1, Name: "更新后", PolicyType: policy_group_entity.PolicyTypeCommand, Policy: "{}",
			}
			mockRepo.EXPECT().Update(gomock.Any(), pg).Return(nil)

			err := PolicyGroup().Update(ctx, pg)
			assert.NoError(t, err)
			assert.Greater(t, pg.Updatetime, int64(0))
		})

		convey.Convey("更新内置组被拒绝", func() {
			pg := &policy_group_entity.PolicyGroup{
				StringID: policy.BuiltinLinuxReadOnly, Name: "不允许", PolicyType: policy_group_entity.PolicyTypeCommand,
			}

			err := PolicyGroup().Update(ctx, pg)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "内置权限组不可修改")
		})
	})
}

func TestPolicyGroupSvc_Delete(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("删除权限组", t, func() {
		convey.Convey("删除用户自定义组成功", func() {
			mockRepo.EXPECT().Delete(gomock.Any(), int64(5)).Return(nil)

			err := PolicyGroup().Delete(ctx, "5")
			assert.NoError(t, err)
		})

		convey.Convey("删除内置组被拒绝", func() {
			err := PolicyGroup().Delete(ctx, policy.BuiltinLinuxReadOnly)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "内置权限组不可删除")
		})
	})
}

func TestPolicyGroupSvc_Copy(t *testing.T) {
	ctx, mockRepo := setupTest(t)

	convey.Convey("复制权限组", t, func() {
		convey.Convey("从内置组复制成功", func() {
			mockRepo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, pg *policy_group_entity.PolicyGroup) error {
					// 验证复制后的属性
					assert.Equal(t, "自定义名称", pg.Name)
					assert.Equal(t, policy_group_entity.PolicyTypeCommand, pg.PolicyType)
					assert.Greater(t, pg.Createtime, int64(0))
					// 验证 groups 字段已被移除
					assert.NotContains(t, pg.Policy, "groups")
					return nil
				},
			)

			result, err := PolicyGroup().Copy(ctx, policy.BuiltinLinuxReadOnly, "自定义名称")
			assert.NoError(t, err)
			assert.Equal(t, "自定义名称", result.Name)
			assert.Equal(t, policy_group_entity.PolicyTypeCommand, result.PolicyType)
		})

		convey.Convey("从内置组复制使用默认名称（副本）", func() {
			mockRepo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, pg *policy_group_entity.PolicyGroup) error {
					assert.Equal(t, "Linux 常用只读 (副本)", pg.Name)
					return nil
				},
			)

			result, err := PolicyGroup().Copy(ctx, policy.BuiltinLinuxReadOnly, "")
			assert.NoError(t, err)
			assert.Equal(t, "Linux 常用只读 (副本)", result.Name)
		})

		convey.Convey("从用户自定义组复制成功", func() {
			source := &policy_group_entity.PolicyGroup{
				ID: 10, Name: "源组", Description: "描述",
				PolicyType: policy_group_entity.PolicyTypeQuery,
				Policy:     `{"allow_types":["SELECT"],"groups":["builtin.sql_readonly"]}`,
			}
			mockRepo.EXPECT().Find(gomock.Any(), int64(10)).Return(source, nil)
			mockRepo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, pg *policy_group_entity.PolicyGroup) error {
					assert.Equal(t, "新名称", pg.Name)
					assert.Equal(t, "描述", pg.Description)
					// groups 应被移除
					assert.NotContains(t, pg.Policy, "groups")
					return nil
				},
			)

			result, err := PolicyGroup().Copy(ctx, "10", "新名称")
			assert.NoError(t, err)
			assert.Equal(t, "新名称", result.Name)
		})

		convey.Convey("复制不存在的内置组返回错误", func() {
			_, err := PolicyGroup().Copy(ctx, "builtin.nonexistent", "test")
			assert.Error(t, err)
		})
	})
}
