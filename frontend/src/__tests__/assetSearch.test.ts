import { describe, it, expect } from "vitest";
import { buildGroupPathMap, filterAssets } from "@/lib/assetSearch";
import type { asset_entity, group_entity } from "../../wailsjs/go/models";

function asset(id: number, name: string, groupId = 0): asset_entity.Asset {
  return { ID: id, Name: name, GroupID: groupId } as asset_entity.Asset;
}
function group(id: number, name: string, parentId = 0): group_entity.Group {
  return { ID: id, Name: name, ParentID: parentId } as group_entity.Group;
}

describe("buildGroupPathMap", () => {
  it("解析多级嵌套路径", () => {
    const groups = [group(1, "生产"), group(2, "数据库", 1), group(3, "缓存", 2)];
    const map = buildGroupPathMap(groups);
    expect(map.get(1)).toBe("生产");
    expect(map.get(2)).toBe("生产/数据库");
    expect(map.get(3)).toBe("生产/数据库/缓存");
  });

  it("孤儿组（父不存在）按自身名返回", () => {
    const groups = [group(2, "孤儿", 999)];
    const map = buildGroupPathMap(groups);
    expect(map.get(2)).toBe("孤儿");
  });

  it("parent 链成环时按当前组视角截断路径", () => {
    const groups = [group(1, "A", 2), group(2, "B", 1)];
    const map = buildGroupPathMap(groups);
    expect(map.get(1)).toBe("B/A");
    expect(map.get(2)).toBe("A/B");
  });
});

describe("filterAssets", () => {
  const groups = [group(1, "生产"), group(2, "测试")];
  const assets = [
    asset(1, "prod-db", 1),
    asset(2, "prod-web", 1),
    asset(3, "cache-1", 0),
    asset(4, "中转站", 2),
    asset(5, "Web服务器", 2),
  ];

  it("空 query 透传所有资产并保持原序", () => {
    const result = filterAssets(assets, groups, { query: "" });
    expect(result.map((r) => r.asset.ID)).toEqual([1, 2, 3, 4, 5]);
    expect(result.every((r) => r.rank === 0)).toBe(true);
  });

  it("空白 query 视为空", () => {
    const result = filterAssets(assets, groups, { query: "   " });
    expect(result).toHaveLength(5);
  });

  it("name startsWith 排在 includes 之前", () => {
    const items = [asset(10, "abc-prod"), asset(11, "prod-x"), asset(12, "x-prod-y")];
    const result = filterAssets(items, [], { query: "prod" });
    expect(result.map((r) => r.asset.ID)).toEqual([11, 10, 12]);
  });

  it("拼音首字母匹配", () => {
    const result = filterAssets(assets, groups, { query: "zzz" });
    expect(result.map((r) => r.asset.Name)).toContain("中转站");
  });

  it("全拼匹配", () => {
    const result = filterAssets(assets, groups, { query: "zhongzhuanzhan" });
    expect(result.map((r) => r.asset.Name)).toContain("中转站");
  });

  it("混合中英拼音匹配", () => {
    const result = filterAssets(assets, groups, { query: "fwq" });
    expect(result.map((r) => r.asset.Name)).toContain("Web服务器");
  });

  it("groupPath 原文匹配", () => {
    const result = filterAssets(assets, groups, { query: "生产" });
    const ids = result.map((r) => r.asset.ID);
    expect(ids).toEqual(expect.arrayContaining([1, 2]));
    expect(ids).not.toContain(3);
  });

  it("groupPath 拼音匹配", () => {
    const result = filterAssets(assets, groups, { query: "sc" });
    const ids = result.map((r) => r.asset.ID);
    expect(ids).toEqual(expect.arrayContaining([1, 2]));
  });

  it("name 命中排在 groupPath 命中之前", () => {
    const result = filterAssets(assets, groups, { query: "prod" });
    expect(result[0].rank).toBe(0);
    expect(result[result.length - 1].rank).toBeGreaterThanOrEqual(result[0].rank);
  });

  it("无匹配淘汰", () => {
    const result = filterAssets(assets, groups, { query: "xyz123" });
    expect(result).toEqual([]);
  });

  it("limit 截断结果", () => {
    const many = Array.from({ length: 20 }, (_, i) => asset(i + 100, `node-${i.toString().padStart(2, "0")}`));
    const result = filterAssets(many, [], { query: "node", limit: 5 });
    expect(result).toHaveLength(5);
  });

  it("同档按 zh-CN localeCompare 排序", () => {
    // 三项都 startsWith "node"（rank 0），按中文 locale 排序后顺序应稳定可预期
    const items = [asset(1, "node-c"), asset(2, "node-a"), asset(3, "node-b")];
    const result = filterAssets(items, [], { query: "node" });
    expect(result.map((r) => r.asset.Name)).toEqual(["node-a", "node-b", "node-c"]);
  });

  it("groupPath 在结果里附带", () => {
    const result = filterAssets(assets, groups, { query: "prod-db" });
    expect(result[0].groupPath).toBe("生产");
  });
});
