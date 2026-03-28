package extension

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckExtensionPolicy(t *testing.T) {
	Convey("CheckExtensionPolicy", t, func() {
		Convey("空策略 JSON 返回 NeedConfirm", func() {
			result := CheckExtensionPolicy("", "list_buckets", "")
			So(result.Decision, ShouldEqual, ExtNeedConfirm)
		})

		Convey("无效 JSON 返回 NeedConfirm", func() {
			result := CheckExtensionPolicy("{invalid", "list_buckets", "")
			So(result.Decision, ShouldEqual, ExtNeedConfirm)
		})

		Convey("allow_list 匹配返回 Allow", func() {
			policy := `{"allow_list":["list_buckets","get_object"],"deny_list":[]}`
			result := CheckExtensionPolicy(policy, "list_buckets", "")
			So(result.Decision, ShouldEqual, ExtAllow)
			So(result.DecisionSource, ShouldEqual, "policy_allow")
		})

		Convey("deny_list 匹配返回 Deny", func() {
			policy := `{"allow_list":[],"deny_list":["delete_bucket"]}`
			result := CheckExtensionPolicy(policy, "delete_bucket", "")
			So(result.Decision, ShouldEqual, ExtDeny)
			So(result.Message, ShouldContainSubstring, "delete_bucket")
			So(result.DecisionSource, ShouldEqual, "policy_deny")
		})

		Convey("deny 优先于 allow", func() {
			policy := `{"allow_list":["delete_bucket"],"deny_list":["delete_bucket"]}`
			result := CheckExtensionPolicy(policy, "delete_bucket", "")
			So(result.Decision, ShouldEqual, ExtDeny)
		})

		Convey("通配符 * 在 allow_list 中匹配所有", func() {
			policy := `{"allow_list":["*"],"deny_list":[]}`
			result := CheckExtensionPolicy(policy, "any_action", "")
			So(result.Decision, ShouldEqual, ExtAllow)
		})

		Convey("通配符 * 在 deny_list 中匹配所有", func() {
			policy := `{"allow_list":["*"],"deny_list":["*"]}`
			result := CheckExtensionPolicy(policy, "any_action", "")
			So(result.Decision, ShouldEqual, ExtDeny)
		})

		Convey("action 不在任何列表中返回 NeedConfirm", func() {
			policy := `{"allow_list":["list_buckets"],"deny_list":["delete_bucket"]}`
			result := CheckExtensionPolicy(policy, "upload_object", "")
			So(result.Decision, ShouldEqual, ExtNeedConfirm)
		})
	})
}
