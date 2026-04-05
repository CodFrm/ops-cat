// pkg/extension/host_default_test.go
package extension

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
)

func TestDefaultHostProvider(t *testing.T) {
	Convey("DefaultHostProvider", t, func() {
		logger, _ := zap.NewDevelopment()
		host := NewDefaultHostProvider(DefaultHostConfig{
			Logger: logger,
		})
		defer host.CloseAll()

		Convey("IOOpen file read", func() {
			dir := t.TempDir()
			path := filepath.Join(dir, "test.txt")
			So(os.WriteFile(path, []byte("content"), 0644), ShouldBeNil)

			id, meta, err := host.IOOpen(IOOpenParams{Type: "file", Path: path, Mode: "read"})
			So(err, ShouldBeNil)
			So(id, ShouldBeGreaterThan, 0)
			So(meta.Size, ShouldEqual, 7)

			data, err := host.IORead(id, 100)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "content")

			So(host.IOClose(id), ShouldBeNil)
		})

		Convey("IOOpen file write", func() {
			dir := t.TempDir()
			path := filepath.Join(dir, "out.txt")

			id, _, err := host.IOOpen(IOOpenParams{Type: "file", Path: path, Mode: "write"})
			So(err, ShouldBeNil)

			n, err := host.IOWrite(id, []byte("output"))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 6)

			So(host.IOClose(id), ShouldBeNil)
			data, _ := os.ReadFile(path) //nolint:gosec // test file with known path
			So(string(data), ShouldEqual, "output")
		})

		Convey("IOOpen unknown type returns error", func() {
			_, _, err := host.IOOpen(IOOpenParams{Type: "unknown"})
			So(err, ShouldNotBeNil)
		})

		Convey("Log does not panic", func() {
			host.Log("info", "test message")
			host.Log("error", "test error")
		})

		Convey("unconfigured services return errors", func() {
			_, err := host.GetAssetConfig(1)
			So(err, ShouldNotBeNil)

			_, err = host.KVGet("key")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestDefaultHostProvider_IOReadEOF(t *testing.T) {
	Convey("IORead preserves data when underlying reader returns (n, io.EOF)", t, func() {
		// Simulate a small HTTP response where Read returns data+EOF in one call.
		// This is the exact scenario that caused "deserialization failed, failed to copy error response body, EOF".
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `<?xml version="1.0"?><Error><Code>AccessDenied</Code></Error>`)
		}))
		defer srv.Close()

		logger, _ := zap.NewDevelopment()
		host := NewDefaultHostProvider(DefaultHostConfig{Logger: logger})
		defer host.CloseAll()

		id, _, err := host.IOOpen(IOOpenParams{
			Type:         "http",
			Method:       "GET",
			URL:          srv.URL,
			AllowPrivate: true, // httptest server binds to loopback
		})
		So(err, ShouldBeNil)

		meta, err := host.IOFlush(id)
		So(err, ShouldBeNil)
		So(meta.Status, ShouldEqual, 403)

		// Read entire response body through IORead
		var body []byte
		for {
			data, err := host.IORead(id, 4096)
			if len(data) > 0 {
				body = append(body, data...)
			}
			if err != nil {
				So(err, ShouldEqual, io.EOF)
				break
			}
		}

		So(string(body), ShouldContainSubstring, "AccessDenied")

		So(host.IOClose(id), ShouldBeNil)
	})
}
