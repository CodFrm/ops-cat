// frontend/src/components/asset/AssetTypeFilterButton.tsx
import { useState } from "react";
import { Filter, Check } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button, Popover, PopoverContent, PopoverTrigger, ScrollArea, Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@opskat/ui";
import type { AssetTypeOption } from "@/lib/assetTypes/options";

interface AssetTypeFilterButtonProps {
  value: string[] | "all";
  options: AssetTypeOption[];
  onChange: (next: string[] | "all") => void;
}

export function AssetTypeFilterButton({ value, options, onChange }: AssetTypeFilterButtonProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  const isAll = value === "all";
  const selectedSet = isAll ? null : new Set(value);
  const activeCount = isAll ? 0 : value.length;

  const builtin = options.filter((o) => o.group === "builtin");
  const extensions = options.filter((o) => o.group === "extension");

  const tooltipLabel = isAll
    ? t("asset.filterByType")
    : t("asset.filterByTypeActive", { count: activeCount });

  const toggleAll = () => {
    onChange("all");
  };

  const toggleOne = (opt: AssetTypeOption) => {
    if (isAll) {
      // Currently "all" → remove just this one, leaving the others.
      onChange(options.filter((o) => o.value !== opt.value).map((o) => o.value));
      return;
    }
    const next = selectedSet!.has(opt.value)
      ? value.filter((v) => v !== opt.value)
      : [...value, opt.value];
    if (next.length === 0) {
      onChange("all");
      return;
    }
    onChange(next);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7 shrink-0 relative"
              aria-label={tooltipLabel}
            >
              <Filter className="h-3.5 w-3.5" />
              {!isAll && (
                <span
                  data-active="true"
                  className="absolute top-1 right-1 h-1.5 w-1.5 rounded-full bg-primary"
                />
              )}
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent side="bottom">{tooltipLabel}</TooltipContent>
      </Tooltip>
      </TooltipProvider>
      <PopoverContent align="end" side="bottom" sideOffset={4} className="w-[240px] p-0">
        <ScrollArea className="max-h-[360px]">
          <div className="py-1">
            <FilterRow
              label={t("asset.filterAllTypes")}
              checked={isAll}
              onClick={toggleAll}
            />
            <div className="my-1 mx-2 h-px bg-border" />
            {builtin.map((opt) => {
              const Icon = opt.icon;
              return (
                <FilterRow
                  key={opt.value}
                  label={opt.labelIsI18nKey ? t(opt.label) : opt.label}
                  icon={<Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
                  checked={isAll || selectedSet!.has(opt.value)}
                  onClick={() => toggleOne(opt)}
                />
              );
            })}
            {extensions.length > 0 && (
              <>
                <div className="px-3 pt-2 pb-1 text-[10px] uppercase tracking-wider text-muted-foreground">
                  {t("asset.filterExtensions")}
                </div>
                {extensions.map((opt) => {
                  const Icon = opt.icon;
                  return (
                    <FilterRow
                      key={opt.value}
                      label={opt.labelIsI18nKey ? t(opt.label) : opt.label}
                      icon={<Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
                      checked={isAll || selectedSet!.has(opt.value)}
                      onClick={() => toggleOne(opt)}
                    />
                  );
                })}
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
