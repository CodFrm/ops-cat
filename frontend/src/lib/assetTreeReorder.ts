export type AssetTreeSortableKind = "asset" | "group";

export interface AssetTreeSortableItem {
  kind: AssetTreeSortableKind;
  id: number;
}

interface GetMoveBeforeIdOptions {
  sortableIds: string[];
  activeSortableId: string;
  overSortableId: string;
  targetKind: AssetTreeSortableKind;
  targetContainerId: number;
  getContainerId: (item: AssetTreeSortableItem) => number | undefined;
}

export function parseAssetTreeSortableId(sortableId: string): AssetTreeSortableItem | null {
  const match = /^(asset|group)-(\d+)$/.exec(sortableId);
  if (!match) return null;

  return {
    kind: match[1] as AssetTreeSortableKind,
    id: Number(match[2]),
  };
}

export function getAssetTreeMoveBeforeId({
  sortableIds,
  activeSortableId,
  overSortableId,
  targetKind,
  targetContainerId,
  getContainerId,
}: GetMoveBeforeIdOptions): number | null {
  const activeIndex = sortableIds.indexOf(activeSortableId);
  const overIndex = sortableIds.indexOf(overSortableId);
  if (activeIndex < 0 || overIndex < 0) return null;

  const projectedIds = moveSortableId(sortableIds, activeIndex, overIndex);
  const movedIndex = projectedIds.indexOf(activeSortableId);
  if (movedIndex < 0) return null;

  for (let i = movedIndex + 1; i < projectedIds.length; i += 1) {
    const item = parseAssetTreeSortableId(projectedIds[i]);
    if (!item || item.kind !== targetKind) continue;
    if (getContainerId(item) === targetContainerId) {
      return item.id;
    }
  }

  return 0;
}

function moveSortableId(sortableIds: string[], fromIndex: number, toIndex: number): string[] {
  const next = [...sortableIds];
  const [moved] = next.splice(fromIndex, 1);
  next.splice(toIndex, 0, moved);
  return next;
}
