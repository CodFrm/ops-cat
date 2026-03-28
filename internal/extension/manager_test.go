package extension

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestManager(t *testing.T) {
	Convey("Manager", t, func() {
		tmpDir := t.TempDir()

		Convey("Scan 空目录返回空列表", func() {
			mgr := NewManager(tmpDir, "1.0.0")
			err := mgr.Scan()
			So(err, ShouldBeNil)
			So(mgr.Extensions(), ShouldBeEmpty)
		})

		Convey("Scan 发现合法扩展", func() {
			extDir := filepath.Join(tmpDir, "oss")
			So(os.MkdirAll(extDir, 0o755), ShouldBeNil)
			manifest := `{
				"name": "oss",
				"displayName": "Object Storage",
				"version": "1.0.0",
				"minAppVersion": "1.0.0",
				"backend": {"runtime": "wasm", "binary": "main.wasm"}
			}`
			So(os.WriteFile(filepath.Join(extDir, "manifest.json"), []byte(manifest), 0o644), ShouldBeNil)
			So(os.WriteFile(filepath.Join(extDir, "main.wasm"), []byte("fake"), 0o644), ShouldBeNil)

			mgr := NewManager(tmpDir, "1.0.0")
			err := mgr.Scan()
			So(err, ShouldBeNil)
			So(len(mgr.Extensions()), ShouldEqual, 1)
			So(mgr.Extensions()[0].Manifest.Name, ShouldEqual, "oss")
		})

		Convey("Scan 跳过版本不兼容的扩展", func() {
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

			mgr := NewManager(tmpDir, "1.0.0")
			err := mgr.Scan()
			So(err, ShouldBeNil)
			So(mgr.Extensions(), ShouldBeEmpty)
		})

		Convey("Scan 跳过 manifest 无效的目录", func() {
			extDir := filepath.Join(tmpDir, "broken")
			So(os.MkdirAll(extDir, 0o755), ShouldBeNil)
			So(os.WriteFile(filepath.Join(extDir, "manifest.json"), []byte("{invalid"), 0o644), ShouldBeNil)

			mgr := NewManager(tmpDir, "1.0.0")
			err := mgr.Scan()
			So(err, ShouldBeNil)
			So(mgr.Extensions(), ShouldBeEmpty)
		})

		Convey("Scan 目录不存在时自动创建", func() {
			nonExist := filepath.Join(tmpDir, "noexist")
			mgr := NewManager(nonExist, "1.0.0")
			err := mgr.Scan()
			So(err, ShouldBeNil)
			So(mgr.Extensions(), ShouldBeEmpty)
			info, statErr := os.Stat(nonExist)
			So(statErr, ShouldBeNil)
			So(info.IsDir(), ShouldBeTrue)
		})

		Convey("GetExtension 按名称查找", func() {
			extDir := filepath.Join(tmpDir, "oss")
			So(os.MkdirAll(extDir, 0o755), ShouldBeNil)
			manifest := `{
				"name": "oss",
				"displayName": "Object Storage",
				"version": "1.0.0",
				"backend": {"runtime": "wasm", "binary": "main.wasm"}
			}`
			So(os.WriteFile(filepath.Join(extDir, "manifest.json"), []byte(manifest), 0o644), ShouldBeNil)

			mgr := NewManager(tmpDir, "1.0.0")
			So(mgr.Scan(), ShouldBeNil)

			ext := mgr.GetExtension("oss")
			So(ext, ShouldNotBeNil)
			So(ext.Manifest.Name, ShouldEqual, "oss")

			missing := mgr.GetExtension("nonexist")
			So(missing, ShouldBeNil)
		})
	})
}

func TestManagerInstallRemove(t *testing.T) {
	Convey("Manager Install/Remove", t, func() {
		tmpDir := t.TempDir()
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
		So(os.WriteFile(filepath.Join(srcDir, "main.wasm"), []byte("fake"), 0o644), ShouldBeNil)

		mgr := NewManager(extDir, "1.0.0")

		Convey("Install 安装扩展", func() {
			info, err := mgr.Install(srcDir)
			So(err, ShouldBeNil)
			So(info, ShouldNotBeNil)
			So(info.Manifest.Name, ShouldEqual, "myext")
			So(len(mgr.Extensions()), ShouldEqual, 1)
		})

		Convey("Remove 卸载扩展", func() {
			_, err := mgr.Install(srcDir)
			So(err, ShouldBeNil)
			So(len(mgr.Extensions()), ShouldEqual, 1)

			err = mgr.Remove("myext")
			So(err, ShouldBeNil)
			So(mgr.Extensions(), ShouldBeEmpty)
		})

		Convey("Remove 不存在的扩展返回错误", func() {
			err := mgr.Remove("nonexist")
			So(err, ShouldNotBeNil)
		})
	})
}
