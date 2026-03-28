package extension

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExtensionHost(t *testing.T) {
	Convey("ExtensionHost", t, func() {
		Convey("LoadPlugin WASM 文件不存在返回错误", func() {
			host := NewExtensionHost(nil)
			_, err := host.LoadPlugin(context.Background(), &ExtensionInfo{
				Manifest: &Manifest{
					Name:    "missing",
					Version: "1.0.0",
					Backend: BackendConfig{Runtime: "wasm", Binary: "main.wasm"},
				},
				Dir: "/nonexistent",
			})
			So(err, ShouldNotBeNil)
		})

		Convey("LoadPlugin 非法 WASM 返回错误", func() {
			tmpDir := t.TempDir()
			So(os.WriteFile(filepath.Join(tmpDir, "bad.wasm"), []byte("not-wasm"), 0o644), ShouldBeNil)

			host := NewExtensionHost(nil)
			_, err := host.LoadPlugin(context.Background(), &ExtensionInfo{
				Manifest: &Manifest{
					Name:    "bad",
					Version: "1.0.0",
					Backend: BackendConfig{Runtime: "wasm", Binary: "bad.wasm"},
				},
				Dir: tmpDir,
			})
			So(err, ShouldNotBeNil)
		})

		Convey("GetPlugin 不存在返回 nil", func() {
			host := NewExtensionHost(nil)
			So(host.GetPlugin("nonexist"), ShouldBeNil)
		})

		Convey("Close 可安全调用（空 host）", func() {
			host := NewExtensionHost(nil)
			host.Close(context.Background()) // should not panic
		})
	})
}
