package migrations

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration202605120001 将 conversation_messages.mentions 中的元数据回填到 content 内联 XML，
// 然后 DROP 该列。
//
// SQLite 3.35+ 原生支持 ALTER TABLE DROP COLUMN，本项目内置 modernc.org/sqlite v1.23.1（SQLite 3.41+）
// 不再需要"重建表"workaround。
func migration202605120001() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605120001",
		Migrate: func(tx *gorm.DB) error {
			if err := backfillMessageMentionXML(tx); err != nil {
				return err
			}
			return tx.Exec(`ALTER TABLE conversation_messages DROP COLUMN mentions`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

type legacyMentionRef struct {
	AssetID int64  `json:"assetId"`
	Name    string `json:"name"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
}

type legacyAssetMeta struct {
	Type      string
	GroupPath string
}

func backfillMessageMentionXML(tx *gorm.DB) error {
	// 1) 预加载 group tree -> id 到 (name, parent_id) 的映射，用于拼 groupPath。
	type groupRow struct {
		ID       int64
		Name     string
		ParentID int64
	}
	var groups []groupRow
	if err := tx.Raw(`SELECT id, name, parent_id FROM groups`).Scan(&groups).Error; err != nil {
		return err
	}
	gmap := make(map[int64]groupRow, len(groups))
	for _, g := range groups {
		gmap[g.ID] = g
	}
	pathOf := func(gid int64) string {
		if gid == 0 {
			return ""
		}
		var parts []string
		seen := map[int64]bool{}
		for cur := gid; cur != 0; {
			if seen[cur] {
				break
			}
			seen[cur] = true
			g, ok := gmap[cur]
			if !ok {
				break
			}
			parts = append([]string{g.Name}, parts...)
			cur = g.ParentID
		}
		return strings.Join(parts, "/")
	}

	// 2) 预加载 asset 元数据 (id -> type + group_path)。
	type assetRow struct {
		ID      int64
		Type    string
		GroupID int64
	}
	var assets []assetRow
	if err := tx.Raw(`SELECT id, type, group_id FROM assets`).Scan(&assets).Error; err != nil {
		return err
	}
	amap := make(map[int64]legacyAssetMeta, len(assets))
	for _, a := range assets {
		amap[a.ID] = legacyAssetMeta{Type: a.Type, GroupPath: pathOf(a.GroupID)}
	}

	// 3) 扫描所有带 mentions 的消息行。
	type msgRow struct {
		ID       int64
		Content  string
		Mentions string
	}
	var rows []msgRow
	if err := tx.Raw(`
		SELECT id, content, mentions
		FROM conversation_messages
		WHERE mentions IS NOT NULL AND mentions != ''
	`).Scan(&rows).Error; err != nil {
		return err
	}

	for _, r := range rows {
		var refs []legacyMentionRef
		if err := json.Unmarshal([]byte(r.Mentions), &refs); err != nil {
			// 损坏的 JSON 跳过，保留原 content。
			continue
		}
		newContent := spliceMentionsToXML(r.Content, refs, amap)
		if newContent == r.Content {
			continue
		}
		if err := tx.Exec(
			`UPDATE conversation_messages SET content = ? WHERE id = ?`,
			newContent, r.ID,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// spliceMentionsToXML 按 start 降序遍历，把每个 mention 区间替换成 <mention …>@name</mention>；
// 区间外的内容做 XML 转义。
func spliceMentionsToXML(content string, refs []legacyMentionRef, amap map[int64]legacyAssetMeta) string {
	if len(refs) == 0 {
		return content
	}
	// 字符级索引（旧实现按 string slice，多字节字符不安全 — 按当前行为复刻 byte 偏移）。
	// 旧 buildSegments 使用 content.slice(start, end)，JS 字符串 slice 走 UTF-16 code units；
	// 由于历史数据只在 ASCII `@` 处定位，这里复用 byte 偏移保持与历史索引一致即可。
	sorted := make([]legacyMentionRef, len(refs))
	copy(sorted, refs)
	// 降序，保证 splice 后未处理的索引仍然有效。
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start > sorted[j].Start })

	out := content
	for _, ref := range sorted {
		if ref.Start < 0 || ref.End > len(out) || ref.Start >= ref.End {
			continue
		}
		meta := amap[ref.AssetID]
		tag := buildMentionTag(ref.AssetID, ref.Name, meta.Type, meta.GroupPath)
		out = out[:ref.Start] + tag + out[ref.End:]
	}

	// 再对 mention 区间之外的文本做 XML 转义：用占位符隔离 mention 块。
	return escapeOutsideMentions(out)
}

func buildMentionTag(assetID int64, name, assetType, groupPath string) string {
	var b strings.Builder
	b.WriteString(`<mention asset-id="`)
	b.WriteString(strconv.FormatInt(assetID, 10))
	b.WriteString(`"`)
	if assetType != "" {
		b.WriteString(` type="`)
		b.WriteString(escapeXMLAttr(assetType))
		b.WriteString(`"`)
	}
	if groupPath != "" {
		b.WriteString(` group="`)
		b.WriteString(escapeXMLAttr(groupPath))
		b.WriteString(`"`)
	}
	b.WriteString(">@")
	b.WriteString(escapeXMLText(name))
	b.WriteString("</mention>")
	return b.String()
}

// escapeOutsideMentions 把 mention 标签视为占位符，仅对其余文本做 < > & 转义。
func escapeOutsideMentions(s string) string {
	const openTag = "<mention "
	const closeTag = "</mention>"
	var b strings.Builder
	i := 0
	for i < len(s) {
		idx := strings.Index(s[i:], openTag)
		if idx < 0 {
			b.WriteString(escapeXMLText(s[i:]))
			break
		}
		b.WriteString(escapeXMLText(s[i : i+idx]))
		end := strings.Index(s[i+idx:], closeTag)
		if end < 0 {
			// 没有闭合标签，把剩余当文本转义。
			b.WriteString(escapeXMLText(s[i+idx:]))
			break
		}
		tagEnd := i + idx + end + len(closeTag)
		b.WriteString(s[i+idx : tagEnd])
		i = tagEnd
	}
	return b.String()
}

func escapeXMLText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func escapeXMLAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
