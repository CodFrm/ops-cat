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
		Convey("无策略时使用默认 Redis 策略", func() {
			out := testRedisPolicy(ctx, nil, nil, "GET user:1")
			So(out.Decision, ShouldEqual, Allow)

			out = testRedisPolicy(ctx, nil, nil, "SET user:1 val")
			So(out.Decision, ShouldEqual, NeedConfirm)

			out = testRedisPolicy(ctx, nil, nil, "DEBUG STATS")
			So(out.Decision, ShouldEqual, Deny)
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

		Convey("组通用 allow 仅在资产策略 NeedConfirm 时提升 Redis 决策为 Allow", func() {
			// 与 runtime checkRedisPermission 对齐：资产策略已是 Allow（默认 Redis 策略
			// 允许 GET *）时直接返回，不会被 dev组 的 allow 抢走 MatchedSource
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["GET *"]}`),
			}
			out := testRedisPolicy(ctx, nil, groups, "GET user:1")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "")
		})

		Convey("Redis 默认策略 NeedConfirm 时组 allow 升级为 Allow", func() {
			// SET 默认 NeedConfirm，dev组 显式 allow_list 覆盖 → Allow, MatchedSource=dev组
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["SET *"]}`),
			}
			out := testRedisPolicy(ctx, nil, groups, "SET user:1 val")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "dev组")
		})

		Convey("资产 Redis allow 已通过时组 allow 不应改写决策来源", func() {
			// runtime checkRedisPermission 只用组 allow 把 NeedConfirm 升为 Allow；
			// 资产策略本身命中 Allow 时，tester 不能再被组规则"抢走"成 MatchedSource=组名
			p := &asset_entity.RedisPolicy{AllowList: []string{"GET *"}}
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["GET *"]}`),
			}
			out := testRedisPolicy(ctx, p, groups, "GET user:1")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "")
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
		Convey("无策略时使用默认 K8S 策略", func() {
			out := testK8sPolicy(ctx, nil, nil, "kubectl get pods")
			So(out.Decision, ShouldEqual, Allow)

			out = testK8sPolicy(ctx, nil, nil, "kubectl apply -f deploy.yaml")
			So(out.Decision, ShouldEqual, NeedConfirm)

			out = testK8sPolicy(ctx, nil, nil, "kubectl delete pod nginx")
			So(out.Decision, ShouldEqual, Deny)
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
			group := &group_entity.Group{Name: "测试组", K8sPol: `{"allow_list":["kubectl get *"]}`}
			out := testK8sPolicy(ctx, nil, []*group_entity.Group{group}, "kubectl get ns")
			So(out.Decision, ShouldEqual, Allow)
		})

		Convey("组通用 CmdPolicy deny 命中 K8s 命令", func() {
			// 与真实路径 checkK8sPermission 对齐：testK8sPolicy 也要走 CheckGroupGenericPolicy
			groups := []*group_entity.Group{
				makeGroup("安全组", `{"deny_list":["kubectl delete *"]}`),
			}
			out := testK8sPolicy(ctx, nil, groups, "kubectl delete pod nginx")
			So(out.Decision, ShouldEqual, Deny)
			So(out.MatchedSource, ShouldEqual, "安全组")
			So(out.MatchedPattern, ShouldEqual, "kubectl delete *")
		})

		Convey("组通用 CmdPolicy allow 命中 K8s 命令", func() {
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["kubectl rollout *"]}`),
			}
			// kubectl rollout restart 不在默认允许里，依赖组通用 allow 提升为 Allow
			out := testK8sPolicy(ctx, nil, groups, "kubectl rollout restart deploy/api")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "dev组")
		})

		Convey("K8s 组通用 allow 必须按子命令逐条命中", func() {
			groups := []*group_entity.Group{
				makeGroup("k8s组", `{"allow_list":["kubectl *"]}`),
			}
			// 第二个子命令不是 kubectl —— 不能因为 "kubectl *" 整串匹配就放行
			out := testK8sPolicy(ctx, nil, groups, "kubectl get pods && curl http://evil.com")
			So(out.Decision, ShouldNotEqual, Allow)
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
		Convey("无策略时使用默认 SQL 策略", func() {
			out := testQueryPolicy(ctx, nil, nil, "SELECT * FROM users")
			So(out.Decision, ShouldEqual, Allow)

			out = testQueryPolicy(ctx, nil, nil, "INSERT INTO users VALUES (1)")
			So(out.Decision, ShouldEqual, NeedConfirm)

			out = testQueryPolicy(ctx, nil, nil, "DROP TABLE users")
			So(out.Decision, ShouldEqual, Deny)
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

		Convey("组通用 allow 仅在资产 SQL 策略 NeedConfirm 时提升决策为 Allow", func() {
			// 与 runtime checkDatabasePermission 对齐：资产/默认策略已是 Allow（默认允许 SELECT）
			// 时不会被 dev组 的 allow 抢走 MatchedSource。
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["SELECT *"]}`),
			}
			out := testQueryPolicy(ctx, nil, groups, "SELECT * FROM users")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "")
		})

		Convey("SQL 默认策略 NeedConfirm 时组 allow 升级为 Allow", func() {
			// INSERT 默认 NeedConfirm，dev组 显式 allow_list 覆盖 → Allow, MatchedSource=dev组
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["INSERT *"]}`),
			}
			out := testQueryPolicy(ctx, nil, groups, "INSERT INTO users VALUES (1)")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "dev组")
		})

		Convey("资产 SQL allow 已通过时组 allow 不应改写决策来源", func() {
			// runtime checkDatabasePermission 仅在 NeedConfirm 时用组 allow 升级；
			// 资产策略本身就允许 SELECT 时，tester 不能被组规则"抢走"决策来源
			p := &asset_entity.QueryPolicy{AllowTypes: []string{"SELECT"}}
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["SELECT *"]}`),
			}
			out := testQueryPolicy(ctx, p, groups, "SELECT * FROM users")
			So(out.Decision, ShouldEqual, Allow)
			So(out.MatchedSource, ShouldEqual, "")
		})

		Convey("组通用 deny UPDATE * 命中分号后的 UPDATE 语句（多语句拆分检查）", func() {
			// 防止 `SELECT 1; UPDATE users SET ...` 整串匹配 SELECT 规则导致 UPDATE 被静默放行
			groups := []*group_entity.Group{
				makeGroup("prod组", `{"deny_list":["UPDATE *"]}`),
			}
			out := testQueryPolicy(ctx, nil, groups, "SELECT 1; UPDATE users SET name='x' WHERE id=1")
			So(out.Decision, ShouldEqual, Deny)
			So(out.MatchedSource, ShouldEqual, "prod组")
		})

		Convey("组 allow SELECT * 不能放行多语句中的 UPDATE", func() {
			groups := []*group_entity.Group{
				makeGroup("dev组", `{"allow_list":["SELECT *"]}`),
			}
			out := testQueryPolicy(ctx, nil, groups, "SELECT 1; UPDATE users SET name='x' WHERE id=1")
			So(out.Decision, ShouldNotEqual, Allow)
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
