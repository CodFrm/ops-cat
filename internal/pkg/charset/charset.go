// Package charset 提供字符集名解析，统一 SSH/SFTP/Query 导出等模块对 GBK/GB18030/Big5/Shift-JIS 等非 UTF-8 编码的处理。
package charset

import (
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
)

// Lookup 按名字（大小写、连字符/下划线、CP/数字编号均容忍）解析字符集。
//
// 返回值约定：
//   - 空字符串、"utf-8"、"utf8"、"65001"：返回 (nil, true)，调用方应跳过转换直接走 UTF-8。
//   - 已识别的非 UTF-8 名字：返回 (encoding, true)。
//   - 未识别的名字：返回 (nil, false)。
func Lookup(name string) (encoding.Encoding, bool) {
	n := normalize(name)
	if n == "" || n == "utf-8" || n == "utf8" || n == "65001" {
		return nil, true
	}
	switch n {
	case "utf-16le", "utf16le", "1200":
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), true
	case "utf-16be", "utf16be", "1201":
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), true
	case "gb18030", "54936":
		return simplifiedchinese.GB18030, true
	case "gbk", "cp936", "936":
		return simplifiedchinese.GBK, true
	case "big5", "950":
		return traditionalchinese.Big5, true
	case "shift-jis", "shiftjis", "sjis", "cp932", "932":
		return japanese.ShiftJIS, true
	case "euc-jp", "eucjp", "20932":
		return japanese.EUCJP, true
	case "iso-2022-jp", "iso2022jp", "50220", "50221", "50222":
		return japanese.ISO2022JP, true
	case "euc-kr", "euckr", "cp949", "949":
		return korean.EUCKR, true
	case "iso-8859-1", "latin1", "latin-1", "28591":
		return charmap.ISO8859_1, true
	case "windows-1252", "cp1252", "1252":
		return charmap.Windows1252, true
	}
	return nil, false
}

// IsUTF8 判断名字是否表示 UTF-8 或为空（无需转换）。未知名字也返回 false。
func IsUTF8(name string) bool {
	n := normalize(name)
	return n == "" || n == "utf-8" || n == "utf8" || n == "65001"
}

func normalize(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "_", "-"))
}
