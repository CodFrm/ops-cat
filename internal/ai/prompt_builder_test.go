package ai

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPromptBuilderBuild(t *testing.T) {
	Convey("PromptBuilder.Build", t, func() {
		Convey("无 OpenTabs 时仍包含语言提示和角色描述", func() {
			got := NewPromptBuilder("zh-cn", AIContext{}).Build()
			So(got, ShouldContainSubstring, "OpsKat AI assistant")
			So(got, ShouldContainSubstring, "Chinese")
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
