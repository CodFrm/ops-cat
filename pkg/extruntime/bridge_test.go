package extruntime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBridgeExecuteTool(t *testing.T) {
	Convey("Bridge.ExecuteTool", t, func() {
		Convey("Given 未注册的扩展 → 返回错误", func() {
			bridge := NewBridge(nil)
			result, err := bridge.ExecuteTool(context.Background(), "nonexist", "tool", json.RawMessage("{}"))
			So(err, ShouldNotBeNil)
			So(result, ShouldEqual, "")
			So(err.Error(), ShouldContainSubstring, "not registered")
		})

		Convey("Given 不存在的工具 → 返回错误", func() {
			ext := &ExtensionInfo{
				Manifest: &Manifest{
					Name:    "oss",
					Version: "1.0.0",
					Backend: BackendConfig{Runtime: "wasm", Binary: "main.wasm"},
					Tools:   []ToolDef{{Name: "list_buckets", Description: "List"}},
				},
			}
			bridge := NewBridge(nil)
			bridge.RegisterExtension(ext)
			_, err := bridge.ExecuteTool(context.Background(), "oss", "nonexist", json.RawMessage("{}"))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not found")
		})

		Convey("Given host 未初始化 → 返回错误", func() {
			ext := &ExtensionInfo{
				Manifest: &Manifest{
					Name:    "oss",
					Version: "1.0.0",
					Backend: BackendConfig{Runtime: "wasm", Binary: "main.wasm"},
					Tools:   []ToolDef{{Name: "list_buckets", Description: "List"}},
				},
			}
			bridge := NewBridge(nil)
			bridge.RegisterExtension(ext)
			_, err := bridge.ExecuteTool(context.Background(), "oss", "list_buckets", json.RawMessage("{}"))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "host not initialized")
		})
	})
}

func TestBridgeExtensionNames(t *testing.T) {
	Convey("Bridge.ExtensionNames", t, func() {
		bridge := NewBridge(nil)
		bridge.RegisterExtension(&ExtensionInfo{
			Manifest: &Manifest{Name: "oss", Version: "1.0.0", Backend: BackendConfig{Runtime: "wasm", Binary: "m.wasm"}},
		})
		bridge.RegisterExtension(&ExtensionInfo{
			Manifest: &Manifest{Name: "k8s", Version: "1.0.0", Backend: BackendConfig{Runtime: "wasm", Binary: "m.wasm"}},
		})
		names := bridge.ExtensionNames()
		So(len(names), ShouldEqual, 2)
		So(names, ShouldContain, "oss")
		So(names, ShouldContain, "k8s")
	})
}

func TestBridgeGetExtensionPrompts(t *testing.T) {
	Convey("Bridge.GetExtensionPrompts", t, func() {
		Convey("Given 无扩展 → 返回空", func() {
			bridge := NewBridge(nil)
			So(bridge.GetExtensionPrompts(), ShouldEqual, "")
		})

		Convey("Given 扩展有 prompt_file → 加载文件内容", func() {
			tmpDir := t.TempDir()
			promptContent := "Custom prompt for OSS."
			So(os.WriteFile(filepath.Join(tmpDir, "prompt.md"), []byte(promptContent), 0o644), ShouldBeNil)

			ext := &ExtensionInfo{
				Manifest: &Manifest{
					Name:       "oss",
					Version:    "1.0.0",
					Backend:    BackendConfig{Runtime: "wasm", Binary: "m.wasm"},
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

		Convey("Given 扩展无 prompt_file 但有工具 → 自动生成", func() {
			ext := &ExtensionInfo{
				Manifest: &Manifest{
					Name:        "oss",
					Version:     "1.0.0",
					Backend:     BackendConfig{Runtime: "wasm", Binary: "m.wasm"},
					Description: "OSS extension",
					Tools: []ToolDef{
						{Name: "list_buckets", Description: "列出存储桶"},
						{Name: "upload", Description: "上传"},
					},
				},
				Dir: t.TempDir(),
			}
			bridge := NewBridge(nil)
			bridge.RegisterExtension(ext)
			prompts := bridge.GetExtensionPrompts()
			So(prompts, ShouldContainSubstring, "list_buckets")
			So(prompts, ShouldContainSubstring, "upload")
			So(prompts, ShouldContainSubstring, "OSS extension")
		})

		Convey("Given 扩展无工具无 prompt → 跳过", func() {
			ext := &ExtensionInfo{
				Manifest: &Manifest{
					Name:    "empty",
					Version: "1.0.0",
					Backend: BackendConfig{Runtime: "wasm", Binary: "m.wasm"},
				},
				Dir: t.TempDir(),
			}
			bridge := NewBridge(nil)
			bridge.RegisterExtension(ext)
			So(bridge.GetExtensionPrompts(), ShouldEqual, "")
		})
	})
}

func TestParseToolName(t *testing.T) {
	Convey("ParseToolName", t, func() {
		Convey("Given oss.list_buckets → (oss, list_buckets, true)", func() {
			ext, tool, ok := ParseToolName("oss.list_buckets")
			So(ok, ShouldBeTrue)
			So(ext, ShouldEqual, "oss")
			So(tool, ShouldEqual, "list_buckets")
		})

		Convey("Given list_assets → false", func() {
			_, _, ok := ParseToolName("list_assets")
			So(ok, ShouldBeFalse)
		})

		Convey("Given oss.nested.tool → (oss, nested.tool, true)", func() {
			ext, tool, ok := ParseToolName("oss.nested.tool")
			So(ok, ShouldBeTrue)
			So(ext, ShouldEqual, "oss")
			So(tool, ShouldEqual, "nested.tool")
		})
	})
}
