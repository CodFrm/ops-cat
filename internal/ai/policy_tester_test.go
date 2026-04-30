package ai

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	"github.com/opskat/opskat/internal/model/entity/group_entity"
	"github.com/opskat/opskat/internal/model/entity/policy"
	. "github.com/smartystreets/goconvey/convey"
)

func makeGroup(name, cmdPolicyJSON string) *group_entity.Group {
	return &group_entity.Group{Name: name, CmdPolicy: cmdPolicyJSON}
}

func TestTestSSHPolicy(t *testing.T) {
	ctx := context.Background()

	Convey("testSSHPolicy", t, func() {
		Convey("无策略时返回 NeedConfirm", func() {
			out := testSSHPolicy(ctx, nil, nil, "ls -la")
			So(out.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("资产 allow 规则匹配", func() {
			p := &asset_entity.CommandPolicy{AllowList: []string{"ls *"}}
			out := testSSHPolicy(ctx, p, nil, "ls -la")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "")
		})

		Convey("资产 deny 规则匹配", func() {
			p := &asset_entity.CommandPolicy{DenyList: []string{"curl *"}}
			out := testSSHPolicy(ctx, p, nil, "curl http://example.com")
			So(out.Decision, ShouldEqual, Deny)
			So(out.MatchedSource, ShouldEqual, "")
		})

		Convey("引用内置权限组 — 高危拒绝", func() {
			p := &asset_entity.CommandPolicy{
				Groups: []string{policy.BuiltinDangerousDeny},
			}
			out := testSSHPolicy(ctx, p, nil, "rm -rf /tmp")
			So(out.Decision, ShouldEqual, Deny)
		})

		Convey("引用内置权限组 — Linux 只读允许", func() {
			p := &asset_entity.CommandPolicy{
				Groups: []string{policy.BuiltinLinuxReadOnly},
			}
			out := testSSHPolicy(ctx, p, nil, "ls -la")
			So(out.Decision, ShouldEqual, Allow)
		})

		Convey("引用组 + 内联规则共存", func() {
			p := &asset_entity.CommandPolicy{
				AllowList: []string{"my-custom-cmd *"},
				Groups:    []string{policy.BuiltinLinuxReadOnly, policy.BuiltinDangerousDeny},
			}
			out := testSSHPolicy(ctx, p, nil, "my-custom-cmd foo")
			So(out.Decision, ShouldEqual, Allow)

			out = testSSHPolicy(ctx, p, nil, "ls -la")
			So(out.Decision, ShouldEqual, Allow)

			out = testSSHPolicy(ctx, p, nil, "rm -rf /tmp")
			So(out.Decision, ShouldEqual, Deny)
		})

		Convey("组通用策略 deny 匹配", func() {
			groups := []*group_entity.Group{
				makeGroup("生产组", `{"deny_list":["GET *"]}`),
			}
			out := testSSHPolicy(ctx, nil, groups, "GET user")
			So(out.Decision, ShouldEqual, Deny)
			So(out.MatchedSource, ShouldEqual, "生产组")
			So(out.MatchedPattern, ShouldEqual, "GET *")
		})

		Convey("组通用策略 allow 匹配", func() {
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["kubectl get *"]}`),
			}
			out := testSSHPolicy(ctx, nil, groups, "kubectl get pods")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "dev组")
		})

		Convey("资产 deny 优先于组 allow", func() {
			p := &asset_entity.CommandPolicy{DenyList: []string{"kubectl *"}}
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["kubectl get *"]}`),
			}
			out := testSSHPolicy(ctx, p, groups, "kubectl get pods")
			So(out.Decision, ShouldEqual, Deny)
			So(out.MatchedSource, ShouldEqual, "")
		})

		Convey("默认策略（引用内置组）正确生效", func() {
			p := policy.DefaultCommandPolicy()
			out := testSSHPolicy(ctx, p, nil, "ls -la")
			So(out.Decision, ShouldEqual, Allow)

			out = testSSHPolicy(ctx, p, nil, "rm -rf /tmp")
			So(out.Decision, ShouldEqual, Deny)

			out = testSSHPolicy(ctx, p, nil, "vim /etc/config")
			So(out.Decision, ShouldEqual, NeedConfirm)
		})
	})
}

