package ai

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckMongoDBPolicy_DefaultsAndWildcard(t *testing.T) {
	Convey("CheckMongoDBPolicy defaults and wildcard semantics", t, func() {
		ctx := context.Background()

		Convey("nil policy uses default read-only allow", func() {
			result := CheckMongoDBPolicy(ctx, nil, "find")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("nil policy requires confirmation for writes", func() {
			result := CheckMongoDBPolicy(ctx, nil, "insertOne")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("nil policy applies default dangerous deny", func() {
			result := CheckMongoDBPolicy(ctx, nil, "dropDatabase")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "dropDatabase")
		})

		Convey("allow_types wildcard allows any non-dangerous operation", func() {
			policy := &asset_entity.MongoPolicy{AllowTypes: []string{"*"}}
			result := CheckMongoDBPolicy(ctx, policy, "insertOne")
			So(result.Decision, ShouldEqual, Allow)

			result = CheckMongoDBPolicy(ctx, policy, "dropDatabase")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("deny_types wildcard denies every operation", func() {
			policy := &asset_entity.MongoPolicy{DenyTypes: []string{"*"}}
			result := CheckMongoDBPolicy(ctx, policy, "find")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
			So(result.MatchedPattern, ShouldEqual, "*")
		})
	})
}
