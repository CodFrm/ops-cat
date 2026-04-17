import { useMemo, useState } from "react";
import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Popover, PopoverAnchor, PopoverContent } from "@opskat/ui";
import { useTabStore } from "@/stores/tabStore";
import { filterMatches, highlightMatch } from "@/lib/highlightMatch";
import { TabFilterInput } from "./TabFilterInput";

interface TabFilterPopoverProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Real DOM element to anchor against — typically the container of the ⋯ button */
  children: ReactElement;
}

export function TabFilterPopover({ open, onOpenChange, children }: TabFilterPopoverProps) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const tabs = useTabStore((s) => s.tabs);
  const activateTab = useTabStore((s) => s.activateTab);

  const matched = useMemo(() => tabs.filter((tab) => filterMatches(tab.label, query)), [tabs, query]);

  const activate = (id: string) => {
    activateTab(id);
    onOpenChange(false);
    setQuery("");
  };

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor asChild>{children}</PopoverAnchor>
      <PopoverContent align="end" side="bottom" sideOffset={4} className="w-[320px] p-0">
        <TabFilterInput
          autoFocus
          value={query}
          onChange={(v) => {
            setQuery(v);
            setCursor(0);
          }}
          onClose={() => onOpenChange(false)}
          onEnter={() => {
            if (matched[cursor]) activate(matched[cursor].id);
          }}
          onArrow={(dir) => {
            setCursor((c) => {
              if (matched.length === 0) return 0;
              if (dir === "up") return (c - 1 + matched.length) % matched.length;
              return (c + 1) % matched.length;
            });
          }}
        />
        <div className="max-h-[320px] overflow-y-auto py-1">
          {matched.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground">{t("sideTabs.emptyHint")}</p>
          ) : (
            matched.map((tab, idx) => (
              <button
                key={tab.id}
                onClick={() => activate(tab.id)}
                className={
                  "w-full text-left px-3 py-1.5 text-sm flex items-center gap-2 " +
                  (idx === cursor ? "bg-accent text-accent-foreground" : "hover:bg-muted")
                }
              >
                <span className="truncate flex-1">
                  {highlightMatch(tab.label, query).map((seg, i) =>
                    seg.match ? (
                      <mark key={i} className="bg-transparent text-primary font-medium">
                        {seg.text}
                      </mark>
                    ) : (
                      <span key={i}>{seg.text}</span>
                    )
                  )}
                </span>
              </button>
            ))
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
