import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { TreeSelect } from "@opskat/ui";
import { defaultGroupIcon, useGroupTree } from "@/lib/assetTree";

interface GroupSelectProps {
  value: number;
  onValueChange: (value: number) => void;
  /** When editing a group, pass its ID to exclude self and descendants (prevents circular refs) */
  excludeGroupId?: number;
  placeholder?: string;
  /** Custom className for the trigger button */
  className?: string;
}

/**
 * Reusable group selector with tree structure.
 * Node icons come from each group's own Icon field (see useGroupTree / buildGroupTree).
 * Supports search and circular reference prevention.
 */
export function GroupSelect({ value, onValueChange, excludeGroupId, placeholder, className }: GroupSelectProps) {
  const { t } = useTranslation();
  const excludeIds = useMemo(() => (excludeGroupId ? [excludeGroupId] : undefined), [excludeGroupId]);
  const tree = useGroupTree({ excludeIds });

  return (
    <TreeSelect
      value={value}
      onValueChange={onValueChange}
      nodes={tree}
      placeholder={placeholder || t("asset.ungrouped")}
      placeholderIcon={defaultGroupIcon}
      searchable
      searchPlaceholder={t("asset.search")}
      className={className}
    />
  );
}
