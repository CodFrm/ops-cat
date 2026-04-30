import { createElement, useMemo, type ReactNode } from "react";
import { Folder, Server } from "lucide-react";
import type { TreeNode } from "@opskat/ui";
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";
import { useAssetStore } from "@/stores/assetStore";
import type { asset_entity, group_entity } from "../../wailsjs/go/models";

const ICON_CLASS = "h-3.5 w-3.5 shrink-0 text-muted-foreground";

/** Render the icon node for an entity that carries an optional `Icon` field (e.g. asset or group). */
function renderEntityIcon(icon: string | undefined, fallback: typeof Server): ReactNode {
  const Component = icon ? getIconComponent(icon) : fallback;
  const style = icon ? { color: getIconColor(icon) } : undefined;
  return createElement(Component, { className: ICON_CLASS, style });
}

/** Default placeholder icon shown in the AssetSelect trigger when no asset is selected. */
export const defaultAssetIcon: ReactNode = createElement(Server, { className: ICON_CLASS });
/** Default placeholder icon shown in the GroupSelect trigger when no group is selected. */
export const defaultGroupIcon: ReactNode = createElement(Folder, { className: ICON_CLASS });

/**
 * Build a TreeNode[] from assets + groups. Groups become non-selectable containers
 * (with negated IDs to avoid colliding with asset IDs); ungrouped assets are root-level.
 * Empty groups (no matching descendant assets) are pruned. Each node's icon is derived
 * from its entity's `Icon` field (via getIconComponent/getIconColor) with a fallback
 * to Server / Folder when no icon is configured.
 */
export function buildAssetTree(
  assets: asset_entity.Asset[],
  groups: group_entity.Group[],
  options: UseAssetTreeOptions = {}
): TreeNode[] {
  const visibleAssets = filterAssetTreeAssets(assets, options);
  const groupsByParent = new Map<number, group_entity.Group[]>();
  const assetsByGroup = new Map<number, asset_entity.Asset[]>();

  for (const group of groups) {
    const parentId = group.ParentID || 0;
    const siblings = groupsByParent.get(parentId) ?? [];
    siblings.push(group);
    groupsByParent.set(parentId, siblings);
  }

  for (const asset of visibleAssets) {
    const groupId = asset.GroupID || 0;
    const siblings = assetsByGroup.get(groupId) ?? [];
    siblings.push(asset);
    assetsByGroup.set(groupId, siblings);
  }

  const visit = (parentId: number): TreeNode[] =>
    (groupsByParent.get(parentId) ?? [])
      .map((g) => {
        const childGroups = visit(g.ID);
        const childAssets: TreeNode[] = (assetsByGroup.get(g.ID) ?? []).map((a) => ({
          id: a.ID,
          label: a.Name,
          icon: renderEntityIcon(a.Icon, Server),
        }));
        return {
          id: -g.ID,
          label: g.Name,
          icon: renderEntityIcon(g.Icon, Folder),
          selectable: false,
          children: [...childGroups, ...childAssets],
        };
      })
      .filter((g) => g.children && g.children.length > 0);

  const nodes: TreeNode[] = visit(0);
  for (const a of assetsByGroup.get(0) ?? []) {
    nodes.push({ id: a.ID, label: a.Name, icon: renderEntityIcon(a.Icon, Server) });
  }
  return nodes;
}

/** Collect all selectable (leaf asset) IDs under a TreeNode, recursively. */
export function collectLeafIds(node: TreeNode): number[] {
  if (node.selectable === false) {
    const ids: number[] = [];
    for (const child of node.children ?? []) ids.push(...collectLeafIds(child));
    return ids;
  }
  return [node.id];
}

export interface UseAssetTreeOptions {
  /** Filter assets by type (e.g. "ssh"). */
  filterType?: string;
  /** Asset IDs to exclude (e.g. exclude self for jump host selection). */
  excludeIds?: Iterable<number>;
  /** Only include assets with Status === 1. */
  activeOnly?: boolean;
}

/** Shared asset filtering for tree/picker UIs. Keep status/type/exclude semantics in one place. */
export function filterAssetTreeAssets(
  assets: asset_entity.Asset[],
  { filterType, excludeIds, activeOnly }: UseAssetTreeOptions = {}
): asset_entity.Asset[] {
  const exclude = excludeIds ? new Set(excludeIds) : undefined;
  return assets.filter((asset) => {
    if (activeOnly && asset.Status !== 1) return false;
    if (filterType && asset.Type !== filterType) return false;
    if (exclude?.has(asset.ID)) return false;
    return true;
  });
}

/**
 * Shared hook for asset picker components: reads assets/groups from the store,
 * applies common filters, and returns a TreeNode[] with per-entity icons resolved.
 * Use this in any new asset picker so icon/filter behaviour stays consistent.
 */
export function useAssetTree({ filterType, excludeIds, activeOnly }: UseAssetTreeOptions = {}): TreeNode[] {
  const { assets, groups } = useAssetStore();

  return useMemo(
    () => buildAssetTree(assets, groups, { filterType, excludeIds, activeOnly }),
    [assets, groups, filterType, excludeIds, activeOnly]
  );
}

export interface UseGroupTreeOptions {
  /** Group IDs to exclude (e.g. the group being edited and its descendants, to prevent cycles). */
  excludeIds?: Iterable<number>;
}

/**
 * Build a TreeNode[] of groups (no assets). Selectable leaves so the user picks one group.
 * Each node's icon comes from group.Icon with a Folder fallback.
 */
export function buildGroupTree(groups: group_entity.Group[], excludeIds?: Iterable<number>): TreeNode[] {
  const exclude = new Set<number>(excludeIds ?? []);
  const visit = (parentId: number): TreeNode[] =>
    groups
      .filter((g) => (g.ParentID || 0) === parentId && !exclude.has(g.ID))
      .map((g) => ({
        id: g.ID,
        label: g.Name,
        icon: renderEntityIcon(g.Icon, Folder),
        children: visit(g.ID),
      }));
  return visit(0);
}

/** Shared hook for group pickers — reads groups from the store and builds a TreeNode[] with icons. */
export function useGroupTree({ excludeIds }: UseGroupTreeOptions = {}): TreeNode[] {
  const { groups } = useAssetStore();

  const fullExclude = useMemo(() => {
    const ids = new Set<number>();
    if (!excludeIds) return ids;
    const seeds = Array.from(excludeIds);
    for (const seed of seeds) {
      const stack = [seed];
      while (stack.length) {
        const id = stack.pop()!;
        if (ids.has(id)) continue;
        ids.add(id);
        for (const g of groups) if ((g.ParentID || 0) === id) stack.push(g.ID);
      }
    }
    return ids;
  }, [groups, excludeIds]);

  return useMemo(() => buildGroupTree(groups, fullExclude), [groups, fullExclude]);
}