func TestTestRedisPolicy(t *testing.T) {
	ctx := context.Background()

	Convey("testRedisPolicy", t, func() {
		Convey("无策略时 auto-allow", func() {
			out := testRedisPolicy(ctx, nil, nil, "GET user:1")
			So(out.Decision, ShouldEqual, Allow)
		})

		Convey("引用内置组 — 拒绝 FLUSHDB", func() {
			p := &asset_entity.RedisPolicy{
				Groups: []string{policy.BuiltinRedisDangerousDeny},
			}
			out := testRedisPolicy(ctx, p, nil, "FLUSHDB")
			So(out.Decision, ShouldEqual, Deny)
		})

		Convey("引用内置组 — 允许 GET", func() {
			p := &asset_entity.RedisPolicy{
				Groups: []string{policy.BuiltinRedisReadOnly},
			}
			out := testRedisPolicy(ctx, p, nil, "GET user:1")
			So(out.Decision, ShouldEqual, Allow)
		})

		Convey("资产 allow 规则匹配", func() {
			p := &asset_entity.RedisPolicy{AllowList: []string{"GET *"}}
			out := testRedisPolicy(ctx, p, nil, "GET user:1")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "")
		})

		Convey("资产 allow 存在但命令不匹配时 NeedConfirm", func() {
			p := &asset_entity.RedisPolicy{AllowList: []string{"GET *"}}
			out := testRedisPolicy(ctx, p, nil, "SET user:1 val")
			So(out.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("组通用策略 deny 匹配 Redis 命令", func() {
			groups := []*group_entity.Group{
				makeGroup("生产组", `{"deny_list":["GET *"]}`),
			}
			out := testRedisPolicy(ctx, nil, groups, "GET user:1")
			So(out.Decision, ShouldEqual, Deny)
			So(out.MatchedSource, ShouldEqual, "生产组")
			So(out.MatchedPattern, ShouldEqual, "GET *")
		})

		Convey("组通用策略 allow 匹配 Redis 命令", func() {
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["GET *"]}`),
			}
			out := testRedisPolicy(ctx, nil, groups, "GET user:1")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "dev组")
		})

		Convey("组通用 deny 优先于资产 allow", func() {
			p := &asset_entity.RedisPolicy{AllowList: []string{"GET *"}}
			groups := []*group_entity.Group{
				makeGroup("安全组", `{"deny_list":["GET *"]}`),
			}
			out := testRedisPolicy(ctx, p, groups, "GET user:1")
			So(out.Decision, ShouldEqual, Deny)
			So(out.MatchedSource, ShouldEqual, "安全组")
		})

		Convey("多层组链 deny 父组匹配", func() {
			groups := []*group_entity.Group{
				makeGroup("子组", `{}`),
				makeGroup("根组", `{"deny_list":["DEL *"]}`),
			}
			out := testRedisPolicy(ctx, nil, groups, "DEL user:1")
			So(out.Decision, ShouldEqual, Deny)
			So(out.MatchedSource, ShouldEqual, "根组")
		})

		Convey("默认策略（引用内置组）正确生效", func() {
			p := policy.DefaultRedisPolicy()
			out := testRedisPolicy(ctx, p, nil, "GET user:1")
			So(out.Decision, ShouldEqual, Allow)

			out = testRedisPolicy(ctx, p, nil, "FLUSHDB")
			So(out.Decision, ShouldEqual, Deny)
		})
	})
}

func TestTestK8sPolicy(t *testing.T) {
	ctx := context.Background()

	Convey("testK8sPolicy", t, func() {
		Convey("无策略时返回 NeedConfirm", func() {
			out := testK8sPolicy(ctx, nil, nil, "kubectl get pods")
			So(out.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("引用内置组时允许只读命令", func() {
			p := &asset_entity.K8sPolicy{
				Groups: []string{policy.BuiltinK8sReadOnly},
			}
			out := testK8sPolicy(ctx, p, nil, "kubectl get pods -A")
			So(out.Decision, ShouldEqual, Allow)
		})

		Convey("引用内置组时拒绝高危命令", func() {
			p := &asset_entity.K8sPolicy{
				Groups: []string{policy.BuiltinK8sDangerousDeny},
			}
			out := testK8sPolicy(ctx, p, nil, "kubectl delete pod nginx")
			So(out.Decision, ShouldEqual, Deny)
		})

		Convey("资产 allow 规则匹配", func() {
			p := &asset_entity.K8sPolicy{AllowList: []string{"kubectl get *"}}
			out := testK8sPolicy(ctx, p, nil, "kubectl get pods")
			So(out.Decision, ShouldEqual, Allow)
		})

		Convey("资产 deny 规则匹配", func() {
			p := &asset_entity.K8sPolicy{DenyList: []string{"kubectl delete *"}}
			out := testK8sPolicy(ctx, p, nil, "kubectl delete pod nginx")
			So(out.Decision, ShouldEqual, Deny)
		})

		Convey("组链上的 K8S 策略会生效", func() {
			group := &group_entity.Group{Name: "测试组", K8sPolicy: `{"allow_list":["kubectl get *"]}`}
			out := testK8sPolicy(ctx, nil, []*group_entity.Group{group}, "kubectl get ns")
			So(out.Decision, ShouldEqual, Allow)
		})

		Convey("默认策略正确生效", func() {
			p := policy.DefaultK8sPolicy()
			out := testK8sPolicy(ctx, p, nil, "kubectl get pods")
			So(out.Decision, ShouldEqual, Allow)

			out = testK8sPolicy(ctx, p, nil, "kubectl delete pod nginx")
			So(out.Decision, ShouldEqual, Deny)

			out = testK8sPolicy(ctx, p, nil, "kubectl apply -f deploy.yaml")
			So(out.Decision, ShouldEqual, NeedConfirm)
		})
	})
}

func TestTestQueryPolicy(t *testing.T) {
	ctx := context.Background()

	Convey("testQueryPolicy", t, func() {
		Convey("无策略时 Allow（无限制）", func() {
			out := testQueryPolicy(ctx, nil, nil, "SELECT * FROM users")
			So(out.Decision, ShouldEqual, Allow)
		})

		Convey("引用内置组 — SQL 只读允许 SELECT", func() {
			p := &asset_entity.QueryPolicy{
				Groups: []string{policy.BuiltinSQLReadOnly},
			}
			out := testQueryPolicy(ctx, p, nil, "SELECT * FROM users")
			So(out.Decision, ShouldEqual, Allow)
		})

		Convey("引用内置组 — SQL 高危拒绝 DROP TABLE", func() {
			p := &asset_entity.QueryPolicy{
				Groups: []string{policy.BuiltinSQLDangerousDeny},
			}
			out := testQueryPolicy(ctx, p, nil, "DROP TABLE users")
			So(out.Decision, ShouldEqual, Deny)
		})

		Convey("组通用策略 deny 匹配 SQL", func() {
			groups := []*group_entity.Group{
				makeGroup("生产组", `{"deny_list":["DELETE *"]}`),
			}
			out := testQueryPolicy(ctx, nil, groups, "DELETE FROM users WHERE id=1")
			So(out.Decision, ShouldEqual, Deny)
			So(out.MatchedSource, ShouldEqual, "生产组")
			So(out.MatchedPattern, ShouldEqual, "DELETE *")
		})

		Convey("组通用策略 allow 匹配 SQL", func() {
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["SELECT *"]}`),
			}
			out := testQueryPolicy(ctx, nil, groups, "SELECT * FROM users")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "dev组")
		})

		Convey("资产 deny_types 覆盖", func() {
			p := &asset_entity.QueryPolicy{DenyTypes: []string{"INSERT"}}
			out := testQueryPolicy(ctx, p, nil, "INSERT INTO users VALUES (1)")
			So(out.Decision, ShouldEqual, Deny)
			So(out.MatchedSource, ShouldEqual, "")
		})

		Convey("默认策略（引用内置组）正确生效", func() {
			p := policy.DefaultQueryPolicy()
			out := testQueryPolicy(ctx, p, nil, "SELECT * FROM users")
			So(out.Decision, ShouldEqual, Allow)

			out = testQueryPolicy(ctx, p, nil, "DROP TABLE users")
			So(out.Decision, ShouldEqual, Deny)
		})
	})
}

