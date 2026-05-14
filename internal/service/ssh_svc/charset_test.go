package ssh_svc

import (
	"bytes"
	"io"
	"testing"
	"testing/iotest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecodeStream(t *testing.T) {
	Convey("decodeStream", t, func() {
		Convey("空 charset 直通", func() {
			r := decodeStream("", bytes.NewReader([]byte{0xD6, 0xD0}))
			out, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []byte{0xD6, 0xD0})
		})

		Convey("utf-8 直通", func() {
			r := decodeStream("utf-8", bytes.NewReader([]byte("中")))
			out, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "中")
		})

		Convey("GBK 双字节汉字解码为 UTF-8", func() {
			r := decodeStream("gbk", bytes.NewReader([]byte{0xD6, 0xD0})) // "中"
			out, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "中")
		})

		Convey("GBK 字符被极端分片（每次只读 1 字节）仍能解码", func() {
			// "中文" GBK = D6 D0 CE C4
			src := iotest.OneByteReader(bytes.NewReader([]byte{0xD6, 0xD0, 0xCE, 0xC4}))
			r := decodeStream("gbk", src)
			out, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "中文")
		})

		Convey("未知 charset 退化为直通而不是 panic", func() {
			r := decodeStream("klingon", bytes.NewReader([]byte{0xD6, 0xD0}))
			out, err := io.ReadAll(r)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []byte{0xD6, 0xD0})
		})
	})
}

func TestEncodeForRemote(t *testing.T) {
	Convey("encodeForRemote", t, func() {
		Convey("空 charset 直通", func() {
			got, err := encodeForRemote("", []byte("中"))
			So(err, ShouldBeNil)
			So(got, ShouldResemble, []byte("中"))
		})

		Convey("GBK 把 UTF-8 转成 GBK 字节", func() {
			got, err := encodeForRemote("gbk", []byte("中"))
			So(err, ShouldBeNil)
			So(got, ShouldResemble, []byte{0xD6, 0xD0})
		})

		Convey("GBK 上的不可表示字符（emoji）被替换且不报错", func() {
			got, err := encodeForRemote("gbk", []byte("😀"))
			So(err, ShouldBeNil)
			// ReplaceUnsupported 用 SUB 字符 (0x1A) 替换；只要不报错且产物非空即可。
			So(len(got), ShouldBeGreaterThan, 0)
			So(got, ShouldResemble, []byte{0x1A})
		})

		Convey("ASCII 在 GBK 下不变", func() {
			got, err := encodeForRemote("gbk", []byte("hello\r\n"))
			So(err, ShouldBeNil)
			So(got, ShouldResemble, []byte("hello\r\n"))
		})

		Convey("未知 charset 退化为直通", func() {
			got, err := encodeForRemote("klingon", []byte("中"))
			So(err, ShouldBeNil)
			So(got, ShouldResemble, []byte("中"))
		})
	})
}
