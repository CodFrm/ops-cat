import { forwardRef, useImperativeHandle, useMemo, useState } from "react";
import { flushSync } from "react-dom";
import { useTranslation } from "react-i18next";
import { Database, Server, Table2 } from "lucide-react";
import { useAssetStore } from "@/stores/assetStore";
import { useQueryStore, type DatabaseTabState } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta, type Tab } from "@/stores/tabStore";
import { filterAssets } from "@/lib/assetSearch";
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";
import { pinyinMatch } from "@/lib/pinyin";

export interface MentionItem {
  id: number;
  label: string;
  type: string;
  icon: string;
  groupPath: string;
  kind: "asset" | "database" | "table";
  database?: string;
  table?: string;
  driver?: string;
}

export interface MentionListProps {
  query: string;
  command: (item: MentionItem) => void;
}

export interface MentionListRef {
  onKeyDown: (props: { event: KeyboardEvent }) => boolean;
}

const MAX_ITEMS = 8;

interface ActiveDatabaseTab {
  id: string;
  meta: QueryTabMeta;
}

interface RankedMentionItem {
  item: MentionItem;
  sourceRank: number;
  matchRank: number;
  order: number;
}

function resolveActiveDatabaseTab(tabs: Tab[], activeTabId: string | null): ActiveDatabaseTab | null {
  const tab = tabs.find((item) => item.id === activeTabId);
  if (!tab || tab.type !== "query") return null;
  const meta = tab.meta as QueryTabMeta;
  if (meta.assetType !== "database") return null;
  return { id: tab.id, meta };
}

function itemKey(item: MentionItem) {
  return `${item.kind}:${item.id}:${item.database ?? ""}:${item.table ?? ""}`;
}

function fieldMatchRank(value: string | undefined, query: string): number | null {
  if (!value) return null;
  const lowerValue = value.toLowerCase();
  const lowerQuery = query.toLowerCase();
  if (lowerValue.startsWith(lowerQuery)) return 0;
  if (lowerValue.includes(lowerQuery)) return 1;
  if (pinyinMatch(value, query)) return 2;
  return null;
}

function bestRank(values: Array<string | undefined>, query: string): number | null {
  let best: number | null = null;
  for (const value of values) {
    const rank = fieldMatchRank(value, query);
    if (rank === null) continue;
    best = best === null ? rank : Math.min(best, rank);
  }
  return best;
}

function mentionMatchRank(item: MentionItem, query: string): number | null {
  const q = query.trim();
  if (!q) return 0;
  const primary =
    item.kind === "table" ? [item.table] : item.kind === "database" ? [item.database, item.label] : [item.label];
  const primaryRank = bestRank(primary, q);
  if (primaryRank !== null) return primaryRank;

  const path = [item.groupPath, item.database, item.table].filter(Boolean).join(" ");
  const contextRank = bestRank([item.label, item.groupPath, item.database, item.driver, item.type, path], q);
  return contextRank === null ? null : contextRank + 3;
}

function queryKindRank(item: MentionItem, sourceRank: number) {
  if (sourceRank === 0) return 0;
  if (item.kind === "table") return 1;
  if (item.kind === "database") return 2;
  return 3;
}

function buildDatabaseMentionItems(activeTab: ActiveDatabaseTab | null, dbState: DatabaseTabState | undefined) {
  if (!activeTab || !dbState) return [];
  const out: Array<{ item: MentionItem; sourceRank: number }> = [];
  const seen = new Set<string>();
  const meta = activeTab.meta;
  const base = {
    id: meta.assetId,
    type: meta.assetType,
    icon: meta.assetIcon,
    groupPath: meta.assetName,
    driver: meta.driver,
  };
  const push = (item: MentionItem, sourceRank: number) => {
    const key = itemKey(item);
    if (seen.has(key)) return;
    seen.add(key);
    out.push({ item, sourceRank });
  };
  const databaseItem = (database: string): MentionItem => ({ ...base, kind: "database", label: database, database });
  const tableItem = (database: string, table: string): MentionItem => ({
    ...base,
    kind: "table",
    label: `${database}.${table}`,
    database,
    table,
  });
  const pushDatabase = (database: string | undefined) => {
    if (!database) return;
    push(databaseItem(database), database === meta.defaultDatabase ? 1 : 2);
  };
  const pushTable = (database: string | undefined, table: string | undefined) => {
    if (!database || !table) return;
    push(tableItem(database, table), 5);
  };
  const activeInner = dbState.innerTabs.find((tab) => tab.id === dbState.activeInnerTabId);
  if (activeInner?.type === "table") {
    push(tableItem(activeInner.database, activeInner.table), 0);
  } else if (activeInner?.type === "sql") {
    if (activeInner.selectedDb) push(databaseItem(activeInner.selectedDb), 0);
  }
  pushDatabase(meta.defaultDatabase);
  for (const tab of dbState.innerTabs) {
    if (tab.type === "table") {
      const item = tableItem(tab.database, tab.table);
      push(item, 4);
    }
  }
  for (const db of dbState.databases) {
    const tables = dbState.tables[db] ?? [];
    for (const table of tables) {
      pushTable(db, table);
    }
  }
  for (const [db, tables] of Object.entries(dbState.tables)) {
    if (dbState.databases.includes(db)) continue;
    for (const table of tables) {
      pushTable(db, table);
    }
  }
  return out;
}

