package extension

import (
	"encoding/json"
	"testing"

	"github.com/opskat/opskat/internal/ai"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBridge(t *testing.T) {
	Convey("Bridge", t, func() {
		Convey("MergeToolDefs 合并扩展工具到内置工具列表", func() {
			builtinTools := []ai.ToolDef{
				{Name: "list_assets", Description: "List assets"},
			}

			extInfo := &ExtensionInfo{
				Manifest: &Manifest{
					Name: "oss",
					Tools: []ToolDef{
						{
							Name:        "list_buckets",
							Description: "列出存储桶",
							Parameters:  json.RawMessage(`{"type":"object","properties":{"prefix":{"type":"string"}}}`),
						},
						{
							Name:        "upload_object",
							Description: "上传对象",
							Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
						},
					},
				},
			}

			bridge := NewBridge(nil)
			bridge.RegisterExtension(extInfo)
			merged := bridge.MergeToolDefs(builtinTools)

			So(len(merged), ShouldEqual, 3)
			So(merged[0].Name, ShouldEqual, "list_assets")
			So(merged[1].Name, ShouldEqual, "oss.list_buckets")
			So(merged[2].Name, ShouldEqual, "oss.upload_object")
		})

		Convey("MergeToolDefs 无扩展时返回原始列表", func() {
			builtinTools := []ai.ToolDef{
				{Name: "list_assets", Description: "List assets"},
			}
			bridge := NewBridge(nil)
			merged := bridge.MergeToolDefs(builtinTools)
			So(len(merged), ShouldEqual, 1)
		})

		Convey("ExtensionNames 返回已注册扩展名列表", func() {
			bridge := NewBridge(nil)
			bridge.RegisterExtension(&ExtensionInfo{
				Manifest: &Manifest{Name: "oss"},
			})
			bridge.RegisterExtension(&ExtensionInfo{
				Manifest: &Manifest{Name: "k8s"},
			})
			names := bridge.ExtensionNames()
			So(len(names), ShouldEqual, 2)
			So(names, ShouldContain, "oss")
			So(names, ShouldContain, "k8s")
		})
	})
}

func TestParseExtensionToolName(t *testing.T) {
	Convey("ParseExtensionToolName", t, func() {
		Convey("解析合法的扩展工具名", func() {
			ext, tool, ok := ParseExtensionToolName("oss.list_buckets")
			So(ok, ShouldBeTrue)
			So(ext, ShouldEqual, "oss")
			So(tool, ShouldEqual, "list_buckets")
		})

		Convey("内置工具名返回 false", func() {
			_, _, ok := ParseExtensionToolName("list_assets")
			So(ok, ShouldBeFalse)
		})

		Convey("多级点号只分割第一个", func() {
			ext, tool, ok := ParseExtensionToolName("oss.nested.tool")
			So(ok, ShouldBeTrue)
			So(ext, ShouldEqual, "oss")
			So(tool, ShouldEqual, "nested.tool")
		})
	})
}