func TestCheckGenericDenyAllow(t *testing.T) {
	Convey("checkGenericDeny/Allow", t, func() {
		Convey("deny 匹配返回结果", func() {
			rules := []taggedRule{
				{"GET *", "生产组"},
				{"SET *", "安全组"},
			}
			out := checkGenericDeny(rules, "GET user:1", MatchRedisRule)
			So(out, ShouldNotBeNil)
			So(out.Decision, ShouldEqual, Deny)
			So(out.MatchedSource, ShouldEqual, "生产组")
		})

		Convey("deny 不匹配返回 nil", func() {
			rules := []taggedRule{{"SET *", "安全组"}}
			out := checkGenericDeny(rules, "GET user:1", MatchRedisRule)
			So(out, ShouldBeNil)
		})

		Convey("allow 匹配返回结果", func() {
			rules := []taggedRule{{"GET *", "dev组"}}
			out := checkGenericAllow(rules, "GET user:1", MatchRedisRule)
			So(out, ShouldNotBeNil)
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "dev组")
		})

		Convey("allow 不匹配返回 nil", func() {
			rules := []taggedRule{{"SET *", "dev组"}}
			out := checkGenericAllow(rules, "GET user:1", MatchRedisRule)
			So(out, ShouldBeNil)
		})
	})
}
