import { describe, expect, it } from "vitest";
import {
  buildMentionXml,
  escapeXmlAttr,
  escapeXmlText,
  extractMentions,
  parseMentionContent,
  stripMentionTags,
} from "../mentionXml";

describe("mentionXml", () => {
  it("buildMentionXml 渲染完整属性", () => {
    const xml = buildMentionXml({
      assetId: 42,
      name: "prod-db",
      type: "mysql",
      host: "10.0.0.5",
      groupPath: "生产/数据库",
    });
    expect(xml).toBe('<mention asset-id="42" type="mysql" host="10.0.0.5" group="生产/数据库">@prod-db</mention>');
  });

  it("buildMentionXml 只输出有值的可选属性", () => {
    const xml = buildMentionXml({ assetId: 1, name: "x" });
    expect(xml).toBe('<mention asset-id="1">@x</mention>');
  });

  it('escapeXmlText 转义 & < >，不动 "', () => {
    expect(escapeXmlText('a < b & c > d"')).toBe('a &lt; b &amp; c &gt; d"');
  });

  it('escapeXmlAttr 额外转义 "', () => {
    expect(escapeXmlAttr('a"b')).toBe("a&quot;b");
  });

  it("parseMentionContent 拆分文本与 mention", () => {
    const content = 'ssh into <mention asset-id="1" type="ssh" host="1.1.1.1" group="生产">@prod-db</mention> then ls';
    const segs = parseMentionContent(content);
    expect(segs).toHaveLength(3);
    expect(segs[0]).toEqual({ type: "text", text: "ssh into " });
    expect(segs[1]).toEqual({
      type: "mention",
      text: "@prod-db",
      attrs: {
        assetId: 1,
        name: "prod-db",
        type: "ssh",
        host: "1.1.1.1",
        groupPath: "生产",
      },
    });
    expect(segs[2]).toEqual({ type: "text", text: " then ls" });
  });

  it("parseMentionContent 对缺 asset-id 的标签当作文本", () => {
    const segs = parseMentionContent('hi <mention type="ssh">@x</mention>');
    expect(segs).toEqual([
      { type: "text", text: "hi " },
      { type: "text", text: "@x" },
    ]);
  });

  it("parseMentionContent 反转义文本里的 &lt; &amp;", () => {
    const segs = parseMentionContent("a &lt; b &amp; c");
    expect(segs).toEqual([{ type: "text", text: "a < b & c" }]);
  });

  it("round-trip：build + 内嵌入 content 再 parse 等价", () => {
    const tag = buildMentionXml({ assetId: 7, name: 'a"<&>b', type: "ssh", host: "h" });
    const content = `before ${tag} after`;
    const segs = parseMentionContent(content);
    expect(segs[0]).toEqual({ type: "text", text: "before " });
    expect(segs[1].type).toBe("mention");
    if (segs[1].type === "mention") {
      expect(segs[1].attrs.name).toBe('a"<&>b');
    }
    expect(segs[2]).toEqual({ type: "text", text: " after" });
  });

  it("extractMentions 按顺序输出 attrs", () => {
    const c = '<mention asset-id="1">@a</mention> <mention asset-id="2" type="redis">@b</mention>';
    const mentions = extractMentions(c);
    expect(mentions).toEqual([
      { assetId: 1, name: "a", type: undefined, host: undefined, groupPath: undefined },
      { assetId: 2, name: "b", type: "redis", host: undefined, groupPath: undefined },
    ]);
  });

  it("stripMentionTags 降级为纯内部文本", () => {
    const c = 'hi <mention asset-id="1" host="evil">@x</mention> there';
    expect(stripMentionTags(c)).toBe("hi @x there");
  });
});
