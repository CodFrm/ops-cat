package ai

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

// mention 元数据通过 inline XML 与消息正文原子绑定，取代旧的 Manager.StashMentions 旁路。
// LLM body 形如：`<mention asset-id="37" name="local-docker" type="ssh" host="..." group="..." start="0" end="13">@local-docker</mention> 看看容器`
// gormStore 在 AppendMessage 时反向 ParseMentions(text) 得到 row.Mentions JSON。
// start/end 属性是该 mention 在 raw 显示文本（不含 XML 标签）中的 JS UTF-16 偏移，
// 与前端 ChatMessage.mentions 一致——这样前端 chip 渲染（UserMessage.buildSegments）
// 直接复用即可。

var (
	mentionElemRe = regexp.MustCompile(`<mention\s+([^>]*?)>([\s\S]*?)</mention>`)
	mentionAttrRe = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9_\-]*)="([^"]*)"`)
)

// WrapMentions 把 raw 中每个 mention 字面 "@name" 出现位置包成 <mention .../> 标签。
// mentions 应该已经按 Start 升序排序；如果 raw 里找不到 "@name" 字面则跳过该 mention。
// Start/End attr 直接写入 mention.Start/End（caller 责任保证是 JS UTF-16 偏移）。
//
// 单个 mention 的字面 "@name" 在 raw 里可能多次出现，本函数按顺序消费——即第一个 mention
// 匹配 raw 里第一次出现的 "@name"，第二个 mention 匹配第二次出现的 "@name"，以此类推。
// 调用方（前端 buildLLMBody / hook opsMentionResolver）都按 mention 出现顺序传入，
// 这一约定与现有 mentions 数组语义一致。
func WrapMentions(raw string, mentions []MentionedAsset) string {
	if len(mentions) == 0 {
		return raw
	}
	var b strings.Builder
	cursor := 0
	for _, m := range mentions {
		marker := "@" + m.Name
		idx := strings.Index(raw[cursor:], marker)
		if idx < 0 {
			continue
		}
		idx += cursor
		b.WriteString(raw[cursor:idx])
		b.WriteString(formatMentionTag(m, marker))
		cursor = idx + len(marker)
	}
	b.WriteString(raw[cursor:])
	return b.String()
}

func formatMentionTag(m MentionedAsset, inner string) string {
	return fmt.Sprintf(
		`<mention asset-id="%d" name="%s" type="%s" host="%s" group="%s" start="%d" end="%d">%s</mention>`,
		m.AssetID,
		xmlAttrEscape(m.Name),
		xmlAttrEscape(m.Type),
		xmlAttrEscape(m.Host),
		xmlAttrEscape(m.GroupPath),
		m.Start, m.End,
		xmlTextEscape(inner),
	)
}

// ParseMentions 从含 inline XML 的文本里解析所有 <mention> 标签，按出现顺序返回。
// 解析失败的字段保持零值（asset-id 非法 → 0；start/end 非法 → 0）。
// 不存在标签时返回 nil。
func ParseMentions(s string) []MentionedAsset {
	matches := mentionElemRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]MentionedAsset, 0, len(matches))
	for _, match := range matches {
		attrPairs := mentionAttrRe.FindAllStringSubmatch(match[1], -1)
		var ma MentionedAsset
		for _, p := range attrPairs {
			switch p[1] {
			case "asset-id":
				if id, err := strconv.ParseInt(p[2], 10, 64); err == nil {
					ma.AssetID = id
				}
			case "name":
				ma.Name = xmlAttrUnescape(p[2])
			case "type":
				ma.Type = xmlAttrUnescape(p[2])
			case "host":
				ma.Host = xmlAttrUnescape(p[2])
			case "group":
				ma.GroupPath = xmlAttrUnescape(p[2])
			case "start":
				if v, err := strconv.Atoi(p[2]); err == nil {
					ma.Start = v
				}
			case "end":
				if v, err := strconv.Atoi(p[2]); err == nil {
					ma.End = v
				}
			}
		}
		out = append(out, ma)
	}
	return out
}

// ByteToUTF16 把 Go 字符串中的 byte offset 转换为 JS UTF-16 code unit 偏移。
// 后端 opsMentionResolver 用 regex 找 @name 的 byte 位置，通过本函数转换后填到
// mention.Start/End——保证 wire 上的 start/end 始终是 UTF-16 单位（与前端一致）。
func ByteToUTF16(s string, byteOff int) int {
	if byteOff <= 0 {
		return 0
	}
	if byteOff >= len(s) {
		return len(utf16.Encode([]rune(s)))
	}
	return len(utf16.Encode([]rune(s[:byteOff])))
}

func xmlAttrEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func xmlAttrUnescape(s string) string {
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return s
}

func xmlTextEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
