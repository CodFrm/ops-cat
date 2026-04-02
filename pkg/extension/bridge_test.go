package extension

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/opskat/opskat/internal/model/entity/policy"
)

func TestBridge(t *testing.T) {
	Convey("Bridge", t, func() {
		bridge := NewBridge()

		manifest := &Manifest{
			Name:    "oss",
			Version: "1.0.0",
			AssetTypes: []AssetTypeDef{
				{Type: "oss", I18n: I18nName{Name: "assetType.oss.name"}},
			},
			Tools: []ToolDef{
				{Name: "list_buckets", I18n: I18nDesc{Description: "tools.list_buckets.description"}},
			},
			Policies: PoliciesDef{
				Type:    "oss",
				Actions: []string{"list", "read", "write"},
				Groups: []PolicyGroupDef{
					{
						ID:     "ext:oss:readonly",
						I18n:   I18nNameDesc{Name: "n", Description: "d"},
						Policy: map[string]any{"allow_list": []any{"list", "read"}},
					},
				},
				Default: []string{"ext:oss:readonly"},
			},
		}
		ext := &Extension{
			Name:     "oss",
			Manifest: manifest,
			SkillMD:  "# OSS Tools\nUse exec_tool...",
		}

		bridge.Register(ext)

		Convey("GetAssetTypes returns registered types", func() {
			types := bridge.GetAssetTypes()
			So(len(types), ShouldEqual, 1)
			So(types[0].Type, ShouldEqual, "oss")
			So(types[0].ExtensionName, ShouldEqual, "oss")
		})

		Convey("GetPolicyGroups returns registered groups", func() {
			groups := bridge.GetPolicyGroups()
			So(len(groups), ShouldEqual, 1)
			So(groups[0].ID, ShouldEqual, "ext:oss:readonly")
		})

		Convey("GetDefaultPolicyGroups returns defaults", func() {
			defaults := bridge.GetDefaultPolicyGroups("oss")
			So(defaults, ShouldResemble, []string{"ext:oss:readonly"})
		})

		Convey("GetSkillMD returns SKILL.md for asset type", func() {
			md := bridge.GetSkillMD("oss")
			So(md, ShouldContainSubstring, "OSS Tools")
		})

		Convey("GetSkillMD returns empty for unknown type", func() {
			md := bridge.GetSkillMD("unknown")
			So(md, ShouldBeEmpty)
		})

		Convey("GetExtensionPolicyGroups returns defaults for known type", func() {
			groups := bridge.GetExtensionPolicyGroups("oss", "oss", 1)
			So(groups, ShouldResemble, []string{"ext:oss:readonly"})
		})

		Convey("GetExtensionPolicyGroups returns nil for unknown type", func() {
			groups := bridge.GetExtensionPolicyGroups("oss", "unknown", 1)
			So(groups, ShouldBeNil)
		})

		Convey("FindExtensionByTool returns extension by tool", func() {
			found := bridge.FindExtensionByTool("oss", "list_buckets")
			So(found, ShouldNotBeNil)
			So(found.Name, ShouldEqual, "oss")
		})

		Convey("FindExtensionByTool returns nil for unknown tool", func() {
			found := bridge.FindExtensionByTool("oss", "nonexistent")
			So(found, ShouldBeNil)
		})

		Convey("Register syncs default policy to policy registry", func() {
			p, ok := policy.GetDefaultPolicyOf("oss")
			So(ok, ShouldBeTrue)
			cp, ok := p.(*policy.CommandPolicy)
			So(ok, ShouldBeTrue)
			So(cp.Groups, ShouldResemble, []string{"ext:oss:readonly"})
		})

		Convey("Unregister removes from policy registry", func() {
			bridge.Unregister("oss")
			_, ok := policy.GetDefaultPolicyOf("oss")
			So(ok, ShouldBeFalse)
		})

		Convey("Unregister removes extension", func() {
			bridge.Unregister("oss")
			So(bridge.GetAssetTypes(), ShouldBeEmpty)
		})
	})
}
