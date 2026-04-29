import { pinyinMatch } from "./pinyin";
import { buildGroupPathMap } from "./groupPath";
import type { asset_entity, group_entity } from "../../wailsjs/go/models";

export { buildGroupPathMap } from "./groupPath";

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
