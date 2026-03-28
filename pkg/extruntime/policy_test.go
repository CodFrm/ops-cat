package extruntime

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckExtensionPolicy(t *testing.T) {
	Convey("CheckExtensionPolicy", t, func() {
		Convey("Given 空策略 JSON", func() {
			result := CheckExtensionPolicy("", "list_buckets", "")
			Convey("Then 返回 NeedConfirm", func() {
				So(result.Decision, ShouldEqual, ExtNeedConfirm)
			})
		})

		Convey("Given 无效 JSON", func() {
			result := CheckExtensionPolicy("{invalid", "list_buckets", "")
			Convey("Then 返回 NeedConfirm", func() {
				So(result.Decision, ShouldEqual, ExtNeedConfirm)
			})
		})

		Convey("Given allow_list 包含 action", func() {
			policy := `{"allow_list":["list_buckets","get_object"],"deny_list":[]}`
			result := CheckExtensionPolicy(policy, "list_buckets", "")
			Convey("Then 返回 Allow", func() {
				So(result.Decision, ShouldEqual, ExtAllow)
				So(result.DecisionSource, ShouldEqual, "policy_allow")
			})
		})

		Convey("Given deny_list 包含 action", func() {
			policy := `{"allow_list":[],"deny_list":["delete_bucket"]}`
			result := CheckExtensionPolicy(policy, "delete_bucket", "")
			Convey("Then 返回 Deny + 消息包含 action 名", func() {
				So(result.Decision, ShouldEqual, ExtDeny)
				So(result.Message, ShouldContainSubstring, "delete_bucket")
				So(result.DecisionSource, ShouldEqual, "policy_deny")
			})
		})

		Convey("Given deny 和 allow 都包含同一 action", func() {
			policy := `{"allow_list":["delete_bucket"],"deny_list":["delete_bucket"]}`
			result := CheckExtensionPolicy(policy, "delete_bucket", "")
			Convey("Then deny 优先", func() {
				So(result.Decision, ShouldEqual, ExtDeny)
			})
		})

		Convey("Given allow_list 包含 *", func() {
			policy := `{"allow_list":["*"],"deny_list":[]}`
			result := CheckExtensionPolicy(policy, "any_action", "")
			Convey("Then 匹配任意 action", func() {
				So(result.Decision, ShouldEqual, ExtAllow)
			})
		})

		Convey("Given deny_list 包含 *", func() {
			policy := `{"allow_list":["*"],"deny_list":["*"]}`
			result := CheckExtensionPolicy(policy, "any_action", "")
			Convey("Then deny * 优先于 allow *", func() {
				So(result.Decision, ShouldEqual, ExtDeny)
			})
		})

		Convey("Given action 不在任何列表中", func() {
			policy := `{"allow_list":["list_buckets"],"deny_list":["delete_bucket"]}`
			result := CheckExtensionPolicy(policy, "upload_object", "")
			Convey("Then 返回 NeedConfirm", func() {
				So(result.Decision, ShouldEqual, ExtNeedConfirm)
			})
		})
	})
}
