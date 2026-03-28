package extension

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBridge(t *testing.T) {
	Convey("Bridge", t, func() {
		Convey("ExecuteTool 扩展未注册时返回错误", func() {
			bridge := NewBridge(nil)
			_, err := bridge.ExecuteTool(t.Context(), "nonexistent", "some_tool", json.RawMessage("{}"))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not registered")
		})

		Convey("ExecuteTool 工具不存在时返回错误", func() {
			ext := &ExtensionInfo{
				Manifest: &Manifest{
					Name: "oss",
					Tools: []ToolDef{
						{Name: "list_buckets", Description: "List buckets"},
					},
				},
			}
			bridge := NewBridge(nil)
			bridge.RegisterExtension(ext)
			_, err := bridge.ExecuteTool(t.Context(), "oss", "nonexistent_tool", json.RawMessage("{}"))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not found")
		})

		Convey("ExecuteTool 插件未加载时返回错误", func() {
			ext := &ExtensionInfo{
				Manifest: &Manifest{
					Name: "oss",
					Tools: []ToolDef{
						{Name: "list_buckets", Description: "List buckets"},
					},
				},
			}
			bridge := NewBridge(nil)
			bridge.RegisterExtension(ext)
			_, err := bridge.ExecuteTool(t.Context(), "oss", "list_buckets", json.RawMessage("{}"))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "host not initialized")
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

func TestGetExtensionPrompts(t *testing.T) {
	Convey("GetExtensionPrompts", t, func() {
		Convey("无扩展时返回空字符串", func() {
			bridge := NewBridge(nil)
			So(bridge.GetExtensionPrompts(), ShouldEqual, "")
		})

		Convey("从 prompt_file 加载", func() {
			tmpDir := t.TempDir()
			promptContent := "This is a custom prompt for the OSS extension."
			err := os.WriteFile(filepath.Join(tmpDir, "prompt.md"), []byte(promptContent), 0o644)
			So(err, ShouldBeNil)

			ext := &ExtensionInfo{
				Manifest: &Manifest{
					Name:       "oss",
					PromptFile: "prompt.md",
				},
				Dir: tmpDir,
			}
			bridge := NewBridge(nil)
			bridge.RegisterExtension(ext)
			prompts := bridge.GetExtensionPrompts()
			So(prompts, ShouldContainSubstring, "Extensions")
			So(prompts, ShouldContainSubstring, "oss")
			So(prompts, ShouldContainSubstring, promptContent)
		})

		Convey("自动从工具定义生成 prompt", func() {
			ext := &ExtensionInfo{
				Manifest: &Manifest{
					Name:        "oss",
					Description: "OSS extension",
					Tools: []ToolDef{
						{
							Name:        "list_buckets",
							Description: "列出存储桶",
							Parameters:  json.RawMessage(`{"type":"object","properties":{"prefix":{"type":"string"}}}`),
						},
						{
							Name:        "upload_object",
							Description: "上传对象",
						},
					},
				},
				Dir: t.TempDir(),
			}
			bridge := NewBridge(nil)
			bridge.RegisterExtension(ext)
			prompts := bridge.GetExtensionPrompts()
			So(prompts, ShouldContainSubstring, "Extensions")
			So(prompts, ShouldContainSubstring, "list_buckets")
			So(prompts, ShouldContainSubstring, "upload_object")
			So(prompts, ShouldContainSubstring, "OSS extension")
		})

		Convey("扩展无工具且无 prompt_file 时跳过", func() {
			ext := &ExtensionInfo{
				Manifest: &Manifest{
					Name: "empty_ext",
				},
				Dir: t.TempDir(),
			}
			bridge := NewBridge(nil)
			bridge.RegisterExtension(ext)
			prompts := bridge.GetExtensionPrompts()
			So(prompts, ShouldEqual, "")
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
