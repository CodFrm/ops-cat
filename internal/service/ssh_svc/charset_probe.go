package ssh_svc

import (
	"bytes"
	"strings"
	"time"

	"github.com/opskat/opskat/internal/pkg/charset"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// probeCharset 在新建好 SSH client 后探测远端 locale 字符集。
// 返回值：
//   - 空字符串：未探测到 / 是 UTF-8 / 探测失败 — 走 UTF-8 直通（与改动前一致）。
//   - 非空字符串：探测到的非 UTF-8 字符集名（如 "gbk"），交由 Session 做双向编解码。
//
// 实现：开一个独立 exec 通道执行 `locale charmap || echo "${LANG##*.}"`，把 stdout
// 当 ASCII 解析。整体限时 3 秒，任何错误都返回 ""。不应让探测失败把整条连接阻断。
func probeCharset(client *ssh.Client) string {
	type result struct{ value string }
	ch := make(chan result, 1)

	go func() {
		ch <- result{value: runCharsetProbe(client)}
	}()

	select {
	case r := <-ch:
		return r.value
	case <-time.After(3 * time.Second):
		logger.Default().Warn("charset probe timed out", zap.Duration("after", 3*time.Second))
		return ""
	}
}

func runCharsetProbe(client *ssh.Client) string {
	session, err := client.NewSession()
	if err != nil {
		logger.Default().Debug("charset probe: NewSession failed", zap.Error(err))
		return ""
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil && !isExpectedSessionCloseErr(closeErr) {
			logger.Default().Debug("charset probe: close session", zap.Error(closeErr))
		}
	}()

	var out bytes.Buffer
	session.Stdout = &out
	// stderr 丢弃；命令本身把 stderr 重定向到 /dev/null。
	// `locale charmap` 在 GNU/BSD 系统通用；fallback 拿 $LANG 后缀（zh_CN.GBK -> GBK）。
	cmd := `(locale charmap 2>/dev/null) || printf '%s' "${LANG##*.}"`
	if err := session.Run(cmd); err != nil {
		logger.Default().Debug("charset probe: run failed", zap.Error(err))
		return ""
	}
	return parseProbedCharset(out.String())
}

// parseProbedCharset 将 `locale charmap` / `$LANG` 后缀输出规范化为 charset.Lookup 能识别的名字。
// 返回空表示 UTF-8 或无法识别 — 均走直通。
func parseProbedCharset(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	// 取首行（防御性，正常应只有一行）。
	if i := strings.IndexAny(trimmed, "\r\n"); i >= 0 {
		trimmed = trimmed[:i]
	}
	trimmed = strings.TrimSpace(trimmed)
	if charset.IsUTF8(trimmed) {
		return ""
	}
	if _, ok := charset.Lookup(trimmed); !ok {
		return ""
	}
	return strings.ToLower(trimmed)
}

// isExpectedSessionCloseErr 把单次 exec 完毕后常见的关闭返回值视为正常。
func isExpectedSessionCloseErr(err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "EOF") || strings.Contains(msg, "session already closed")
}
