package policy_group_entity

import (
	"testing"

	"github.com/opskat/opskat/internal/model/entity/policy"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestPolicyGroup_Validate(t *testing.T) {
	convey.Convey("权限组校验", t, func() {
		convey.Convey("名称为空时应返回错误", func() {
			pg := &PolicyGroup{PolicyType: PolicyTypeCommand}
			err := pg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "名称")
		})

		convey.Convey("无效的策略类型应返回错误", func() {
			pg := &PolicyGroup{Name: "test", PolicyType: "unknown"}
			err := pg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "策略类型")
		})

		convey.Convey("command类型校验通过", func() {
			pg := &PolicyGroup{Name: "test", PolicyType: PolicyTypeCommand}
			err := pg.Validate()
			assert.NoError(t, err)
		})

		convey.Convey("query类型校验通过", func() {
			pg := &PolicyGroup{Name: "test", PolicyType: PolicyTypeQuery}
			err := pg.Validate()
			assert.NoError(t, err)
		})

		convey.Convey("redis类型校验通过", func() {
			pg := &PolicyGroup{Name: "test", PolicyType: PolicyTypeRedis}
			err := pg.Validate()
			assert.NoError(t, err)
		})
	})
}

func TestPolicyGroup_ToItem(t *testing.T) {
	convey.Convey("ToItem转换", t, func() {
		convey.Convey("内置组转换后ID为字符串且Builtin为true", func() {
			pg := &PolicyGroup{
				BuiltinID:   policy.BuiltinLinuxReadOnly,
				Name:        "Linux Read-Only",
				Description: "Common Linux read-only commands",
				PolicyType:  PolicyTypeCommand,
				Policy:      `{}`,
			}
			item := pg.ToItem()
			assert.Equal(t, policy.BuiltinLinuxReadOnly, item.ID)
			assert.Equal(t, pg.Name, item.Name)
			assert.Equal(t, pg.Description, item.Description)
			assert.Equal(t, pg.PolicyType, item.PolicyType)
			assert.Equal(t, pg.Policy, item.Policy)
			assert.True(t, item.Builtin)
		})

		convey.Convey("用户组转换后ID为数字字符串且Builtin为false", func() {
			pg := &PolicyGroup{
				ID:         1,
				Name:       "用户组",
				PolicyType: PolicyTypeQuery,
				Policy:     `{}`,
			}
			item := pg.ToItem()
			assert.False(t, item.Builtin)
			assert.Equal(t, "1", item.ID)
		})
	})
}

func TestIsBuiltinID(t *testing.T) {
	convey.Convey("IsBuiltinID检查", t, func() {
		convey.Convey("builtin:前缀为内置", func() {
			assert.True(t, IsBuiltinID("builtin:linux-readonly"))
			assert.True(t, IsBuiltinID("builtin:k8s-readonly"))
		})

		convey.Convey("空字符串不是内置", func() {
			assert.False(t, IsBuiltinID(""))
		})

		convey.Convey("数字字符串不是内置", func() {
			assert.False(t, IsBuiltinID("1"))
			assert.False(t, IsBuiltinID("100"))
		})
	})
}

func TestFindBuiltin(t *testing.T) {
	convey.Convey("FindBuiltin查找内置权限组", t, func() {
		convey.Convey("按已知ID查找应返回对应内置组", func() {
			pg := FindBuiltin(policy.BuiltinLinuxReadOnly)
			assert.NotNil(t, pg)
			assert.Equal(t, policy.BuiltinLinuxReadOnly, pg.BuiltinID)
			assert.Equal(t, PolicyTypeCommand, pg.PolicyType)
		})

		convey.Convey("按不存在的ID查找应返回nil", func() {
			pg := FindBuiltin("builtin:nonexistent")
			assert.Nil(t, pg)
		})

		convey.Convey("数字字符串查找应返回nil", func() {
			pg := FindBuiltin("1")
			assert.Nil(t, pg)
		})
	})
}

func TestBuiltinGroups(t *testing.T) {
	convey.Convey("BuiltinGroups内置权限组列表", t, func() {
		groups := BuiltinGroups()

		convey.Convey("共返回12个内置组", func() {
			assert.Len(t, groups, 12)
		})

		convey.Convey("所有内置组ID均以builtin:开头", func() {
			for _, g := range groups {
				assert.True(t, IsBuiltinID(g.BuiltinID), "内置组ID应以builtin:开头，实际ID=%s", g.BuiltinID)
			}
		})

		convey.Convey("command类型内置组有5个", func() {
			var count int
			for _, g := range groups {
				if g.PolicyType == PolicyTypeCommand {
					count++
				}
			}
			assert.Equal(t, 5, count)
		})

		convey.Convey("query类型内置组有2个", func() {
			var count int
			for _, g := range groups {
				if g.PolicyType == PolicyTypeQuery {
					count++
				}
			}
			assert.Equal(t, 2, count)
		})

		convey.Convey("redis类型内置组有2个", func() {
			var count int
			for _, g := range groups {
				if g.PolicyType == PolicyTypeRedis {
					count++
				}
			}
			assert.Equal(t, 2, count)
		})

		convey.Convey("mongo类型内置组有3个", func() {
			var count int
			for _, g := range groups {
				if g.PolicyType == PolicyTypeMongo {
					count++
				}
			}
			assert.Equal(t, 3, count)
		})
	})
}
