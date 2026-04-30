import React, { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Server, MessageSquare } from "lucide-react";
import { Dialog, DialogContent, DialogTitle, Input, ScrollArea, cn } from "@opskat/ui";
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";
import { filterAssets } from "@/lib/assetSearch";
import { highlightMatch, type HighlightSegment } from "@/lib/highlightMatch";
import { openAssetDefault } from "@/lib/openAssetDefault";
import { useAssetStore } from "@/stores/assetStore";
import { useRecentAssetStore } from "@/stores/recentAssetStore";
import { useTabStore, type Tab, type InfoTabMeta, type PageTabMeta } from "@/stores/tabStore";
import type { asset_entity } from "../../../wailsjs/go/models";

// ──────────────────────────────────────────────
// Props
// ──────────────────────────────────────────────

export interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
}

// ──────────────────────────────────────────────
// Row types
// ──────────────────────────────────────────────

interface TabRow {
  kind: "tab";
  id: string;
  tab: Tab;
}

interface AssetRow {
  kind: "asset";
  id: string;
  asset: asset_entity.Asset;
  groupPath: string;
}

type Row = TabRow | AssetRow;

interface Section {
  label: string;
  rows: Row[];
}

// ──────────────────────────────────────────────
// Helper: resolve asset id from a tab
// ──────────────────────────────────────────────

function tabAssetId(tab: Tab): number | null {
  if (tab.meta.type === "terminal" || tab.meta.type === "query") return tab.meta.assetId;
  if (tab.meta.type === "info" && (tab.meta as InfoTabMeta).targetType === "asset")
    return (tab.meta as InfoTabMeta).targetId;
  if (tab.meta.type === "page" && (tab.meta as PageTabMeta).assetId) return (tab.meta as PageTabMeta).assetId!;
  return null;
}

// ──────────────────────────────────────────────
// Helper: resolve icon meta for a tab
// ──────────────────────────────────────────────

interface IconMeta {
  component: React.ComponentType<{ className?: string; style?: React.CSSProperties }>;
  style?: React.CSSProperties;
  muted?: boolean;
}

function resolveTabIcon(tab: Tab): IconMeta {
  if (tab.type === "ai") return { component: MessageSquare, muted: true };
  if (tab.icon) {
    const component = getIconComponent(tab.icon);
    const color = getIconColor(tab.icon);
    return { component, style: color ? { color } : undefined };
  }
  return { component: Server, muted: true };
}

function resolveAssetIcon(asset: asset_entity.Asset): IconMeta {
  if (asset.Icon) {
    const component = getIconComponent(asset.Icon);
    const color = getIconColor(asset.Icon);
    return { component, style: color ? { color } : undefined };
  }
  return { component: Server, muted: true };
}

function renderIcon(meta: IconMeta): React.ReactNode {
  const className = cn("h-4 w-4 shrink-0", meta.muted && "text-muted-foreground");
  return React.createElement(meta.component, { className, style: meta.style });
}

// ──────────────────────────────────────────────
// Helper: highlight segments renderer
// ──────────────────────────────────────────────

function HighlightedText({ segments }: { segments: HighlightSegment[] }) {
  return (
    <>
      {segments.map((seg, i) =>
        seg.match ? (
          <mark key={i} className="bg-primary/20 text-foreground rounded-sm px-0">
            {seg.text}
          </mark>
        ) : (
          <span key={i}>{seg.text}</span>
        )
      )}
    </>
  );
}

// ──────────────────────────────────────────────
// Main component
// ──────────────────────────────────────────────

