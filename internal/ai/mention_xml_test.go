package ai

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWrapMentions(t *testing.T) {
	Convey("WrapMentions", t, func() {
		Convey("空 mentions 返回原文", func() {
			So(WrapMentions("hi", nil), ShouldEqual, "hi")
			So(WrapMentions("hi", []MentionedAsset{}), ShouldEqual, "hi")
		})

		Convey("单 mention 按字面 @name 包标签", func() {
			got := WrapMentions("@x 看", []MentionedAsset{
				{AssetID: 1, Name: "x", Type: "ssh", Start: 0, End: 2},
			})
			So(got, ShouldEqual,
				`<mention asset-id="1" name="x" type="ssh" host="" group="" start="0" end="2">@x</mention> 看`)
		})

		Convey("多 mention 按出现顺序分别消费", func() {
			got := WrapMentions("@a 然后 @b 最后", []MentionedAsset{
				{AssetID: 1, Name: "a", Start: 0, End: 2},
				{AssetID: 2, Name: "b", Start: 5, End: 7},
			})
			So(got, ShouldContainSubstring, `asset-id="1"`)
			So(got, ShouldContainSubstring, `asset-id="2"`)
			So(got, ShouldContainSubstring, ">@a</mention>")
			So(got, ShouldContainSubstring, ">@b</mention>")
			// b 标签必须出现在 a 之后
			aPos := indexOf(got, `asset-id="1"`)
			bPos := indexOf(got, `asset-id="2"`)
			So(aPos, ShouldBeLessThan, bPos)
		})

		Convey("raw 里找不到 @name 字面时跳过", func() {
			got := WrapMentions("plain text", []MentionedAsset{
				{AssetID: 1, Name: "missing"},
			})
			So(got, ShouldEqual, "plain text")
		})

		Convey("attr 值含特殊字符做 escape", func() {
			got := WrapMentions(`@x ok`, []MentionedAsset{
				{AssetID: 1, Name: "x", Host: `a"b<c>d&e`, Start: 0, End: 2},
			})
			So(got, ShouldContainSubstring, `host="a&quot;b&lt;c&gt;d&amp;e"`)
		})
	})
}

func TestParseMentions(t *testing.T) {
	Convey("ParseMentions", t, func() {
		Convey("无 mention 标签返回 nil", func() {
			So(ParseMentions("plain text"), ShouldBeNil)
			So(ParseMentions(""), ShouldBeNil)
		})

		Convey("提取所有属性", func() {
			s := `<mention asset-id="42" name="prod-db" type="mysql" host="10.0.0.5" group="生产/db" start="6" end="14">@prod-db</mention> 看`
			got := ParseMentions(s)
			So(len(got), ShouldEqual, 1)
			So(got[0].AssetID, ShouldEqual, 42)
			So(got[0].Name, ShouldEqual, "prod-db")
			So(got[0].Type, ShouldEqual, "mysql")
			So(got[0].Host, ShouldEqual, "10.0.0.5")
			So(got[0].GroupPath, ShouldEqual, "生产/db")
			So(got[0].Start, ShouldEqual, 6)
			So(got[0].End, ShouldEqual, 14)
		})

		Convey("多 mention 按出现顺序", func() {
			s := `<mention asset-id="1" name="a" start="0" end="2">@a</mention> 和 <mention asset-id="2" name="b" start="5" end="7">@b</mention>`
			got := ParseMentions(s)
			So(len(got), ShouldEqual, 2)
			So(got[0].AssetID, ShouldEqual, 1)
			So(got[1].AssetID, ShouldEqual, 2)
		})

		Convey("escape attr 反向 unescape", func() {
			s := `<mention asset-id="1" name="x" host="a&quot;b&lt;c&gt;d&amp;e">@x</mention>`
			got := ParseMentions(s)
			So(len(got), ShouldEqual, 1)
			So(got[0].Host, ShouldEqual, `a"b<c>d&e`)
		})

		Convey("非法 asset-id / start / end 落零值", func() {
			s := `<mention asset-id="abc" name="x" start="xx" end="">@x</mention>`
			got := ParseMentions(s)
			So(len(got), ShouldEqual, 1)
			So(got[0].AssetID, ShouldEqual, 0)
			So(got[0].Start, ShouldEqual, 0)
			So(got[0].End, ShouldEqual, 0)
			So(got[0].Name, ShouldEqual, "x")
		})
	})
}

func TestWrapParseRoundTrip(t *testing.T) {
	Convey("Wrap → Parse round-trip 保字段", t, func() {
		original := []MentionedAsset{
			{AssetID: 42, Name: "prod-db", Type: "mysql", Host: "10.0.0.5", GroupPath: "生产/数据库", Start: 0, End: 8},
			{AssetID: 43, Name: "cache", Type: "redis", Host: "10.0.0.6", Start: 12, End: 18},
		}
		wrapped := WrapMentions("@prod-db 看 @cache 测", original)
		parsed := ParseMentions(wrapped)
		So(len(parsed), ShouldEqual, 2)
		for i := range original {
			So(parsed[i].AssetID, ShouldEqual, original[i].AssetID)
			So(parsed[i].Name, ShouldEqual, original[i].Name)
			So(parsed[i].Type, ShouldEqual, original[i].Type)
			So(parsed[i].Host, ShouldEqual, original[i].Host)
			So(parsed[i].GroupPath, ShouldEqual, original[i].GroupPath)
			So(parsed[i].Start, ShouldEqual, original[i].Start)
			So(parsed[i].End, ShouldEqual, original[i].End)
		}
	})
}

func TestByteToUTF16(t *testing.T) {
	Convey("ByteToUTF16", t, func() {
		Convey("ASCII 等长", func() {
			So(ByteToUTF16("hello", 0), ShouldEqual, 0)
			So(ByteToUTF16("hello", 3), ShouldEqual, 3)
			So(ByteToUTF16("hello", 5), ShouldEqual, 5)
		})

		Convey("CJK 每字符 3 byte → 1 utf16 unit", func() {
			s := "@本地" // @=1B, 本=3B, 地=3B → 7 bytes; UTF-16 = 3 units
			So(ByteToUTF16(s, 0), ShouldEqual, 0)
			So(ByteToUTF16(s, 1), ShouldEqual, 1) // "@"
			So(ByteToUTF16(s, 4), ShouldEqual, 2) // "@本"
			So(ByteToUTF16(s, 7), ShouldEqual, 3) // "@本地"
		})

		Convey("越界 byteOff clamp 到字符串长度", func() {
			s := "abc"
			So(ByteToUTF16(s, -1), ShouldEqual, 0)
			So(ByteToUTF16(s, 100), ShouldEqual, 3)
		})

		Convey("surrogate pair（U+1F600 emoji 占 2 个 utf16 unit）", func() {
			s := "ab\U0001F600cd"
			// a=1B, b=1B, emoji=4B, c=1B, d=1B
			So(ByteToUTF16(s, 2), ShouldEqual, 2)       // "ab"
			So(ByteToUTF16(s, 2+4), ShouldEqual, 2+2)   // "ab😀" → 4 units (surrogate pair)
			So(ByteToUTF16(s, 2+4+1), ShouldEqual, 2+3) // "ab😀c"
		})
	})
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
