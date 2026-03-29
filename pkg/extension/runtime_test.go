// pkg/extension/runtime_test.go
package extension

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
)

func TestLoadPlugin(t *testing.T) {
	Convey("LoadPlugin", t, func() {
		ctx := context.Background()
		host := NewDefaultHostProvider(DefaultHostConfig{
			Logger: zap.NewNop(),
		})
		defer host.CloseAll()

		Convey("should reject invalid WASM bytes", func() {
			manifest := &Manifest{Name: "test", Version: "1.0.0"}
			_, err := LoadPlugin(ctx, manifest, []byte("not wasm"), host)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "compile wasm")
		})

		Convey("should load minimal valid WASM module", func() {
			// Minimal valid WASM module (magic + version, empty)
			minimalWasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
			manifest := &Manifest{Name: "test", Version: "1.0.0"}
			p, err := LoadPlugin(ctx, manifest, minimalWasm, host)
			So(err, ShouldBeNil)
			So(p, ShouldNotBeNil)
			So(p.Manifest().Name, ShouldEqual, "test")
			So(p.Close(ctx), ShouldBeNil)
		})
	})
}
