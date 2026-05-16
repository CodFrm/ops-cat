package charset

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

func TestLookup(t *testing.T) {
	Convey("Lookup", t, func() {
		Convey("空与 utf-8 视为无需转换", func() {
			for _, name := range []string{"", " ", "utf-8", "UTF-8", "utf8", "UTF8", "65001"} {
				enc, ok := Lookup(name)
				So(ok, ShouldBeTrue)
				So(enc, ShouldBeNil)
			}
		})

		Convey("GBK 变体", func() {
			for _, name := range []string{"gbk", "GBK", "cp936", "CP936", "936"} {
				enc, ok := Lookup(name)
				So(ok, ShouldBeTrue)
				So(enc, ShouldEqual, simplifiedchinese.GBK)
			}
		})

		Convey("GB18030", func() {
			enc, ok := Lookup("gb18030")
			So(ok, ShouldBeTrue)
			So(enc, ShouldEqual, simplifiedchinese.GB18030)
		})

		Convey("Big5", func() {
			enc, ok := Lookup("Big5")
			So(ok, ShouldBeTrue)
			So(enc, ShouldEqual, traditionalchinese.Big5)
		})

		Convey("Shift-JIS 变体（含下划线规范化）", func() {
			for _, name := range []string{"shift-jis", "shiftjis", "sjis", "shift_jis", "cp932", "932"} {
				enc, ok := Lookup(name)
				So(ok, ShouldBeTrue)
				So(enc, ShouldEqual, japanese.ShiftJIS)
			}
		})

		Convey("EUC-KR", func() {
			for _, name := range []string{"euc-kr", "EUCKR", "cp949", "949"} {
				enc, ok := Lookup(name)
				So(ok, ShouldBeTrue)
				So(enc, ShouldEqual, korean.EUCKR)
			}
		})

		Convey("EUC-JP", func() {
			enc, ok := Lookup("eucjp")
			So(ok, ShouldBeTrue)
			So(enc, ShouldEqual, japanese.EUCJP)
		})

		Convey("ISO-2022-JP", func() {
			enc, ok := Lookup("iso-2022-jp")
			So(ok, ShouldBeTrue)
			So(enc, ShouldEqual, japanese.ISO2022JP)
		})

		Convey("Latin-1", func() {
			enc, ok := Lookup("latin1")
			So(ok, ShouldBeTrue)
			So(enc, ShouldEqual, charmap.ISO8859_1)
		})

		Convey("Windows-1252", func() {
			enc, ok := Lookup("windows-1252")
			So(ok, ShouldBeTrue)
			So(enc, ShouldEqual, charmap.Windows1252)
		})

		Convey("未知字符集", func() {
			enc, ok := Lookup("klingon")
			So(ok, ShouldBeFalse)
			So(enc, ShouldBeNil)
		})
	})
}

func TestIsUTF8(t *testing.T) {
	Convey("IsUTF8", t, func() {
		So(IsUTF8(""), ShouldBeTrue)
		So(IsUTF8(" "), ShouldBeTrue)
		So(IsUTF8("utf-8"), ShouldBeTrue)
		So(IsUTF8("UTF-8"), ShouldBeTrue)
		So(IsUTF8("utf8"), ShouldBeTrue)
		So(IsUTF8("65001"), ShouldBeTrue)
		So(IsUTF8("gbk"), ShouldBeFalse)
		So(IsUTF8("klingon"), ShouldBeFalse)
	})
}
