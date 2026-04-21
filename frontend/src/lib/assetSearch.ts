import { pinyinMatch } from "./pinyin";
import type { asset_entity, group_entity } from "../../wailsjs/go/models";

/** 把 group 列表解析为 groupId → "父/子/孙" 路径映射。 */
export function buildGroupPathMap(groups: group_entity.Group[]): Map<number, string> {
  const byId = new Map<number, group_entity.Group>();
  for (const g of groups) byId.set(g.ID, g);
  const cache = new Map<number, string>();
  const resolve = (id: number): string => {
    if (cache.has(id)) return cache.get(id)!;
    const g = byId.get(id);
    if (!g) return "";
    const parent = g.ParentID ? resolve(g.ParentID) : "";
    const full = parent ? `${parent}/${g.Name}` : g.Name;
    cache.set(id, full);
    return full;
  };
  const map = new Map<number, string>();
  for (const g of groups) map.set(g.ID, resolve(g.ID));
  return map;
}

export interface FilteredAsset {
  asset: asset_entity.Asset;
  groupPath: string;
  rank: number;
}

export interface FilterAssetsOptions {
  query: string;
  limit?: number;
}

function rankAsset(name: string, groupPath: string, query: string): number | null {
  if (!query) return 0;
  const lowerName = name.toLowerCase();
  const lowerQuery = query.toLowerCase();
  if (lowerName.startsWith(lowerQuery)) return 0;
  if (lowerName.includes(lowerQuery)) return 1;
  if (pinyinMatch(name, query)) return 2;
  if (groupPath) {
    const lowerPath = groupPath.toLowerCase();
    if (lowerPath.includes(lowerQuery)) return 3;
    if (pinyinMatch(groupPath, query)) return 3;
  }
  return null;
}

/** 资产搜索的统一入口：按 name 原文/拼音 + groupPath 原文/拼音 匹配，按相关度排序，可选 limit。 */
export function filterAssets(
  assets: asset_entity.Asset[],
  groups: group_entity.Group[],
  { query, limit }: FilterAssetsOptions
): FilteredAsset[] {
  const groupPathMap = buildGroupPathMap(groups);
  const trimmed = query.trim();
  const items: FilteredAsset[] = [];
  for (const asset of assets) {
    const groupPath = asset.GroupID ? groupPathMap.get(asset.GroupID) || "" : "";
    const rank = rankAsset(asset.Name, groupPath, trimmed);
    if (rank === null) continue;
    items.push({ asset, groupPath, rank });
  }
  if (trimmed) {
    items.sort((a, b) => {
      if (a.rank !== b.rank) return a.rank - b.rank;
      return a.asset.Name.localeCompare(b.asset.Name, "zh-CN");
    });
  }
  return typeof limit === "number" ? items.slice(0, limit) : items;
}
