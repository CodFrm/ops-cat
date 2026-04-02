// internal/model/entity/policy_group_entity/policy_group_ext_test.go
package policy_group_entity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExtensionPolicyGroups(t *testing.T) {
	Convey("Extension policy groups", t, func() {
		Convey("IsExtensionID recognizes ext: prefix", func() {
			So(IsExtensionID("ext:oss:readonly"), ShouldBeTrue)
			So(IsExtensionID("ext:k8s:admin"), ShouldBeTrue)
			So(IsExtensionID("builtin:linux-readonly"), ShouldBeFalse)
			So(IsExtensionID("123"), ShouldBeFalse)
		})

		Convey("RegisterExtensionGroup and FindExtensionGroup", func() {
			RegisterExtensionGroup(&PolicyGroup{
				BuiltinID:   "ext:oss:readonly",
				Name:        "OSS Read-Only",
				Description: "Allow list and read only",
				PolicyType:  "oss",
				Policy:      `{"allow_list":["list","read"]}`,
			})

			pg := FindExtensionGroup("ext:oss:readonly")
			So(pg, ShouldNotBeNil)
			So(pg.Name, ShouldEqual, "OSS Read-Only")

			So(FindExtensionGroup("ext:nonexistent"), ShouldBeNil)

			UnregisterExtensionGroups("oss")
			So(FindExtensionGroup("ext:oss:readonly"), ShouldBeNil)
		})
	})
}
