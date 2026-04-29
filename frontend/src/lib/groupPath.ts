import type { asset_entity, group_entity } from "../../wailsjs/go/models";

interface GroupPathOptions {
  separator?: string;
}

const DEFAULT_SEPARATOR = "/";

function buildGroupLookup(groups: group_entity.Group[]): Map<number, group_entity.Group> {
  const byId = new Map<number, group_entity.Group>();
  for (const group of groups) byId.set(group.ID, group);
  return byId;
}

function resolveGroupPathFromLookup(
  byId: Map<number, group_entity.Group>,
  groupId: number | undefined,
  separator: string
): string {
  if (!groupId) return "";

  const names: string[] = [];
  const seen = new Set<number>();
  let currentId = groupId;

  while (currentId > 0) {
    if (seen.has(currentId)) break;
    seen.add(currentId);

    const group = byId.get(currentId);
    if (!group) break;

    names.unshift(group.Name);
    currentId = group.ParentID || 0;
  }

  return names.join(separator);
}

export function resolveGroupPath(
  groups: group_entity.Group[],
  groupId: number | undefined,
  { separator = DEFAULT_SEPARATOR }: GroupPathOptions = {}
): string {
  return resolveGroupPathFromLookup(buildGroupLookup(groups), groupId, separator);
}

/** 把 group 列表解析为 groupId -> "父/子/孙" 路径映射。异常 parent 环会在当前视角截断，避免缓存污染。 */
export function buildGroupPathMap(
  groups: group_entity.Group[],
  { separator = DEFAULT_SEPARATOR }: GroupPathOptions = {}
): Map<number, string> {
  const byId = buildGroupLookup(groups);
  const map = new Map<number, string>();
  for (const group of groups) {
    map.set(group.ID, resolveGroupPathFromLookup(byId, group.ID, separator));
  }
  return map;
}

export function formatAssetPath(
  asset: asset_entity.Asset,
  groups: group_entity.Group[],
  { separator = DEFAULT_SEPARATOR }: GroupPathOptions = {}
): string {
  const groupPath = resolveGroupPath(groups, asset.GroupID, { separator });
  return groupPath ? `${groupPath}${separator}${asset.Name}` : asset.Name;
}
