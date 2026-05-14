import { describe, expect, it } from "vitest";
import {
  getAssetTreeMoveBeforeId,
  type AssetTreeSortableItem,
  type AssetTreeSortableKind,
} from "@/lib/assetTreeReorder";

function beforeId({
  sortableIds,
  activeSortableId,
  overSortableId,
  targetKind,
  targetContainerId,
  containers,
}: {
  sortableIds: string[];
  activeSortableId: string;
  overSortableId: string;
  targetKind: AssetTreeSortableKind;
  targetContainerId: number;
  containers: Array<[string, number]>;
}) {
  const containerBySortableId = new Map(containers);
  return getAssetTreeMoveBeforeId({
    sortableIds,
    activeSortableId,
    overSortableId,
    targetKind,
    targetContainerId,
    getContainerId: (item: AssetTreeSortableItem) => containerBySortableId.get(`${item.kind}-${item.id}`),
  });
}

describe("getAssetTreeMoveBeforeId", () => {
  it("maps an asset drag downward to the next sibling beforeID", () => {
    expect(
      beforeId({
        sortableIds: ["asset-1", "asset-2", "asset-3"],
        activeSortableId: "asset-1",
        overSortableId: "asset-2",
        targetKind: "asset",
        targetContainerId: 10,
        containers: [
          ["asset-1", 10],
          ["asset-2", 10],
          ["asset-3", 10],
        ],
      })
    ).toBe(3);
  });

  it("maps an asset drag to the end to beforeID 0", () => {
    expect(
      beforeId({
        sortableIds: ["asset-1", "asset-2"],
        activeSortableId: "asset-1",
        overSortableId: "asset-2",
        targetKind: "asset",
        targetContainerId: 10,
        containers: [
          ["asset-1", 10],
          ["asset-2", 10],
        ],
      })
    ).toBe(0);
  });

  it("keeps upward asset drags mapped to the hovered sibling beforeID", () => {
    expect(
      beforeId({
        sortableIds: ["asset-1", "asset-2", "asset-3"],
        activeSortableId: "asset-3",
        overSortableId: "asset-2",
        targetKind: "asset",
        targetContainerId: 10,
        containers: [
          ["asset-1", 10],
          ["asset-2", 10],
          ["asset-3", 10],
        ],
      })
    ).toBe(2);
  });

  it("maps a group drag downward to the next sibling beforeID", () => {
    expect(
      beforeId({
        sortableIds: ["group-1", "asset-10", "group-2", "group-3"],
        activeSortableId: "group-1",
        overSortableId: "group-2",
        targetKind: "group",
        targetContainerId: 0,
        containers: [
          ["group-1", 0],
          ["asset-10", 1],
          ["group-2", 0],
          ["group-3", 0],
        ],
      })
    ).toBe(3);
  });

  it("maps a group drag to the end to beforeID 0", () => {
    expect(
      beforeId({
        sortableIds: ["group-1", "group-2"],
        activeSortableId: "group-1",
        overSortableId: "group-2",
        targetKind: "group",
        targetContainerId: 0,
        containers: [
          ["group-1", 0],
          ["group-2", 0],
        ],
      })
    ).toBe(0);
  });

  it("keeps upward group drags mapped to the hovered sibling beforeID", () => {
    expect(
      beforeId({
        sortableIds: ["group-1", "group-2", "group-3"],
        activeSortableId: "group-3",
        overSortableId: "group-2",
        targetKind: "group",
        targetContainerId: 0,
        containers: [
          ["group-1", 0],
          ["group-2", 0],
          ["group-3", 0],
        ],
      })
    ).toBe(2);
  });
});
