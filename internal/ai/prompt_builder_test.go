package ai

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRenderMentionContext(t *testing.T) {
	Convey("RenderMentionContext", t, func() {
		Convey("空切片返回空串", func() {
			So(RenderMentionContext(nil), ShouldEqual, "")
			So(RenderMentionContext([]MentionedAsset{}), ShouldEqual, "")
		})

		Convey("单个资产渲染", func() {
			got := RenderMentionContext([]MentionedAsset{
				{AssetID: 42, Name: "prod-db", Type: "mysql", Host: "10.0.0.5", GroupPath: "生产/数据库"},
			})
			So(got, ShouldContainSubstring, "Assets referenced in the user's message")
			So(got, ShouldContainSubstring, "@prod-db")
			So(got, ShouldContainSubstring, "ID=42")
			So(got, ShouldContainSubstring, "type=mysql")
			So(got, ShouldContainSubstring, "host=10.0.0.5")
			So(got, ShouldContainSubstring, "group=生产/数据库")
		})

		Convey("多个资产每项独占一行", func() {
			got := RenderMentionContext([]MentionedAsset{
				{AssetID: 42, Name: "a", Type: "ssh", Host: "1.1.1.1"},
				{AssetID: 43, Name: "b", Type: "redis", Host: "2.2.2.2"},
			})
			bulletCount := 0
			for _, l := range strings.Split(got, "\n") {
				if strings.HasPrefix(strings.TrimSpace(l), "- @") {
					bulletCount++
				}
			}
			So(bulletCount, ShouldEqual, 2)
		})

		Convey("GroupPath 为空不输出 group 字段", func() {
			got := RenderMentionContext([]MentionedAsset{
				{AssetID: 42, Name: "x", Type: "ssh", Host: "1.1.1.1"},
			})
			So(got, ShouldNotContainSubstring, "group=")
		})
	})
}
