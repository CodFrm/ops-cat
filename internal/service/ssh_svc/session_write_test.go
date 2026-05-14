package ssh_svc

import (
	"bytes"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// fakeStdin 用作 Session.stdin 替身，记录所有写入字节。
type fakeStdin struct{ bytes.Buffer }

func (f *fakeStdin) Close() error { return nil }

// 断言：Session 配置了 charset 后，Session.Write 把前端的 UTF-8 字节按目标字符集
// 编码后再写到底层 stdin。这是 #114 修复的入口（用户输入路径）。
func TestSessionWrite_EncodesToRemoteCharset(t *testing.T) {
	Convey("Session.Write 按 charset 编码后再下发", t, func() {
		Convey("charset=gbk 时 '中' (UTF-8) → GBK 字节 D6 D0", func() {
			stdin := &fakeStdin{}
			sess := &Session{
				ID:      "test",
				charset: "gbk",
				stdin:   io.WriteCloser(stdin),
			}
			err := sess.Write([]byte("中"))
			So(err, ShouldBeNil)
			So(stdin.Bytes(), ShouldResemble, []byte{0xD6, 0xD0})
		})

		Convey("charset='' 时按原样写入（回归保险）", func() {
			stdin := &fakeStdin{}
			sess := &Session{
				ID:      "test",
				charset: "",
				stdin:   io.WriteCloser(stdin),
			}
			err := sess.Write([]byte("中"))
			So(err, ShouldBeNil)
			So(stdin.Bytes(), ShouldResemble, []byte("中"))
		})

		Convey("charset=gbk 时 ASCII 控制字符（CR）原样下发", func() {
			stdin := &fakeStdin{}
			sess := &Session{
				ID:      "test",
				charset: "gbk",
				stdin:   io.WriteCloser(stdin),
			}
			err := sess.Write([]byte("ls\r"))
			So(err, ShouldBeNil)
			So(stdin.Bytes(), ShouldResemble, []byte("ls\r"))
		})

		Convey("会话已关闭时 Write 返回错误且不写入", func() {
			stdin := &fakeStdin{}
			sess := &Session{
				ID:      "test",
				charset: "gbk",
				stdin:   io.WriteCloser(stdin),
				closed:  true,
			}
			err := sess.Write([]byte("中"))
			So(err, ShouldNotBeNil)
			So(stdin.Len(), ShouldEqual, 0)
		})
	})
}
