import { useMemo, useState, useEffect } from "react";
import type { ReactElement } from "react";
import { useTranslation } from "react-i18next";
import { Popover, PopoverAnchor, PopoverContent } from "@opskat/ui";
import { useTabStore, type Tab } from "@/stores/tabStore";
import { filterMatches, highlightMatch } from "@/lib/highlightMatch";
import { TabFilterInput } from "./TabFilterInput";
import { resolveTabLabel } from "./pageTabMeta";

interface TabFilterPopoverProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Real DOM element to anchor against — typically the container of the ⋯ button */
  children: ReactElement;
  tabs: Tab[];
}

export function TabFilterPopover({ open, onOpenChange, children, tabs }: TabFilterPopoverProps) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const activateTab = useTabStore((s) => s.activateTab);

  useEffect(() => {
    if (!open) {
      setQuery("");
      setCursor(0);
    }
  }, [open]);

  const items = useMemo(() => tabs.map((tab) => ({ tab, label: resolveTabLabel(tab, t) })), [tabs, t]);

  const matched = useMemo(() => items.filter(({ label }) => filterMatches(label, query)), [items, query]);

  const activate = (id: string) => {
    activateTab(id);
    onOpenChange(false);
    setQuery("");
  };

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor asChild>{children}</PopoverAnchor>
      <PopoverContent
        align="end"
        side="bottom"
        sideOffset={4}
        className="w-[320px] p-0"
        onOpenAutoFocus={(e) => {
          // Let the input's own autoFocus handle focus; Radix's default focus move
          // can race with the still-animating DropdownMenu above.
          e.preventDefault();
        }}
        onFocusOutside={(e) => {
          // The sibling DropdownMenu (via which the user arrived here) animates out
          // over ~150ms. During that window it can fire "focus outside" in two ways:
          //   1) pointer hover steals focus onto a still-mounted closing menu item
          //   2) the focused item unmounts and focus falls to <body>
          // Both would dismiss this popover. Ignore them.
          const t = e.target as HTMLElement | null;
          if (!t || t === document.body || t.closest('[data-slot="dropdown-menu-content"]')) {
            e.preventDefault();
          }
        }}
        onPointerDownOutside={(e) => {
          const t = e.target as HTMLElement | null;
          if (t?.closest('[data-slot="dropdown-menu-content"]')) e.preventDefault();
        }}
      >
        <TabFilterInput
          autoFocus
          value={query}
          onChange={(v) => {
            setQuery(v);
            setCursor(0);
          }}
          onClose={() => onOpenChange(false)}
          onEnter={() => {
            if (matched[cursor]) activate(matched[cursor].tab.id);
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
            matched.map(({ tab, label }, idx) => (
              <button
                key={tab.id}
                onClick={() => activate(tab.id)}
                className={
                  "w-full text-left px-3 py-1.5 text-sm flex items-center gap-2 " +
                  (idx === cursor ? "bg-accent text-accent-foreground" : "hover:bg-muted")
                }
              >
                <span className="truncate flex-1">
                  {highlightMatch(label, query).map((seg, i) =>
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
