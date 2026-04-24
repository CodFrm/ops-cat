import { useTranslation } from "react-i18next";
import { TreeCheckList } from "@opskat/ui";
import { useAssetTree } from "@/lib/assetTree";

interface AssetMultiSelectProps {
  values: number[];
  onValuesChange: (values: number[]) => void;
  /** Filter assets by type (e.g., "ssh"). Default: all types */
  filterType?: string;
  /** Only include assets with Status === 1 (default: true) */
  activeOnly?: boolean;
  searchPlaceholder?: string;
  emptyText?: string;
  className?: string;
}

/**
 * Multi-select asset picker rendered as an embedded tree with checkboxes.
 * Groups are non-selectable containers with tri-state selection over descendants.
 * Node icons come from each asset/group's own Icon field (see useAssetTree).
 */
export function AssetMultiSelect({
  values,
  onValuesChange,
  filterType,
  activeOnly = true,
  searchPlaceholder,
  emptyText,
  className,
}: AssetMultiSelectProps) {
  const { t } = useTranslation();
  const tree = useAssetTree({ filterType, activeOnly });

  return (
    <TreeCheckList
      values={values}
      onValuesChange={onValuesChange}
      nodes={tree}
      searchable
      searchPlaceholder={searchPlaceholder ?? t("asset.search")}
      emptyText={emptyText}
      className={className}
    />
  );
}