export const MentionList = forwardRef<MentionListRef, MentionListProps>(function MentionList({ query, command }, ref) {
  const { t } = useTranslation();
  const { assets, groups } = useAssetStore();
  const tabs = useTabStore((s) => s.tabs);
  const activeTabId = useTabStore((s) => s.activeTabId);
  const dbStates = useQueryStore((s) => s.dbStates);
  const [selection, setSelection] = useState({ itemCount: 0, index: 0 });

  const activeDatabaseTab = useMemo(() => resolveActiveDatabaseTab(tabs, activeTabId), [activeTabId, tabs]);
  const activeDbState = activeDatabaseTab ? dbStates[activeDatabaseTab.id] : undefined;

  const items: MentionItem[] = useMemo(() => {
    let order = 0;
    const ranked: RankedMentionItem[] = [];
    const addItem = (item: MentionItem, sourceRank: number) => {
      const matchRank = mentionMatchRank(item, query);
      if (matchRank === null) return;
      ranked.push({ item, sourceRank, matchRank, order });
      order += 1;
    };

    for (const { item, sourceRank } of buildDatabaseMentionItems(activeDatabaseTab, activeDbState)) {
      addItem(item, sourceRank);
    }
    for (const { asset, groupPath } of filterAssets(assets, groups, { query, limit: MAX_ITEMS })) {
      addItem(
        {
          id: asset.ID,
          label: asset.Name,
          type: asset.Type,
          icon: asset.Icon,
          groupPath,
          kind: "asset",
        },
        3
      );
    }

    const hasQuery = query.trim().length > 0;
    ranked.sort((a, b) => {
      if (hasQuery) {
        if (a.matchRank !== b.matchRank) return a.matchRank - b.matchRank;
        const aKindRank = queryKindRank(a.item, a.sourceRank);
        const bKindRank = queryKindRank(b.item, b.sourceRank);
        if (aKindRank !== bKindRank) return aKindRank - bKindRank;
      }
      if (a.sourceRank !== b.sourceRank) return a.sourceRank - b.sourceRank;
      return a.order - b.order;
    });

    const seen = new Set<string>();
    const out: MentionItem[] = [];
    for (const rankedItem of ranked) {
      const key = itemKey(rankedItem.item);
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(rankedItem.item);
      if (out.length >= MAX_ITEMS) break;
    }
    return out;
  }, [activeDatabaseTab, activeDbState, assets, groups, query]);
  const selectedIndex = selection.itemCount === items.length ? selection.index : 0;

  useImperativeHandle(ref, () => ({
    onKeyDown: ({ event }) => {
      if (event.key === "ArrowUp") {
        flushSync(() =>
          setSelection((current) => {
            const currentIndex = current.itemCount === items.length ? current.index : 0;
            return { itemCount: items.length, index: (currentIndex + items.length - 1) % Math.max(items.length, 1) };
          })
        );
        return true;
      }
      if (event.key === "ArrowDown") {
        flushSync(() =>
          setSelection((current) => {
            const currentIndex = current.itemCount === items.length ? current.index : 0;
            return { itemCount: items.length, index: (currentIndex + 1) % Math.max(items.length, 1) };
          })
        );
        return true;
      }
      if (event.key === "Enter") {
        const item = items[selectedIndex];
        if (item) command(item);
        return true;
      }
      return false;
    },
  }));

  if (assets.length === 0 && !activeDbState) return null;

  if (items.length === 0) {
    return (
      <div className="bg-popover text-popover-foreground rounded-md border shadow-md px-3 py-2 text-xs text-muted-foreground">
        {t("ai.mentionNotFound")}
      </div>
    );
  }

  return (
    <div
      role="listbox"
      className="bg-popover text-popover-foreground rounded-md border shadow-md overflow-hidden min-w-[240px] max-w-[360px]"
    >
      {items.map((item, idx) => {
        const Icon =
          item.kind === "database"
            ? Database
            : item.kind === "table"
              ? Table2
              : item.icon
                ? getIconComponent(item.icon)
                : Server;
        return (
          <button
            role="option"
            aria-selected={idx === selectedIndex}
            key={itemKey(item)}
            onClick={() => command(item)}
            className={
              "flex items-center gap-2 w-full px-2.5 py-1.5 text-xs text-left " +
              (idx === selectedIndex ? "bg-accent" : "hover:bg-accent/60")
            }
          >
            <Icon
              className="h-3.5 w-3.5 shrink-0 text-muted-foreground"
              style={item.kind === "asset" && item.icon ? { color: getIconColor(item.icon) } : undefined}
            />
            <span className="flex-1 min-w-0 truncate">
              {item.groupPath && <span className="text-muted-foreground">{item.groupPath}/</span>}
              <span className="text-foreground">{item.label}</span>
            </span>
          </button>
        );
      })}
    </div>
  );
});
