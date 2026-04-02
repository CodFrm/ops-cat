// pkg/extension/io_handle_test.go
package extension

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIOHandleManager(t *testing.T) {
	Convey("IOHandleManager", t, func() {
		mgr := NewIOHandleManager()
		defer mgr.CloseAll()

		Convey("file read handle", func() {
			dir := t.TempDir()
			path := filepath.Join(dir, "test.txt")
			So(os.WriteFile(path, []byte("hello world"), 0644), ShouldBeNil)

			h, meta, err := mgr.OpenFile(path, "read")
			So(err, ShouldBeNil)
			So(h, ShouldBeGreaterThan, 0)
			So(meta.Size, ShouldEqual, 11)

			buf := make([]byte, 5)
			n, err := mgr.Read(h, buf)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 5)
			So(string(buf), ShouldEqual, "hello")

			buf2 := make([]byte, 20)
			n2, err := mgr.Read(h, buf2)
			So(err, ShouldBeNil)
			So(n2, ShouldEqual, 6)
			So(string(buf2[:n2]), ShouldEqual, " world")

			So(mgr.Close(h), ShouldBeNil)
		})

		Convey("file write handle", func() {
			dir := t.TempDir()
			path := filepath.Join(dir, "out.txt")

			h, _, err := mgr.OpenFile(path, "write")
			So(err, ShouldBeNil)

			n, err := mgr.Write(h, []byte("written"))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 7)

			So(mgr.Close(h), ShouldBeNil)

			data, _ := os.ReadFile(path) //nolint:gosec // test file with known path
			So(string(data), ShouldEqual, "written")
		})

		Convey("close invalid handle returns error", func() {
			err := mgr.Close(9999)
			So(err, ShouldNotBeNil)
		})

		Convey("CloseAll closes all handles", func() {
			dir := t.TempDir()
			path := filepath.Join(dir, "a.txt")
			So(os.WriteFile(path, []byte("a"), 0644), ShouldBeNil)

			h, _, _ := mgr.OpenFile(path, "read")
			mgr.CloseAll()

			_, err := mgr.Read(h, make([]byte, 1))
			So(err, ShouldNotBeNil)
		})
	})
}
