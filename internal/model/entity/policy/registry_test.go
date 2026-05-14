package policy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDefaultPolicyRegistry(t *testing.T) {
	Convey("DefaultPolicy Registry", t, func() {
		Convey("内置类型已注册", func() {
			p, ok := GetDefaultPolicyOf("ssh")
			So(ok, ShouldBeTrue)
			So(p, ShouldNotBeNil)
			cp, ok := p.(*CommandPolicy)
			So(ok, ShouldBeTrue)
			So(cp.Groups, ShouldContain, BuiltinLinuxReadOnly)

			p, ok = GetDefaultPolicyOf("serial")
			So(ok, ShouldBeTrue)
			cp, ok = p.(*CommandPolicy)
			So(ok, ShouldBeTrue)
			So(cp.Groups, ShouldContain, BuiltinLinuxReadOnly)

			p, ok = GetDefaultPolicyOf("database")
			So(ok, ShouldBeTrue)
			qp, ok := p.(*QueryPolicy)
			So(ok, ShouldBeTrue)
			So(qp.Groups, ShouldContain, BuiltinSQLReadOnly)

			p, ok = GetDefaultPolicyOf("redis")
			So(ok, ShouldBeTrue)
			rp, ok := p.(*RedisPolicy)
			So(ok, ShouldBeTrue)
			So(rp.Groups, ShouldContain, BuiltinRedisReadOnly)

			p, ok = GetDefaultPolicyOf("kafka")
			So(ok, ShouldBeTrue)
			kp, ok := p.(*KafkaPolicy)
			So(ok, ShouldBeTrue)
			So(kp.Groups, ShouldContain, BuiltinKafkaMetadataReadOnly)
		})

		Convey("未注册类型返回 false", func() {
			_, ok := GetDefaultPolicyOf("nonexistent")
			So(ok, ShouldBeFalse)
		})

		Convey("动态注册和注销", func() {
			RegisterDefaultPolicy("oss", func() any {
				return &CommandPolicy{Groups: []string{"ext:oss:readonly"}}
			})
			defer UnregisterDefaultPolicy("oss")

			p, ok := GetDefaultPolicyOf("oss")
			So(ok, ShouldBeTrue)
			cp, ok := p.(*CommandPolicy)
			So(ok, ShouldBeTrue)
			So(cp.Groups, ShouldResemble, []string{"ext:oss:readonly"})

			UnregisterDefaultPolicy("oss")
			_, ok = GetDefaultPolicyOf("oss")
			So(ok, ShouldBeFalse)
		})

		Convey("覆盖注册", func() {
			RegisterDefaultPolicy("test-type", func() any {
				return &CommandPolicy{Groups: []string{"a"}}
			})
			defer UnregisterDefaultPolicy("test-type")

			RegisterDefaultPolicy("test-type", func() any {
				return &CommandPolicy{Groups: []string{"b"}}
			})

			p, _ := GetDefaultPolicyOf("test-type")
			cp := p.(*CommandPolicy)
			So(cp.Groups, ShouldResemble, []string{"b"})
		})
	})
}
