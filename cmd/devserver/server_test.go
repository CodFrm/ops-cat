// cmd/devserver/server_test.go
package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/opskat/opskat/pkg/extension"

	. "github.com/smartystreets/goconvey/convey"
)

func TestServerAPI(t *testing.T) {
	Convey("DevServer API", t, func() {
		dir := t.TempDir()
		dataDir := filepath.Join(dir, ".devserver")
		_ = os.MkdirAll(dataDir, 0755)

		// Create minimal test manifest
		m := &extension.Manifest{
			Name:    "test-ext",
			Version: "1.0.0",
			Tools: []extension.ToolDef{
				{Name: "echo", I18n: extension.I18nDesc{Description: "Echo tool"}},
			},
		}

		host := NewDevServerHost(dataDir)
		// Plugin is nil for API-only tests (tool/action calls will fail, but API routing works)
		srv := NewServer(m, nil, host, dir, "")

		Convey("GET /api/manifest returns manifest info", func() {
			req := httptest.NewRequest("GET", "/api/manifest", nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, 200)
			var result map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &result)
			So(result["name"], ShouldEqual, "test-ext")
		})

		Convey("GET /api/config reads config file", func() {
			_ = os.WriteFile(filepath.Join(dataDir, "config.json"), []byte(`{"endpoint":"test"}`), 0644)

			req := httptest.NewRequest("GET", "/api/config", nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, 200)
			var result map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &result)
			So(result["endpoint"], ShouldEqual, "test")
		})

		Convey("PUT /api/config writes config file", func() {
			body := bytes.NewReader([]byte(`{"endpoint":"updated"}`))
			req := httptest.NewRequest("PUT", "/api/config", body)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, 200)

			data, _ := os.ReadFile(filepath.Join(dataDir, "config.json")) //nolint:gosec // test file with known path
			var result map[string]any
			_ = json.Unmarshal(data, &result)
			So(result["endpoint"], ShouldEqual, "updated")
		})

		Convey("POST /api/tool/echo returns error when plugin is nil", func() {
			body := bytes.NewReader([]byte(`{"key":"value"}`))
			req := httptest.NewRequest("POST", "/api/tool/echo", body)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, 500)
		})
	})
}
