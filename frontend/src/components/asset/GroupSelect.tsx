import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Folder } from "lucide-react";
import { TreeSelect, type TreeNode } from "@opskat/ui";
import { useAssetStore } from "@/stores/assetStore";

interface GroupSelectProps {
  value: number;
  onValueChange: (value: number) => void;
  /** When editing a group, pass its ID to exclude self and descendants (prevents circular refs) */
  excludeGroupId?: number;
  placeholder?: string;
  /** Custom className for the trigger button */
  className?: string;
}

const folderIcon = <Folder className="h-3.5 w-3.5 text-muted-foreground" />;

/**
 * Reusable group selector with tree structure.
 * Supports search and circular reference prevention.
 */
export function GroupSelect({ value, onValueChange, excludeGroupId, placeholder, className }: GroupSelectProps) {
  const { t } = useTranslation();
  const { groups } = useAssetStore();

  const tree = useMemo((): TreeNode[] => {
    // Collect IDs to exclude (self + all descendants)
    const excludeIds = new Set<number>();
    if (excludeGroupId) {
      const collectDescendants = (id: number) => {
        excludeIds.add(id);
        for (const g of groups.filter((g) => (g.ParentID || 0) === id)) {
          collectDescendants(g.ID);
        }
      };
      collectDescendants(excludeGroupId);
    }

    const buildChildren = (parentId: number): TreeNode[] => {
      return groups
        .filter((g) => (g.ParentID || 0) === parentId && !excludeIds.has(g.ID))
        .map((g) => ({
          id: g.ID,
          label: g.Name,
          icon: folderIcon,
          children: buildChildren(g.ID),
        }));
    };

    return buildChildren(0);
  }, [groups, excludeGroupId]);

  return (
    <TreeSelect
      value={value}
      onValueChange={onValueChange}
      nodes={tree}
      placeholder={placeholder || t("asset.ungrouped")}
      placeholderIcon={folderIcon}
      searchable
      searchPlaceholder={t("asset.search")}
      className={className}
    />
  );
}
