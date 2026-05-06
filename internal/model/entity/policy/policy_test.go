package policy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCommandPolicyIsEmpty(t *testing.T) {
	Convey("CommandPolicy.IsEmpty()", t, func() {
		Convey("空策略返回 true", func() {
			p := &CommandPolicy{}
			So(p.IsEmpty(), ShouldBeTrue)
		})

		Convey("有 AllowList 时返回 false", func() {
			p := &CommandPolicy{AllowList: []string{"ls"}}
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("有 DenyList 时返回 false", func() {
			p := &CommandPolicy{DenyList: []string{"rm -rf *"}}
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("有 Groups 时返回 false", func() {
			p := &CommandPolicy{Groups: []string{BuiltinLinuxReadOnly}}
			So(p.IsEmpty(), ShouldBeFalse)
		})
	})
}

func TestQueryPolicyIsEmpty(t *testing.T) {
	Convey("QueryPolicy.IsEmpty()", t, func() {
		Convey("空策略返回 true", func() {
			p := &QueryPolicy{}
			So(p.IsEmpty(), ShouldBeTrue)
		})

		Convey("有 AllowTypes 时返回 false", func() {
			p := &QueryPolicy{AllowTypes: []string{"SELECT"}}
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("有 DenyTypes 时返回 false", func() {
			p := &QueryPolicy{DenyTypes: []string{"DROP"}}
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("有 DenyFlags 时返回 false", func() {
			p := &QueryPolicy{DenyFlags: []string{"no_where_delete"}}
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("有 Groups 时返回 false", func() {
			p := &QueryPolicy{Groups: []string{BuiltinSQLReadOnly}}
			So(p.IsEmpty(), ShouldBeFalse)
		})
	})
}

func TestRedisPolicyIsEmpty(t *testing.T) {
	Convey("RedisPolicy.IsEmpty()", t, func() {
		Convey("空策略返回 true", func() {
			p := &RedisPolicy{}
			So(p.IsEmpty(), ShouldBeTrue)
		})

		Convey("有 AllowList 时返回 false", func() {
			p := &RedisPolicy{AllowList: []string{"GET"}}
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("有 DenyList 时返回 false", func() {
			p := &RedisPolicy{DenyList: []string{"FLUSHALL"}}
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("有 Groups 时返回 false", func() {
			p := &RedisPolicy{Groups: []string{BuiltinRedisReadOnly}}
			So(p.IsEmpty(), ShouldBeFalse)
		})
	})
}

func TestDefaultCommandPolicy(t *testing.T) {
	Convey("DefaultCommandPolicy()", t, func() {
		p := DefaultCommandPolicy()

		Convey("不为空", func() {
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("包含内置权限组引用", func() {
			So(p.Groups, ShouldContain, BuiltinLinuxReadOnly)
			So(p.Groups, ShouldContain, BuiltinDangerousDeny)
		})
	})
}

func TestDefaultQueryPolicy(t *testing.T) {
	Convey("DefaultQueryPolicy()", t, func() {
		p := DefaultQueryPolicy()

		Convey("不为空", func() {
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("包含内置 SQL 权限组引用", func() {
			So(p.Groups, ShouldContain, BuiltinSQLReadOnly)
			So(p.Groups, ShouldContain, BuiltinSQLDangerousDeny)
		})
	})
}

func TestDefaultRedisPolicy(t *testing.T) {
	Convey("DefaultRedisPolicy()", t, func() {
		p := DefaultRedisPolicy()

		Convey("不为空", func() {
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("包含内置 Redis 权限组引用", func() {
			So(p.Groups, ShouldContain, BuiltinRedisReadOnly)
			So(p.Groups, ShouldContain, BuiltinRedisDangerousDeny)
		})
	})
}

func TestMongoPolicyIsEmpty(t *testing.T) {
	Convey("MongoPolicy.IsEmpty()", t, func() {
		Convey("空策略返回 true", func() {
			p := &MongoPolicy{}
			So(p.IsEmpty(), ShouldBeTrue)
		})

		Convey("有 AllowTypes 时返回 false", func() {
			p := &MongoPolicy{AllowTypes: []string{"find"}}
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("有 DenyTypes 时返回 false", func() {
			p := &MongoPolicy{DenyTypes: []string{"dropDatabase"}}
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("有 Groups 时返回 false", func() {
			p := &MongoPolicy{Groups: []string{BuiltinMongoReadOnly}}
			So(p.IsEmpty(), ShouldBeFalse)
		})
	})
}

func TestDefaultMongoPolicy(t *testing.T) {
	Convey("DefaultMongoPolicy()", t, func() {
		p := DefaultMongoPolicy()

		Convey("不为空", func() {
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("包含内置 MongoDB 权限组引用", func() {
			So(p.Groups, ShouldContain, BuiltinMongoReadOnly)
		})
	})
}

func TestKafkaPolicyIsEmpty(t *testing.T) {
	Convey("KafkaPolicy.IsEmpty()", t, func() {
		Convey("空策略返回 true", func() {
			p := &KafkaPolicy{}
			So(p.IsEmpty(), ShouldBeTrue)
		})

		Convey("有 AllowList 时返回 false", func() {
			p := &KafkaPolicy{AllowList: []string{"topic.read *"}}
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("有 DenyList 时返回 false", func() {
			p := &KafkaPolicy{DenyList: []string{"topic.delete *"}}
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("有 Groups 时返回 false", func() {
			p := &KafkaPolicy{Groups: []string{BuiltinKafkaMetadataReadOnly}}
			So(p.IsEmpty(), ShouldBeFalse)
		})
	})
}

func TestDefaultKafkaPolicy(t *testing.T) {
	Convey("DefaultKafkaPolicy()", t, func() {
		p := DefaultKafkaPolicy()

		Convey("不为空", func() {
			So(p.IsEmpty(), ShouldBeFalse)
		})

		Convey("包含内置 Kafka 权限组引用", func() {
			So(p.Groups, ShouldContain, BuiltinKafkaMetadataReadOnly)
			So(p.Groups, ShouldContain, BuiltinKafkaDangerousDeny)
		})
	})
}
