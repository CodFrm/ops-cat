// cmd/devserver/host_test.go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDevServerHostProvider(t *testing.T) {
	Convey("DevServerHostProvider", t, func() {
		dir := t.TempDir()

		Convey("GetAssetConfig reads from config file", func() {
			cfgFile := filepath.Join(dir, "config.json")
			_ = os.WriteFile(cfgFile, []byte(`{"endpoint":"https://oss.example.com"}`), 0644)

			h := NewDevServerHost(dir)
			defer h.CloseAll()

			cfg, err := h.GetAssetConfig(0)
			So(err, ShouldBeNil)

			var out map[string]string
			_ = json.Unmarshal(cfg, &out)
			So(out["endpoint"], ShouldEqual, "https://oss.example.com")
		})

		Convey("KVGet/KVSet round-trips", func() {
			h := NewDevServerHost(dir)
			defer h.CloseAll()

			err := h.KVSet("test-key", []byte("test-value"))
			So(err, ShouldBeNil)

			val, err := h.KVGet("test-key")
			So(err, ShouldBeNil)
			So(string(val), ShouldEqual, "test-value")
		})

		Convey("Log does not panic", func() {
			h := NewDevServerHost(dir)
			defer h.CloseAll()
			h.Log("info", "test log message")
		})
	})
}
