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

		Convey("无允许列表 → 全部允许", func() {
			p := &asset_entity.RedisPolicy{}
			result := CheckRedisPolicy(ctx, p, "SET mykey value")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
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
			// DefaultRedisPolicy 只含 Groups 引用，mergeRedisPolicy 不解析 Groups
			// 所以 nil policy 时 DenyList/AllowList 都为空 → Allow
			result := CheckRedisPolicy(ctx, nil, "GET mykey")
			So(result.Decision, ShouldEqual, Allow)
		})
	})
}

func TestMergeRedisPolicy(t *testing.T) {
	Convey("mergeRedisPolicy", t, func() {
		Convey("合并自定义与默认拒绝列表", func() {
			custom := &asset_entity.RedisPolicy{
				DenyList: []string{"FLUSHDB"},
			}
			defaults := &asset_entity.RedisPolicy{
				DenyList: []string{"FLUSHALL", "DEBUG"},
			}
			result := mergeRedisPolicy(custom, defaults)
			So(result.DenyList, ShouldHaveLength, 3)
			So(result.DenyList, ShouldContain, "FLUSHDB")
			So(result.DenyList, ShouldContain, "FLUSHALL")
			So(result.DenyList, ShouldContain, "DEBUG")
		})

		Convey("大小写不敏感去重", func() {
			custom := &asset_entity.RedisPolicy{
				DenyList: []string{"flushall"},
			}
			defaults := &asset_entity.RedisPolicy{
				DenyList: []string{"FLUSHALL"},
			}
			result := mergeRedisPolicy(custom, defaults)
			So(result.DenyList, ShouldHaveLength, 1)
			So(result.DenyList[0], ShouldEqual, "flushall")
		})

		Convey("custom 为 nil", func() {
			defaults := &asset_entity.RedisPolicy{
				DenyList: []string{"FLUSHALL"},
			}
			result := mergeRedisPolicy(nil, defaults)
			So(result.DenyList, ShouldHaveLength, 1)
			So(result.DenyList[0], ShouldEqual, "FLUSHALL")
		})

		Convey("defaults 为 nil", func() {
			custom := &asset_entity.RedisPolicy{
				DenyList: []string{"FLUSHDB"},
			}
			result := mergeRedisPolicy(custom, nil)
			So(result.DenyList, ShouldHaveLength, 1)
			So(result.DenyList[0], ShouldEqual, "FLUSHDB")
		})

		Convey("custom 的 AllowList 保留", func() {
			custom := &asset_entity.RedisPolicy{
				AllowList: []string{"GET", "SET"},
				DenyList:  []string{"FLUSHALL"},
			}
			defaults := &asset_entity.RedisPolicy{
				DenyList: []string{"DEBUG"},
			}
			result := mergeRedisPolicy(custom, defaults)
			So(result.AllowList, ShouldResemble, []string{"GET", "SET"})
			So(result.DenyList, ShouldHaveLength, 2)
		})
	})
}
