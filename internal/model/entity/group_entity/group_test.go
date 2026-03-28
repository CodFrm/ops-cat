package group_entity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"

	"github.com/opskat/opskat/internal/model/entity/policy"
)

func TestGroup_Validate(t *testing.T) {
	Convey("分组校验", t, func() {
		Convey("名称为空时应返回错误", func() {
			g := &Group{}
			err := g.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "名称")
		})

		Convey("名称非空时校验通过", func() {
			g := &Group{Name: "测试分组"}
			err := g.Validate()
			assert.NoError(t, err)
		})
	})
}

func TestGroup_IsRoot(t *testing.T) {
	Convey("是否为顶层分组", t, func() {
		Convey("ParentID为0时是顶层分组", func() {
			g := &Group{Name: "根分组", ParentID: 0}
			assert.True(t, g.IsRoot())
		})

		Convey("ParentID大于0时不是顶层分组", func() {
			g := &Group{Name: "子分组", ParentID: 1}
			assert.False(t, g.IsRoot())
		})
	})
}

func TestGroup_CommandPolicy(t *testing.T) {
	Convey("命令权限策略序列化与反序列化", t, func() {
		Convey("设置策略后可正确读取", func() {
			g := &Group{Name: "测试分组"}
			p := &policy.CommandPolicy{
				AllowList: []string{"ls", "pwd"},
				DenyList:  []string{"rm -rf"},
				Groups:    []string{policy.BuiltinLinuxReadOnly, policy.BuiltinDangerousDeny},
			}
			err := g.SetCommandPolicy(p)
			assert.NoError(t, err)

			got, err := g.GetCommandPolicy()
			assert.NoError(t, err)
			assert.Equal(t, p.AllowList, got.AllowList)
			assert.Equal(t, p.DenyList, got.DenyList)
			assert.Equal(t, p.Groups, got.Groups)
		})

		Convey("设置空策略时清空字段", func() {
			g := &Group{Name: "测试分组"}
			// 先设置一个非空策略
			err := g.SetCommandPolicy(&policy.CommandPolicy{
				AllowList: []string{"ls"},
			})
			assert.NoError(t, err)
			assert.NotEmpty(t, g.CmdPolicy)

			// 设置空策略应清空字段
			err = g.SetCommandPolicy(&policy.CommandPolicy{})
			assert.NoError(t, err)
			assert.Empty(t, g.CmdPolicy)
		})

		Convey("字段为空时GetCommandPolicy返回零值", func() {
			g := &Group{Name: "测试分组"}
			got, err := g.GetCommandPolicy()
			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.Empty(t, got.AllowList)
			assert.Empty(t, got.DenyList)
			assert.Empty(t, got.Groups)
		})
	})
}

func TestGroup_QueryPolicy(t *testing.T) {
	Convey("SQL权限策略序列化与反序列化", t, func() {
		Convey("设置策略后可正确读取", func() {
			g := &Group{Name: "测试分组"}
			p := &policy.QueryPolicy{
				AllowTypes: []string{"SELECT", "SHOW"},
				DenyTypes:  []string{"DROP"},
				DenyFlags:  []string{"no_where_delete"},
				Groups:     []string{policy.BuiltinSQLReadOnly},
			}
			err := g.SetQueryPolicy(p)
			assert.NoError(t, err)

			got, err := g.GetQueryPolicy()
			assert.NoError(t, err)
			assert.Equal(t, p.AllowTypes, got.AllowTypes)
			assert.Equal(t, p.DenyTypes, got.DenyTypes)
			assert.Equal(t, p.DenyFlags, got.DenyFlags)
			assert.Equal(t, p.Groups, got.Groups)
		})

		Convey("设置空策略时清空字段", func() {
			g := &Group{Name: "测试分组"}
			err := g.SetQueryPolicy(&policy.QueryPolicy{
				AllowTypes: []string{"SELECT"},
			})
			assert.NoError(t, err)
			assert.NotEmpty(t, g.QryPolicy)

			err = g.SetQueryPolicy(&policy.QueryPolicy{})
			assert.NoError(t, err)
			assert.Empty(t, g.QryPolicy)
		})

		Convey("字段为空时GetQueryPolicy返回零值", func() {
			g := &Group{Name: "测试分组"}
			got, err := g.GetQueryPolicy()
			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.Empty(t, got.AllowTypes)
			assert.Empty(t, got.DenyTypes)
			assert.Empty(t, got.DenyFlags)
			assert.Empty(t, got.Groups)
		})
	})
}

func TestGroup_RedisPolicy(t *testing.T) {
	Convey("Redis权限策略序列化与反序列化", t, func() {
		Convey("设置策略后可正确读取", func() {
			g := &Group{Name: "测试分组"}
			p := &policy.RedisPolicy{
				AllowList: []string{"GET", "SET"},
				DenyList:  []string{"CONFIG SET", "FLUSHALL"},
				Groups:    []string{policy.BuiltinRedisReadOnly, policy.BuiltinRedisDangerousDeny},
			}
			err := g.SetRedisPolicy(p)
			assert.NoError(t, err)

			got, err := g.GetRedisPolicy()
			assert.NoError(t, err)
			assert.Equal(t, p.AllowList, got.AllowList)
			assert.Equal(t, p.DenyList, got.DenyList)
			assert.Equal(t, p.Groups, got.Groups)
		})

		Convey("设置空策略时清空字段", func() {
			g := &Group{Name: "测试分组"}
			err := g.SetRedisPolicy(&policy.RedisPolicy{
				AllowList: []string{"GET"},
			})
			assert.NoError(t, err)
			assert.NotEmpty(t, g.RdsPolicy)

			err = g.SetRedisPolicy(&policy.RedisPolicy{})
			assert.NoError(t, err)
			assert.Empty(t, g.RdsPolicy)
		})

		Convey("字段为空时GetRedisPolicy返回零值", func() {
			g := &Group{Name: "测试分组"}
			got, err := g.GetRedisPolicy()
			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.Empty(t, got.AllowList)
			assert.Empty(t, got.DenyList)
			assert.Empty(t, got.Groups)
		})
	})
}
