package ssh_svc

import (
	"io"

	"github.com/opskat/opskat/internal/pkg/charset"

	xencoding "golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

// decodeStream 把按 name 编码的字节流转成 UTF-8 字节流。
// name 为空 / utf-8 / 未识别 → 原 reader 直通，调用方无感知。
// transform.NewReader 内置缓冲会跨多次 Read 保留未完成的多字节字符，避免 GBK 双字节
// 在 chunk 边界被劈开。
func decodeStream(name string, r io.Reader) io.Reader {
	enc, ok := charset.Lookup(name)
	if !ok || enc == nil {
		return r
	}
	return transform.NewReader(r, enc.NewDecoder())
}

// encodeForRemote 把 UTF-8 字节序列编码到目标字符集。
// name 为空 / utf-8 / 未识别 → 原样返回。
// 不可表示的 Unicode 字符（如 emoji 在 GBK 下）被替换为字符集自身的占位符（GBK 下为 '?'），
// 避免单个不可输入字符让整次写失败。
func encodeForRemote(name string, p []byte) ([]byte, error) {
	enc, ok := charset.Lookup(name)
	if !ok || enc == nil {
		return p, nil
	}
	out, _, err := transform.Bytes(xencoding.ReplaceUnsupported(enc.NewEncoder()), p)
	return out, err
}
