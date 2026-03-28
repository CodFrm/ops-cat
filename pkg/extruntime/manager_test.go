package extruntime

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestManager(t *testing.T) {
	Convey("Manager", t, func() {
		tmpDir := t.TempDir()

		Convey("Scan", func() {
			Convey("Given 空目录 → 返回空列表", func() {
				mgr := NewManager(tmpDir)
				err := mgr.Scan()
				So(err, ShouldBeNil)
				So(mgr.Extensions(), ShouldBeEmpty)
			})

			Convey("Given 包含合法扩展 → 发现扩展", func() {
				extDir := filepath.Join(tmpDir, "oss")
				So(os.MkdirAll(extDir, 0o755), ShouldBeNil)
				manifest := `{
					"name": "oss",
					"displayName": "Object Storage",
					"version": "1.0.0",
					"backend": {"runtime": "wasm", "binary": "main.wasm"}
				}`
				So(os.WriteFile(filepath.Join(extDir, "manifest.json"), []byte(manifest), 0o644), ShouldBeNil)

				mgr := NewManager(tmpDir)
				err := mgr.Scan()
				So(err, ShouldBeNil)
				So(len(mgr.Extensions()), ShouldEqual, 1)
				So(mgr.Extensions()[0].Manifest.Name, ShouldEqual, "oss")
			})

			Convey("Given 版本不兼容 → 跳过", func() {
				extDir := filepath.Join(tmpDir, "future")
				So(os.MkdirAll(extDir, 0o755), ShouldBeNil)
				manifest := `{
					"name": "future",
					"displayName": "Future",
					"version": "1.0.0",
					"minAppVersion": "99.0.0",
					"backend": {"runtime": "wasm", "binary": "main.wasm"}
				}`
				So(os.WriteFile(filepath.Join(extDir, "manifest.json"), []byte(manifest), 0o644), ShouldBeNil)

				mgr := NewManager(tmpDir, WithAppVersion("1.0.0"))
				err := mgr.Scan()
				So(err, ShouldBeNil)
				So(mgr.Extensions(), ShouldBeEmpty)
			})

			Convey("Given manifest 无效 → 跳过不报错", func() {
				extDir := filepath.Join(tmpDir, "broken")
				So(os.MkdirAll(extDir, 0o755), ShouldBeNil)
				So(os.WriteFile(filepath.Join(extDir, "manifest.json"), []byte("{invalid"), 0o644), ShouldBeNil)

				mgr := NewManager(tmpDir)
				err := mgr.Scan()
				So(err, ShouldBeNil)
				So(mgr.Extensions(), ShouldBeEmpty)
			})

			Convey("Given 目录不存在 → 自动创建", func() {
				nonExist := filepath.Join(tmpDir, "noexist")
				mgr := NewManager(nonExist)
				err := mgr.Scan()
				So(err, ShouldBeNil)
				So(mgr.Extensions(), ShouldBeEmpty)
				info, statErr := os.Stat(nonExist)
				So(statErr, ShouldBeNil)
				So(info.IsDir(), ShouldBeTrue)
			})
		})

		Convey("GetExtension", func() {
			extDir := filepath.Join(tmpDir, "oss")
			So(os.MkdirAll(extDir, 0o755), ShouldBeNil)
			manifest := `{
				"name": "oss",
				"displayName": "Object Storage",
				"version": "1.0.0",
				"backend": {"runtime": "wasm", "binary": "main.wasm"}
			}`
			So(os.WriteFile(filepath.Join(extDir, "manifest.json"), []byte(manifest), 0o644), ShouldBeNil)

			mgr := NewManager(tmpDir)
			So(mgr.Scan(), ShouldBeNil)

			Convey("Given 已扫描 → 按名称查找返回", func() {
				ext := mgr.GetExtension("oss")
				So(ext, ShouldNotBeNil)
				So(ext.Manifest.Name, ShouldEqual, "oss")
			})

			Convey("Given 名称不存在 → 返回 nil", func() {
				So(mgr.GetExtension("nonexist"), ShouldBeNil)
			})
		})

		Convey("Install/Remove", func() {
			extDir := filepath.Join(tmpDir, "extensions")
			srcDir := filepath.Join(tmpDir, "source", "myext")
			So(os.MkdirAll(srcDir, 0o755), ShouldBeNil)
			manifest := `{
				"name": "myext",
				"displayName": "My Extension",
				"version": "0.1.0",
				"backend": {"runtime": "wasm", "binary": "main.wasm"}
			}`
			So(os.WriteFile(filepath.Join(srcDir, "manifest.json"), []byte(manifest), 0o644), ShouldBeNil)

			mgr := NewManager(extDir)

			Convey("Given 合法源目录 → Install 复制 manifest+dist+prompt", func() {
				info, err := mgr.Install(srcDir)
				So(err, ShouldBeNil)
				So(info, ShouldNotBeNil)
				So(info.Manifest.Name, ShouldEqual, "myext")
				So(len(mgr.Extensions()), ShouldEqual, 1)
			})

			Convey("Given 已安装 → Remove 删除并从列表移除", func() {
				_, err := mgr.Install(srcDir)
				So(err, ShouldBeNil)
				So(len(mgr.Extensions()), ShouldEqual, 1)

				err = mgr.Remove("myext")
				So(err, ShouldBeNil)
				So(mgr.Extensions(), ShouldBeEmpty)
			})

			Convey("Given 不存在 → Remove 返回错误", func() {
				err := mgr.Remove("nonexist")
				So(err, ShouldNotBeNil)
			})
		})

		Convey("Logger", func() {
			Convey("Given nil logger → Scan 跳过无效扩展不 panic", func() {
				extDir := filepath.Join(tmpDir, "broken2")
				So(os.MkdirAll(extDir, 0o755), ShouldBeNil)
				So(os.WriteFile(filepath.Join(extDir, "manifest.json"), []byte("{bad"), 0o644), ShouldBeNil)

				mgr := NewManager(tmpDir) // no WithLogger
				So(func() { mgr.Scan() }, ShouldNotPanic)
			})
		})
	})
}
