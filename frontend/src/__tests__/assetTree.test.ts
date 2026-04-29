import { describe, expect, it } from "vitest";
import { buildAssetTree, collectLeafIds } from "@/lib/assetTree";
import type { TreeNode } from "@opskat/ui";
import type { asset_entity, group_entity } from "../../wailsjs/go/models";

function asset(id: number, name: string, type: string, groupId = 0, status = 1): asset_entity.Asset {
  return { ID: id, Name: name, Type: type, GroupID: groupId, Status: status } as asset_entity.Asset;
}

function group(id: number, name: string, parentId = 0): group_entity.Group {
  return { ID: id, Name: name, ParentID: parentId } as group_entity.Group;
}

function selectableIds(nodes: TreeNode[]): number[] {
  return nodes.flatMap((node) => collectLeafIds(node));
}

describe("buildAssetTree", () => {
  it("applies common picker filters before building tree nodes", () => {
    const groups = [group(10, "SSH"), group(20, "Database")];
    const assets = [
      asset(1, "ssh-a", "ssh", 10),
      asset(2, "ssh-b", "ssh", 10),
      asset(3, "inactive-ssh", "ssh", 10, 2),
      asset(4, "mysql", "database", 20),
      asset(5, "root-ssh", "ssh"),
    ];

    const tree = buildAssetTree(assets, groups, {
      filterType: "ssh",
      activeOnly: true,
      excludeIds: [2],
    });

    expect(tree.map((node: TreeNode) => node.label)).toEqual(["SSH", "root-ssh"]);
    expect(selectableIds(tree)).toEqual([1, 5]);
  });
});
