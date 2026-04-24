import { useTranslation } from "react-i18next";
import { TreeSelect } from "@opskat/ui";
import { defaultAssetIcon, useAssetTree } from "@/lib/assetTree";

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

/**
 * Reusable asset selector with tree structure (groups as non-selectable containers).
 * Supports search and type filtering. Node icons come from each asset/group's own
 * Icon field (see useAssetTree / buildAssetTree).
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
  const tree = useAssetTree({ filterType, excludeIds });

  return (
    <TreeSelect
      value={value}
      onValueChange={onValueChange}
      nodes={tree}
      placeholder={placeholder}
      placeholderIcon={defaultAssetIcon}
      searchable
      searchPlaceholder={t("asset.search")}
      className={className}
    />
  );
}
