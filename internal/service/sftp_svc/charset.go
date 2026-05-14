package sftp_svc

import (
	"github.com/opskat/opskat/internal/pkg/charset"

	xencoding "golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

// encodePath 把前端传来的 UTF-8 路径编码到远端目标字符集（如 GBK），
// 用于送进 sftp.Client 的所有 path 入参。
// charset 为空 / utf-8 / 无法识别时返回原值；不可表示的字符被替换为编码自身的占位字符。
func encodePath(name, p string) (string, error) {
	enc, ok := charset.Lookup(name)
	if !ok || enc == nil {
		return p, nil
	}
	out, _, err := transform.Bytes(xencoding.ReplaceUnsupported(enc.NewEncoder()), []byte(p))
	if err != nil {
		return p, err
	}
	return string(out), nil
}

// decodeName 把远端 ReadDir/Getwd/RealPath 返回的字节流解码为前端使用的 UTF-8。
// 解码失败（含非法字节序列）退化为原值，避免单个坏文件名让整个列表渲染失败。
func decodeName(name, raw string) string {
	enc, ok := charset.Lookup(name)
	if !ok || enc == nil {
		return raw
	}
	out, _, err := transform.Bytes(enc.NewDecoder(), []byte(raw))
	if err != nil {
		return raw
	}
	return string(out)
}
