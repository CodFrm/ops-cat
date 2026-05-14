package ai

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

func TestExtractRedisCommand(t *testing.T) {
	Convey("ExtractRedisCommand", t, func() {
		Convey("简单命令 GET", func() {
			cmd, args := ExtractRedisCommand("GET mykey")
			So(cmd, ShouldEqual, "GET")
			So(args, ShouldEqual, "mykey")
		})

		Convey("多词命令 CONFIG SET", func() {
			cmd, args := ExtractRedisCommand("CONFIG SET maxmemory 128mb")
			So(cmd, ShouldEqual, "CONFIG SET")
			So(args, ShouldEqual, "maxmemory 128mb")
		})

		Convey("空字符串", func() {
			cmd, args := ExtractRedisCommand("")
			So(cmd, ShouldBeEmpty)
			So(args, ShouldBeEmpty)
		})

		Convey("单命令无参数 PING", func() {
			cmd, args := ExtractRedisCommand("PING")
			So(cmd, ShouldEqual, "PING")
			So(args, ShouldBeEmpty)
		})

		Convey("非多词命令带参数 DEL", func() {
			cmd, args := ExtractRedisCommand("DEL key1 key2")
			So(cmd, ShouldEqual, "DEL")
			So(args, ShouldEqual, "key1 key2")
		})

		Convey("多词命令 XGROUP CREATE", func() {
			cmd, args := ExtractRedisCommand("XGROUP CREATE mystream grpname $")
			So(cmd, ShouldEqual, "XGROUP CREATE")
			So(args, ShouldEqual, "mystream grpname $")
		})
	})
}

func TestMatchRedisRule(t *testing.T) {
	Convey("MatchRedisRule", t, func() {
		Convey("精确匹配", func() {
			So(MatchRedisRule("GET mykey", "GET mykey"), ShouldBeTrue)
		})

		Convey("规则无参数匹配任意参数", func() {
			So(MatchRedisRule("GET", "GET mykey"), ShouldBeTrue)
			So(MatchRedisRule("GET", "GET"), ShouldBeTrue)
		})

		Convey("通配符 * 匹配任意参数", func() {
			So(MatchRedisRule("GET *", "GET mykey"), ShouldBeTrue)
			So(MatchRedisRule("GET *", "GET"), ShouldBeTrue)
		})

		Convey("单独 * 匹配任意 Redis 命令", func() {
			So(MatchRedisRule("*", "INFO"), ShouldBeTrue)
			So(MatchRedisRule("*", "SET a b"), ShouldBeTrue)
		})

		Convey("多词命令的子命令通配符保持语义", func() {
			So(MatchRedisRule("DEBUG *", "DEBUG STATS"), ShouldBeTrue)
			So(MatchRedisRule("DEBUG *", "CONFIG SET maxmemory 128mb"), ShouldBeFalse)
		})

		Convey("key pattern glob 匹配", func() {
			So(MatchRedisRule("DEL user:*", "DEL user:123"), ShouldBeTrue)
			So(MatchRedisRule("DEL user:*", "DEL order:123"), ShouldBeFalse)
			So(MatchRedisRule("DEL cache/*", "DEL cache/a/b"), ShouldBeFalse)
		})

		Convey("不同命令不匹配", func() {
			So(MatchRedisRule("GET mykey", "SET mykey"), ShouldBeFalse)
		})

		Convey("规则有参数但命令无参数", func() {
			So(MatchRedisRule("GET mykey", "GET"), ShouldBeFalse)
		})

		Convey("多词命令大小写不敏感", func() {
			So(MatchRedisRule("config set", "CONFIG SET maxmemory 128mb"), ShouldBeTrue)
		})
	})
}

func TestCheckRedisPolicy(t *testing.T) {
	ctx := context.Background()

	Convey("CheckRedisPolicy", t, func() {
		Convey("拒绝列表命中 → Deny，DecisionSource=SourcePolicyDeny", func() {
			p := &asset_entity.RedisPolicy{
				DenyList: []string{"FLUSHALL"},
			}
			result := CheckRedisPolicy(ctx, p, "FLUSHALL")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("允许列表命中 → Allow，DecisionSource=SourcePolicyAllow", func() {
			p := &asset_entity.RedisPolicy{
				AllowList: []string{"GET", "SET"},
			}
			result := CheckRedisPolicy(ctx, p, "GET mykey")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("有允许列表但未命中 → NeedConfirm", func() {
			p := &asset_entity.RedisPolicy{
				AllowList: []string{"GET"},
			}
			result := CheckRedisPolicy(ctx, p, "DEL mykey")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("空策略使用默认只读 allow", func() {
			p := &asset_entity.RedisPolicy{}
			result := CheckRedisPolicy(ctx, p, "INFO")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("空策略下写命令需要确认", func() {
			p := &asset_entity.RedisPolicy{}
			result := CheckRedisPolicy(ctx, p, "SET mykey value")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("空策略叠加默认 dangerous deny", func() {
			p := &asset_entity.RedisPolicy{}
			result := CheckRedisPolicy(ctx, p, "DEBUG STATS")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "DEBUG *")
		})

		Convey("拒绝列表优先于允许列表", func() {
			p := &asset_entity.RedisPolicy{
				AllowList: []string{"FLUSHALL"},
				DenyList:  []string{"FLUSHALL"},
			}
			result := CheckRedisPolicy(ctx, p, "FLUSHALL")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("nil policy 使用默认策略", func() {
			result := CheckRedisPolicy(ctx, nil, "GET mykey")
			So(result.Decision, ShouldEqual, Allow)

			result = CheckRedisPolicy(ctx, nil, "SET mykey value")
			So(result.Decision, ShouldEqual, NeedConfirm)

			result = CheckRedisPolicy(ctx, nil, "DEBUG STATS")
			So(result.Decision, ShouldEqual, Deny)
		})

		Convey("allow_list wildcard allows any non-dangerous Redis command", func() {
			p := &asset_entity.RedisPolicy{AllowList: []string{"*"}}
			result := CheckRedisPolicy(ctx, p, "SET mykey value")
			So(result.Decision, ShouldEqual, Allow)

			result = CheckRedisPolicy(ctx, p, "DEBUG STATS")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("deny_list wildcard denies every Redis command", func() {
			p := &asset_entity.RedisPolicy{DenyList: []string{"*"}}
			result := CheckRedisPolicy(ctx, p, "INFO")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "*")
		})

		Convey("explicit allow list replaces default read-only allow", func() {
			p := &asset_entity.RedisPolicy{AllowList: []string{"GET *"}}
			result := CheckRedisPolicy(ctx, p, "INFO")
			So(result.Decision, ShouldEqual, NeedConfirm)

			result = CheckRedisPolicy(ctx, p, "DEBUG STATS")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})
	})
}

