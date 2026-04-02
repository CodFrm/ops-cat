import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Folder, Server } from "lucide-react";
import { TreeSelect, type TreeNode } from "@opskat/ui";
import { useAssetStore } from "@/stores/assetStore";

interface AssetSelectProps {
  value: number;
  onValueChange: (value: number) => void;
  /** Filter assets by type (e.g., "ssh"). Default: all types */
  filterType?: string;
  /** Asset IDs to exclude (e.g., exclude self for jump host selection) */
  excludeIds?: number[];
  placeholder?: string;
  /** Custom className for the trigger button */
  className?: string;
}

const folderIcon = <Folder className="h-3.5 w-3.5 text-muted-foreground" />;
const serverIcon = <Server className="h-3.5 w-3.5 text-muted-foreground" />;

/**
 * Reusable asset selector with tree structure (groups as non-selectable containers).
 * Supports search and type filtering.
 */
export function AssetSelect({
  value,
  onValueChange,
  filterType,
  excludeIds,
  placeholder,
  className,
}: AssetSelectProps) {
  const { t } = useTranslation();
  const { assets, groups } = useAssetStore();

  const filteredAssets = useMemo(() => {
    let list = assets;
    if (filterType) list = list.filter((a) => a.Type === filterType);
    if (excludeIds?.length) list = list.filter((a) => !excludeIds.includes(a.ID));
    return list;
  }, [assets, filterType, excludeIds]);

  const tree = useMemo((): TreeNode[] => {
    const nodes: TreeNode[] = [];

    const buildGroupWithAssets = (parentId: number): TreeNode[] => {
      return groups
        .filter((g) => (g.ParentID || 0) === parentId)
        .map((g) => {
          const childGroups = buildGroupWithAssets(g.ID);
          const childAssets: TreeNode[] = filteredAssets
            .filter((a) => a.GroupID === g.ID)
            .map((a) => ({ id: a.ID, label: a.Name, icon: serverIcon }));
          return {
            id: -g.ID, // negative to avoid collision with asset IDs
            label: g.Name,
            icon: folderIcon,
            selectable: false,
            children: [...childGroups, ...childAssets],
          };
        })
        .filter((g) => g.children && g.children.length > 0);
    };

    nodes.push(...buildGroupWithAssets(0));

    // Ungrouped assets
    const ungrouped = filteredAssets.filter((a) => !a.GroupID || a.GroupID === 0);
    for (const a of ungrouped) {
      nodes.push({ id: a.ID, label: a.Name, icon: serverIcon });
    }

    return nodes;
  }, [groups, filteredAssets]);

  return (
    <TreeSelect
      value={value}
      onValueChange={onValueChange}
      nodes={tree}
      placeholder={placeholder}
      placeholderIcon={serverIcon}
      searchable
      searchPlaceholder={t("asset.search")}
      className={className}
    />
  );
}
