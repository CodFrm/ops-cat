package extension

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
)

func TestManager(t *testing.T) {
	Convey("Manager", t, func() {
		ctx := context.Background()
		dir := t.TempDir()
		logger := zap.NewNop()

		newHost := func(extName string) HostProvider {
			return NewDefaultHostProvider(DefaultHostConfig{Logger: logger})
		}

		mgr := NewManager(dir, newHost, logger)

		Convey("Scan with no extensions", func() {
			exts, err := mgr.Scan(ctx)
			So(err, ShouldBeNil)
			So(exts, ShouldBeEmpty)
		})

		Convey("Scan discovers valid extension", func() {
			extDir := filepath.Join(dir, "test-ext")
			os.MkdirAll(extDir, 0755)

			manifest := map[string]any{
				"name":    "test-ext",
				"version": "1.0.0",
				"backend": map[string]any{"runtime": "wasm", "binary": "main.wasm"},
			}
			data, _ := json.Marshal(manifest)
			os.WriteFile(filepath.Join(extDir, "manifest.json"), data, 0644)

			// Minimal valid WASM module
			os.WriteFile(filepath.Join(extDir, "main.wasm"),
				[]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, 0644)

			exts, err := mgr.Scan(ctx)
			So(err, ShouldBeNil)
			So(len(exts), ShouldEqual, 1)
			So(exts[0].Name, ShouldEqual, "test-ext")
		})

		Convey("Scan skips directories without manifest", func() {
			os.MkdirAll(filepath.Join(dir, "no-manifest"), 0755)

			exts, err := mgr.Scan(ctx)
			So(err, ShouldBeNil)
			So(exts, ShouldBeEmpty)
		})

		Convey("GetExtension returns nil for unknown", func() {
			ext := mgr.GetExtension("nonexistent")
			So(ext, ShouldBeNil)
		})

		Reset(func() {
			mgr.Close(ctx)
		})
	})
}
