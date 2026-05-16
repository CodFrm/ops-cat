package ai

import (
	"context"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/policy_group_entity"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckExtensionPolicy(t *testing.T) {
	Convey("checkExtensionPolicy", t, func() {
		ctx := context.Background()

		// Register test extension policy groups
		policy_group_entity.RegisterExtensionGroup(&policy_group_entity.PolicyGroup{
			BuiltinID:  "ext:oss:readonly",
			Name:       "OSS Read-Only",
			PolicyType: "oss",
			Policy:     `{"allow_list":["list","read"],"deny_list":["delete","admin"]}`,
		})
		policy_group_entity.RegisterExtensionGroup(&policy_group_entity.PolicyGroup{
			BuiltinID:  "ext:oss:dangerous-deny",
			Name:       "OSS Dangerous Deny",
			PolicyType: "oss",
			Policy:     `{"deny_list":["delete","admin"]}`,
		})

		Reset(func() {
			policy_group_entity.UnregisterExtensionGroups("oss")
		})

		Convey("Allow when action is in allow_list", func() {
			result := checkExtensionPolicy(ctx, []string{"ext:oss:readonly"}, "read")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("Deny when action is in deny_list", func() {
			result := checkExtensionPolicy(ctx, []string{"ext:oss:readonly"}, "delete")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)
		})

		Convey("NeedConfirm when action not in any list", func() {
			result := checkExtensionPolicy(ctx, []string{"ext:oss:readonly"}, "upload")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})

		Convey("Merging multiple groups: deny takes precedence", func() {
			// "ext:oss:readonly" has allow_list with "read", but also deny_list with "delete"
			// "ext:oss:dangerous-deny" has deny_list with "delete"
			// Even if one group allows "read", if another group denies it, deny wins.
			// Here test that "delete" is denied even across groups.
			result := checkExtensionPolicy(ctx, []string{"ext:oss:readonly", "ext:oss:dangerous-deny"}, "delete")
			So(result.Decision, ShouldEqual, Deny)
			So(result.DecisionSource, ShouldEqual, SourcePolicyDeny)

			// "read" is only in allow_list, not in any deny_list → Allow
			result = checkExtensionPolicy(ctx, []string{"ext:oss:readonly", "ext:oss:dangerous-deny"}, "read")
			So(result.Decision, ShouldEqual, Allow)
			So(result.DecisionSource, ShouldEqual, SourcePolicyAllow)
		})

		Convey("NeedConfirm when no groups configured", func() {
			result := checkExtensionPolicy(ctx, nil, "read")
			So(result.Decision, ShouldEqual, NeedConfirm)
		})
	})
}
