import { useState } from "react";
import { Filter, Check } from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  Button,
  Popover,
  PopoverContent,
  PopoverTrigger,
  ScrollArea,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@opskat/ui";
import type { AssetTypeOption } from "@/lib/assetTypes/options";

interface AssetTypeFilterButtonProps {
  value: string[];
  options: AssetTypeOption[];
  onChange: (next: string[]) => void;
  hideEmptyGroups?: boolean;
  onHideEmptyGroupsChange?: (next: boolean) => void;
}

export function AssetTypeFilterButton({
  value,
  options,
  onChange,
  hideEmptyGroups,
  onHideEmptyGroupsChange,
}: AssetTypeFilterButtonProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  const selectedSet = new Set(value);
  const activeCount = value.length;
  const allChecked = options.length > 0 && activeCount === options.length;
  const anyFilterActive = activeCount > 0 || !!hideEmptyGroups;

  const builtin = options.filter((o) => o.group === "builtin");
  const extensions = options.filter((o) => o.group === "extension");

  const tooltipLabel =
    activeCount === 0 ? t("asset.filterByType") : t("asset.filterByTypeActive", { count: activeCount });

  const toggleAll = () => {
    onChange(allChecked ? [] : options.map((o) => o.value));
  };

  const toggleOne = (opt: AssetTypeOption) => {
    onChange(selectedSet.has(opt.value) ? value.filter((v) => v !== opt.value) : [...value, opt.value]);
  };

  const renderRow = (opt: AssetTypeOption) => {
    const Icon = opt.icon;
    return (
      <FilterRow
        key={opt.value}
        label={opt.labelIsI18nKey ? t(opt.label) : opt.label}
        icon={<Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
        checked={selectedSet.has(opt.value)}
        onClick={() => toggleOne(opt)}
      />
    );
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button variant="ghost" size="icon" className="h-7 w-7 shrink-0 relative" aria-label={tooltipLabel}>
              <Filter className="h-3.5 w-3.5" />
              {anyFilterActive && (
                <span data-active="true" className="absolute top-1 right-1 h-1.5 w-1.5 rounded-full bg-primary" />
              )}
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent side="bottom">{tooltipLabel}</TooltipContent>
      </Tooltip>
      <PopoverContent align="end" side="bottom" sideOffset={4} className="w-[240px] p-0">
        <ScrollArea className="max-h-[360px]">
          <div className="py-1">
            <FilterRow label={t("asset.filterAllTypes")} checked={allChecked} onClick={toggleAll} />
            <div className="my-1 mx-2 h-px bg-border" />
            {builtin.map(renderRow)}
            {extensions.length > 0 && (
              <>
                <div className="px-3 pt-2 pb-1 text-[10px] uppercase tracking-wider text-muted-foreground">
                  {t("asset.filterExtensions")}
                </div>
                {extensions.map(renderRow)}
              </>
            )}
            {onHideEmptyGroupsChange && (
              <>
                <div className="my-1 mx-2 h-px bg-border" />
                <FilterRow
                  label={t("asset.filterHideEmptyGroups")}
                  checked={!!hideEmptyGroups}
                  onClick={() => onHideEmptyGroupsChange(!hideEmptyGroups)}
                />
              </>
            )}
          </div>
        </ScrollArea>
      </PopoverContent>
    </Popover>
  );
}

function FilterRow({
  label,
  icon,
  checked,
  onClick,
}: {
  label: string;
  icon?: React.ReactNode;
  checked: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-2 px-3 py-1.5 text-xs hover:bg-accent text-left"
    >
      <span className="flex h-3.5 w-3.5 items-center justify-center shrink-0">
        {checked ? <Check className="h-3.5 w-3.5 text-primary" /> : null}
      </span>
      {icon}
      <span className="flex-1 truncate">{label}</span>
    </button>
  );
}
