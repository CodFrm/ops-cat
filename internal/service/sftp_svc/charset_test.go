package sftp_svc

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// 这两个函数是 #114 文件名乱码修复的核心：所有发往远端的路径要从 UTF-8 编到目标字符集，
// 所有从远端 ReadDir 出来的名字要从目标字符集解到 UTF-8。

func TestEncodePath(t *testing.T) {
	Convey("encodePath", t, func() {
		Convey("charset='' 时按原样返回（默认 UTF-8 直通）", func() {
			out, err := encodePath("", "/home/用户/测试.txt")
			So(err, ShouldBeNil)
			So(out, ShouldEqual, "/home/用户/测试.txt")
		})

		Convey("charset=utf-8 同样直通", func() {
			out, err := encodePath("utf-8", "/中文/路径")
			So(err, ShouldBeNil)
			So(out, ShouldEqual, "/中文/路径")
		})

		Convey("charset=gbk 把 UTF-8 路径编为 GBK 字节", func() {
			out, err := encodePath("gbk", "/中")
			So(err, ShouldBeNil)
			So([]byte(out), ShouldResemble, append([]byte("/"), 0xD6, 0xD0))
		})

		Convey("纯 ASCII 在 GBK 下不变", func() {
			out, err := encodePath("gbk", "/home/admin")
			So(err, ShouldBeNil)
			So(out, ShouldEqual, "/home/admin")
		})
	})
}

func TestDecodeName(t *testing.T) {
	Convey("decodeName", t, func() {
		Convey("charset='' 时按原样返回", func() {
			So(decodeName("", "anything.txt"), ShouldEqual, "anything.txt")
		})

		Convey("charset=gbk 把 GBK 字节解成 UTF-8", func() {
			gbkName := string([]byte{0xD6, 0xD0, 0xCE, 0xC4, '.', 't', 'x', 't'}) // "中文.txt" in GBK
			So(decodeName("gbk", gbkName), ShouldEqual, "中文.txt")
		})

		Convey("纯 ASCII 在 GBK 下不变", func() {
			So(decodeName("gbk", "README"), ShouldEqual, "README")
		})

		Convey("含非法字节的名字仍能解码（坏字节替换为 U+FFFD，文件不丢）", func() {
			// 单字节 0xFF 在 GBK 中非法；x/text 解码器替换为 U+FFFD，不报错也不 panic。
			// 关键是：含坏字节的文件名不应导致整个目录列表失败。
			out := decodeName("gbk", string([]byte{0xFF}))
			So(out, ShouldNotBeBlank)
		})
	})
}
