package external_edit_svc

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func (s *Service) hydrateSessionEncodingLocked(session *Session) error {
	if session == nil || strings.TrimSpace(session.OriginalEncoding) != "" {
		return nil
	}
	data, err := readLocalEditableFile(session.LocalPath)
	if err != nil {
		return fmt.Errorf("读取本地副本失败: %w", err)
	}
	snapshot, err := detectTextEncoding(data)
	if err != nil {
		return err
	}
	applyEncodingSnapshot(session, snapshot)
	if session.OriginalSize == 0 {
		session.OriginalSize = int64(len(data))
	}
	if sessionBaseHash(session) == "" {
		setSessionBaseHash(session, hashBytes(data))
	}
	if session.LastLocalSHA256 == "" {
		setSessionLocalHash(session, hashBytes(data))
	}
	return nil
}

func applyEncodingSnapshot(session *Session, snapshot *textEncodingSnapshot) {
	if session == nil || snapshot == nil {
		return
	}
	session.OriginalEncoding = snapshot.Encoding
	session.OriginalBOM = snapshot.BOM
	session.OriginalByteSample = snapshot.ByteSample
}

func detectTextEncoding(data []byte) (*textEncodingSnapshot, error) {
	// 这里只接受当前链路可稳定 round-trip 的编码集合。
	// 外部编辑的核心目标是“改文本内容而不破坏文件容器”，因此宁可保守拒绝，也不能把未知编码默默转坏。
	bomName, _, body := splitTextBOM(data)
	switch bomName {
	case textEncodingUTF16LE:
		if _, err := roundTripBody(textEncodingUTF16LE, body); err != nil {
			return nil, err
		}
		return &textEncodingSnapshot{
			Encoding:   textEncodingUTF16LE,
			BOM:        bomName,
			ByteSample: byteSampleHex(data),
		}, nil
	case textEncodingUTF16BE:
		if _, err := roundTripBody(textEncodingUTF16BE, body); err != nil {
			return nil, err
		}
		return &textEncodingSnapshot{
			Encoding:   textEncodingUTF16BE,
			BOM:        bomName,
			ByteSample: byteSampleHex(data),
		}, nil
	case textEncodingUTF8:
		if !utf8.Valid(body) {
			return nil, fmt.Errorf("UTF-8 内容无效")
		}
		return &textEncodingSnapshot{
			Encoding:   textEncodingUTF8,
			BOM:        bomName,
			ByteSample: byteSampleHex(data),
		}, nil
	}
	if utf8.Valid(body) {
		return &textEncodingSnapshot{
			Encoding:   textEncodingUTF8,
			ByteSample: byteSampleHex(data),
		}, nil
	}
	if roundTripped, err := roundTripBody(textEncodingGB18030, body); err == nil && bytes.Equal(roundTripped, body) {
		return &textEncodingSnapshot{
			Encoding:   textEncodingGB18030,
			ByteSample: byteSampleHex(data),
		}, nil
	}
	return nil, fmt.Errorf("暂不支持识别当前文本编码")
}

func validateRoundTrip(session *Session, data []byte) error {
	if session == nil || strings.TrimSpace(session.OriginalEncoding) == "" {
		return fmt.Errorf("当前会话缺少原始编码信息，请重新打开远程文件后再同步")
	}

	// 先校验 BOM，再校验编码回环。
	// 这样能把“编辑器切换编码容器”和“文本内容不可逆”拆成两类可解释错误，方便用户按原编辑器设置回退。
	currentBOM, _, body := splitTextBOM(data)
	if currentBOM != session.OriginalBOM {
		return fmt.Errorf(
			"检测到文件 BOM 已变化（原始 %s，当前 %s），请恢复原始 BOM 后再同步",
			describeBOM(session.OriginalBOM),
			describeBOM(currentBOM),
		)
	}

	roundTripped, err := roundTripBody(session.OriginalEncoding, body)
	if err != nil || !bytes.Equal(roundTripped, body) {
		return fmt.Errorf("检测到文件编码已偏离原始 %s，请使用原始编码重新保存后再同步", describeEncoding(session.OriginalEncoding))
	}
	return nil
}

func splitTextBOM(data []byte) (string, []byte, []byte) {
	switch {
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		return textEncodingUTF8, []byte{0xef, 0xbb, 0xbf}, data[3:]
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		return textEncodingUTF16LE, []byte{0xff, 0xfe}, data[2:]
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		return textEncodingUTF16BE, []byte{0xfe, 0xff}, data[2:]
	default:
		return "", nil, data
	}
}

func roundTripBody(encodingName string, body []byte) ([]byte, error) {
	switch encodingName {
	case textEncodingUTF8:
		if !utf8.Valid(body) {
			return nil, fmt.Errorf("UTF-8 内容无效")
		}
		return append([]byte(nil), body...), nil
	case textEncodingUTF16LE:
		return transformRoundTrip(unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), body)
	case textEncodingUTF16BE:
		return transformRoundTrip(unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), body)
	case textEncodingGB18030:
		return transformRoundTrip(simplifiedchinese.GB18030, body)
	default:
		return nil, fmt.Errorf("未知原始编码: %s", encodingName)
	}
}

func transformRoundTrip(textEncoding encoding.Encoding, body []byte) ([]byte, error) {
	decoderOutput, _, err := transform.Bytes(textEncoding.NewDecoder(), body)
	if err != nil {
		return nil, err
	}
	encoderOutput, _, err := transform.Bytes(textEncoding.NewEncoder(), decoderOutput)
	if err != nil {
		return nil, err
	}
	return encoderOutput, nil
}

func byteSampleHex(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sample := data
	if len(sample) > 24 {
		sample = sample[:24]
	}
	return hex.EncodeToString(sample)
}

func describeBOM(bom string) string {
	switch bom {
	case textEncodingUTF8:
		return "UTF-8 BOM"
	case textEncodingUTF16LE:
		return "UTF-16LE BOM"
	case textEncodingUTF16BE:
		return "UTF-16BE BOM"
	default:
		return "无 BOM"
	}
}

func describeEncoding(name string) string {
	switch name {
	case textEncodingUTF8:
		return "UTF-8"
	case textEncodingUTF16LE:
		return "UTF-16LE"
	case textEncodingUTF16BE:
		return "UTF-16BE"
	case textEncodingGB18030:
		return "GB18030"
	default:
		return name
	}
}

func isLikelyText(_ string, data []byte) bool {
	if len(data) == 0 {
		return true
	}
	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	if bytes.HasPrefix(sample, []byte{0xff, 0xfe}) || bytes.HasPrefix(sample, []byte{0xfe, 0xff}) {
		return true
	}
	if bytes.IndexByte(sample, 0) >= 0 {
		return false
	}

	contentType := http.DetectContentType(sample)
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	if contentType == "application/json" || contentType == "application/xml" || contentType == "image/svg+xml" || contentType == "application/x-empty" {
		return true
	}
	return looksLikeText(sample)
}

func looksLikeText(sample []byte) bool {
	if utf8.Valid(sample) {
		return true
	}
	control := 0
	for _, b := range sample {
		if b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		if b < 0x20 {
			control++
		}
	}
	return control*20 < len(sample)
}
