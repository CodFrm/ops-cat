package ssh_svc

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseProbedCharset(t *testing.T) {
	Convey("parseProbedCharset", t, func() {
		Convey("UTF-8 → 空字符串（直通）", func() {
			So(parseProbedCharset("UTF-8\n"), ShouldEqual, "")
			So(parseProbedCharset("utf-8"), ShouldEqual, "")
			So(parseProbedCharset(" UTF-8 "), ShouldEqual, "")
		})

		Convey("GBK / GB18030 / Big5 → 规范化小写名", func() {
			So(parseProbedCharset("GBK\n"), ShouldEqual, "gbk")
			So(parseProbedCharset("GB18030"), ShouldEqual, "gb18030")
			So(parseProbedCharset("BIG5\n"), ShouldEqual, "big5")
		})

		Convey("空 / 空白 → 空", func() {
			So(parseProbedCharset(""), ShouldEqual, "")
			So(parseProbedCharset("   \n\n"), ShouldEqual, "")
		})

		Convey("无法识别 → 空（不要把奇怪字符串塞给 charset.Lookup）", func() {
			So(parseProbedCharset("klingon"), ShouldEqual, "")
		})

		Convey("多行输出只取首行", func() {
			So(parseProbedCharset("GBK\nsome garbage"), ShouldEqual, "gbk")
		})
	})
}
