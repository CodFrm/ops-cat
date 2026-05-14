// XML 内联 mention 的工具集。
//
// 输入文本中只允许出现两类内容：
//   - 普通文本（< > & 必须按 XML 规则转义为 &lt; &gt; &amp;）
//   - <mention asset-id="..." [type=...] [host=...] [group=...]>@name</mention>
//
// 标签 attr 用双引号，名字里的 & < > " 同样转义。

export interface MentionAttrs {
  assetId: number;
  name: string; // 不含 @ 前缀
  type?: string;
  host?: string;
  groupPath?: string;
}

export type MentionSegment = { type: "text"; text: string } | { type: "mention"; text: string; attrs: MentionAttrs };

const TAG_RE = /<mention\b([^>]*)>([\s\S]*?)<\/mention>/g;
const ATTR_RE = /([a-zA-Z][\w-]*)\s*=\s*"([^"]*)"/g;

export function escapeXmlText(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

export function escapeXmlAttr(s: string): string {
  return escapeXmlText(s).replace(/"/g, "&quot;");
}

function unescapeXml(s: string): string {
  return s
    .replace(/&quot;/g, '"')
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&amp;/g, "&");
}

export function buildMentionXml(attrs: MentionAttrs): string {
  const parts = [`asset-id="${attrs.assetId}"`];
  if (attrs.type) parts.push(`type="${escapeXmlAttr(attrs.type)}"`);
  if (attrs.host) parts.push(`host="${escapeXmlAttr(attrs.host)}"`);
  if (attrs.groupPath) parts.push(`group="${escapeXmlAttr(attrs.groupPath)}"`);
  return `<mention ${parts.join(" ")}>@${escapeXmlText(attrs.name)}</mention>`;
}

function parseAttrs(raw: string): Partial<MentionAttrs> {
  const out: Partial<MentionAttrs> = {};
  let m: RegExpExecArray | null;
  const re = new RegExp(ATTR_RE.source, "g");
  while ((m = re.exec(raw)) !== null) {
    const key = m[1];
    const value = unescapeXml(m[2]);
    switch (key) {
      case "asset-id": {
        const n = Number.parseInt(value, 10);
        if (!Number.isNaN(n)) out.assetId = n;
        break;
      }
      case "type":
        out.type = value;
        break;
      case "host":
        out.host = value;
        break;
      case "group":
        out.groupPath = value;
        break;
    }
  }
  return out;
}

// 把含 <mention> 标签的 content 解析为按出现顺序的 segments。
// 标签之外的文本会做 XML 反转义（& < >）；mention 内文本（"@name"）剥掉 @ 后写入 name。
export function parseMentionContent(content: string): MentionSegment[] {
  if (!content) return [];
  const segs: MentionSegment[] = [];
  let last = 0;
  const re = new RegExp(TAG_RE.source, "g");
  let m: RegExpExecArray | null;
  while ((m = re.exec(content)) !== null) {
    if (m.index > last) {
      segs.push({ type: "text", text: unescapeXml(content.slice(last, m.index)) });
    }
    const attrs = parseAttrs(m[1]);
    const inner = unescapeXml(m[2]);
    const name = inner.startsWith("@") ? inner.slice(1) : inner;
    if (typeof attrs.assetId === "number") {
      segs.push({
        type: "mention",
        text: inner,
        attrs: {
          assetId: attrs.assetId,
          name,
          type: attrs.type,
          host: attrs.host,
          groupPath: attrs.groupPath,
        },
      });
    } else {
      // attrs 缺 asset-id：当普通文本处理，保留原貌
      segs.push({ type: "text", text: inner });
    }
    last = re.lastIndex;
  }
  if (last < content.length) {
    segs.push({ type: "text", text: unescapeXml(content.slice(last)) });
  }
  return segs;
}

// 抽取 content 中所有 mention（按出现顺序，不去重）。
export function extractMentions(content: string): MentionAttrs[] {
  return parseMentionContent(content)
    .filter((s): s is { type: "mention"; text: string; attrs: MentionAttrs } => s.type === "mention")
    .map((s) => s.attrs);
}

// 把 content 中的 <mention> 标签降级为内部纯文本（含 @name），用于不可信存储（如 localStorage）回填时
// 去掉 attrs 的安全语义。
export function stripMentionTags(content: string): string {
  if (!content) return content;
  return content.replace(TAG_RE, (_, _attrs, inner) => unescapeXml(String(inner)));
}
