package ai

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPromptBuilderBuild(t *testing.T) {
	Convey("PromptBuilder.Build", t, func() {
		Convey("无 OpenTabs 时输出语言提示（角色描述已搬到 system_template.go）", func() {
			got := NewPromptBuilder("zh-cn", AIContext{}).Build()
			So(got, ShouldContainSubstring, "Chinese")
			So(got, ShouldNotContainSubstring, "OpsKat AI assistant")
		})

		Convey("OpenTabs 渲染包含每个 tab 名和 ID", func() {
			got := NewPromptBuilder("en", AIContext{
				OpenTabs: []TabInfo{
					{Type: "ssh", AssetID: 42, AssetName: "prod-db"},
					{Type: "database", AssetID: 43, AssetName: "metrics"},
				},
			}).Build()
			So(got, ShouldContainSubstring, "SSH Terminal")
			So(got, ShouldContainSubstring, "prod-db")
			So(got, ShouldContainSubstring, "Database Query")
			So(got, ShouldContainSubstring, "metrics")
		})

		Convey("Extension SKILL.md 被注入", func() {
			b := NewPromptBuilder("en", AIContext{})
			b.SetExtensionSkillMDs(map[string]string{"k8s": "k8s skill body"})
			got := b.Build()
			So(got, ShouldContainSubstring, "From extension: k8s")
			So(got, ShouldContainSubstring, "k8s skill body")
		})
	})
}