export function CommandPalette({ open, onOpenChange, onConnectAsset }: CommandPaletteProps) {
  const { t } = useTranslation();

  // Store subscriptions
  const tabs = useTabStore((s) => s.tabs);
  const assets = useAssetStore((s) => s.assets);
  const groups = useAssetStore((s) => s.groups);
  const recentIds = useRecentAssetStore((s) => s.recentIds);

  // Local state
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);

  // Reset state on open
  const prevOpen = useRef(false);
  useEffect(() => {
    if (open && !prevOpen.current) {
      setQuery("");
      setActiveIndex(0);
    }
    prevOpen.current = open;
  }, [open]);

  // Reset activeIndex when query changes
  useEffect(() => {
    setActiveIndex(0);
  }, [query]);

  // ────────────────────────────────────────────
  // Sections computation
  // ────────────────────────────────────────────

  const sections = useMemo((): Section[] => {
    const assetById = new Map(assets.map((a) => [a.ID, a]));

    if (!query.trim()) {
      // ── Empty query ──────────────────────────
      // Section 1: all open tabs
      const openedRows: TabRow[] = tabs.map((tab) => ({
        kind: "tab",
        id: `tab-${tab.id}`,
        tab,
      }));

      // Collect assetIds already in section 1
      const openedAssetIds = new Set(tabs.map(tabAssetId).filter((id): id is number => id !== null));

      // Section 2: recent assets (not already in opened, still in store, take first 5)
      const recentRows: AssetRow[] = recentIds
        .filter((id) => !openedAssetIds.has(id) && assetById.has(id))
        .slice(0, 5)
        .map((id) => {
          const asset = assetById.get(id)!;
          return {
            kind: "asset",
            id: `asset-recent-${id}`,
            asset,
            groupPath: "",
          };
        });

      const result: Section[] = [];
      if (openedRows.length > 0) {
        result.push({ label: t("commandPalette.section.opened"), rows: openedRows });
      }
      if (recentRows.length > 0) {
        result.push({ label: t("commandPalette.section.recent"), rows: recentRows });
      }
      return result;
    }

    // ── Non-empty query ──────────────────────
    const lowerQuery = query.toLowerCase();

    // Section 1: opened tabs matching query
    const matchedTabs = tabs.filter((tab) => {
      if (tab.label.toLowerCase().includes(lowerQuery)) return true;
      const assetId = tabAssetId(tab);
      if (assetId !== null) {
        const asset = assetById.get(assetId);
        if (asset && asset.Name.toLowerCase().includes(lowerQuery)) return true;
      }
      return false;
    });

    const openedRows: TabRow[] = matchedTabs.map((tab) => ({
      kind: "tab",
      id: `tab-${tab.id}`,
      tab,
    }));

    // Collect assetIds already in section 1
    const openedAssetIds = new Set(matchedTabs.map(tabAssetId).filter((id): id is number => id !== null));

    // Section 2: filtered assets (deduped against opened)
    const filtered = filterAssets(assets, groups, { query, limit: 50 });
    const assetRows: AssetRow[] = filtered
      .filter(({ asset }) => !openedAssetIds.has(asset.ID))
      .map(({ asset, groupPath }) => ({
        kind: "asset",
        id: `asset-${asset.ID}`,
        asset,
        groupPath,
      }));

    const result: Section[] = [];
    if (openedRows.length > 0) {
      result.push({ label: t("commandPalette.section.opened"), rows: openedRows });
    }
    if (assetRows.length > 0) {
      result.push({ label: t("commandPalette.section.assets"), rows: assetRows });
    }
    return result;
  }, [query, tabs, assets, groups, recentIds, t]);

  // Flat list for keyboard navigation
  const flatRows = useMemo(() => sections.flatMap((s) => s.rows), [sections]);

  // ────────────────────────────────────────────
  // Keyboard handler
  // ────────────────────────────────────────────

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.nativeEvent.isComposing) return;

    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (flatRows.length === 0) return;
      setActiveIndex((i) => Math.min(i + 1, flatRows.length - 1));
      return;
    }
    if (e.key === "ArrowUp") {
      e.preventDefault();
      if (flatRows.length === 0) return;
      setActiveIndex((i) => Math.max(i - 1, 0));
      return;
    }
    if (e.key === "Enter") {
      const row = flatRows[activeIndex];
      if (!row) return;
      activateRow(row);
      return;
    }
    // Escape is handled by Radix Dialog, we don't need to duplicate it
  };

  const activateRow = (row: Row) => {
    if (row.kind === "tab") {
      useTabStore.getState().activateTab(row.tab.id);
    } else {
      openAssetDefault(row.asset, onConnectAsset);
    }
    onOpenChange(false);
  };

  // ────────────────────────────────────────────
  // Badge helper
  // ────────────────────────────────────────────

  const badgeKey = (tab: Tab): string => {
    switch (tab.type) {
      case "terminal":
        return "commandPalette.badge.terminal";
      case "query":
        return "commandPalette.badge.query";
      case "info":
        return "commandPalette.badge.info";
      case "page":
        return "commandPalette.badge.page";
      case "ai":
        return "commandPalette.badge.ai";
      default:
        return "";
    }
  };

  // ────────────────────────────────────────────
  // Scroll active row into view
  // ────────────────────────────────────────────

  const listRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!listRef.current) return;
    const el = listRef.current.querySelector(`[data-active-index="${activeIndex}"]`);
    if (el) {
      el.scrollIntoView({ block: "nearest" });
    }
  }, [activeIndex]);

  // ────────────────────────────────────────────
  // Empty state text
  // ────────────────────────────────────────────

  const emptyKey =
    flatRows.length === 0 ? (query.trim() ? "commandPalette.empty.noMatch" : "commandPalette.empty.noContent") : null;

  // ────────────────────────────────────────────
  // Render
  // ────────────────────────────────────────────

  // Track row index across sections
  let rowIndex = 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-2xl p-0 gap-0 top-[12%] translate-y-0 data-[state=closed]:zoom-out-100 data-[state=open]:zoom-in-100 data-[state=closed]:slide-out-to-top-2 data-[state=open]:slide-in-from-top-2"
        showCloseButton={false}
      >
        <DialogTitle className="sr-only">{t("commandPalette.placeholder")}</DialogTitle>

        {/* Search input */}
        <div className="flex items-center border-b px-3">
          <Input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={t("commandPalette.placeholder")}
            className="border-0 shadow-none focus-visible:ring-0 rounded-none h-12 text-sm px-0"
          />
        </div>

        {/* Results list */}
        <ScrollArea className="max-h-96">
          <div ref={listRef} className="py-1">
            {emptyKey ? (
              <p className="px-4 py-8 text-center text-sm text-muted-foreground">{t(emptyKey)}</p>
            ) : (
              <div role="listbox" aria-label={t("commandPalette.placeholder")}>
                {sections.map((section) => (
                  <div key={section.label}>
                    <p className="px-3 py-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      {section.label}
                    </p>
                    {section.rows.map((row) => {
                      const idx = rowIndex++;
                      const isActive = idx === activeIndex;

                      if (row.kind === "tab") {
                        const { tab } = row;
                        const key = badgeKey(tab);
                        const segments = highlightMatch(tab.label, query);
                        return (
                          <button
                            key={row.id}
                            type="button"
                            role="option"
                            data-active-index={idx}
                            aria-selected={isActive}
                            className={cn(
                              "flex w-full items-center gap-2.5 px-3 py-2 cursor-pointer select-none rounded-sm mx-1 text-left",
                              isActive ? "bg-accent text-accent-foreground" : "hover:bg-accent/50"
                            )}
                            onClick={() => activateRow(row)}
                            onMouseEnter={() => setActiveIndex(idx)}
                          >
                            {/* Green dot: open tab indicator */}
                            <span className="h-1.5 w-1.5 rounded-full bg-green-500 shrink-0" />
                            {renderIcon(resolveTabIcon(tab))}
                            <span className="flex-1 truncate text-sm">
                              <HighlightedText segments={segments} />
                            </span>
                            {key && (
                              <span className="shrink-0 rounded-sm bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
                                {t(key)}
                              </span>
                            )}
                          </button>
                        );
                      }

                      // Asset row
                      const { asset, groupPath } = row;
                      const assetIconMeta = resolveAssetIcon(asset);
                      const segments = highlightMatch(asset.Name, query);

                      return (
                        <button
                          key={row.id}
                          type="button"
                          role="option"
                          data-active-index={idx}
                          aria-selected={isActive}
                          className={cn(
                            "flex w-full items-center gap-2.5 px-3 py-2 cursor-pointer select-none rounded-sm mx-1 text-left",
                            isActive ? "bg-accent text-accent-foreground" : "hover:bg-accent/50"
                          )}
                          onClick={() => activateRow(row)}
                          onMouseEnter={() => setActiveIndex(idx)}
                        >
                          {renderIcon(assetIconMeta)}
                          <span className="flex-1 min-w-0 truncate text-sm">
                            <HighlightedText segments={segments} />
                            {groupPath && <span className="ml-2 text-xs text-muted-foreground">{groupPath}</span>}
                          </span>
                        </button>
                      );
                    })}
                  </div>
                ))}
              </div>
            )}
          </div>
        </ScrollArea>

        {/* Footer */}
        <div className="flex items-center gap-3 border-t px-4 py-2 text-xs text-muted-foreground">
          <span>↑↓ {t("commandPalette.footer.navigate")}</span>
          <span>↵ {t("commandPalette.footer.open")}</span>
          <span>Esc {t("commandPalette.footer.close")}</span>
        </div>
      </DialogContent>
    </Dialog>
  );
}
