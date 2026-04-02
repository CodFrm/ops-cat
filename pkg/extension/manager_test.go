package extension

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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
			_ = os.MkdirAll(extDir, 0755)

			manifest := map[string]any{
				"name":    "test-ext",
				"version": "1.0.0",
				"backend": map[string]any{"runtime": "wasm", "binary": "main.wasm"},
			}
			data, _ := json.Marshal(manifest)
			So(os.WriteFile(filepath.Join(extDir, "manifest.json"), data, 0644), ShouldBeNil)

			// Minimal valid WASM module
			So(os.WriteFile(filepath.Join(extDir, "main.wasm"),
				[]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, 0644), ShouldBeNil)

			exts, err := mgr.Scan(ctx)
			So(err, ShouldBeNil)
			So(len(exts), ShouldEqual, 1)
			So(exts[0].Name, ShouldEqual, "test-ext")
		})

		Convey("Scan skips directories without manifest", func() {
			_ = os.MkdirAll(filepath.Join(dir, "no-manifest"), 0755)

			exts, err := mgr.Scan(ctx)
			So(err, ShouldBeNil)
			So(exts, ShouldBeEmpty)
		})

		Convey("GetExtension returns nil for unknown", func() {
			ext := mgr.GetExtension("nonexistent")
			So(ext, ShouldBeNil)
		})

		Convey("Watch calls onChange on filesystem event", func() {
			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			called := make(chan struct{}, 1)
			err := mgr.Watch(watchCtx, func() {
				select {
				case called <- struct{}{}:
				default:
				}
			})
			So(err, ShouldBeNil)

			// Trigger a filesystem event
			So(os.WriteFile(filepath.Join(dir, "trigger.txt"), []byte("x"), 0644), ShouldBeNil)

			select {
			case <-called:
				// success
			case <-time.After(2 * time.Second):
				So("onChange not called", ShouldEqual, "")
			}
		})

		Reset(func() {
			mgr.Close(ctx)
		})
	})
}
