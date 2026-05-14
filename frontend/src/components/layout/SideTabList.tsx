import { useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Search, Server, Folder, MessageSquare } from "lucide-react";
import { cn } from "@opskat/ui";
import { useTabStore, type Tab, type InfoTabMeta } from "@/stores/tabStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";
import { filterMatches, highlightMatch } from "@/lib/highlightMatch";
import { useLayoutStore, isCollapsed } from "@/stores/layoutStore";
import { SideTabItem, SideTabDragContext } from "./SideTabItem";
import { TabFilterInput } from "./TabFilterInput";
import { TabPanelMenu } from "./TabPanelMenu";
import { getBuiltinPageMeta, resolveTabLabel } from "./pageTabMeta";

export function SideTabList() {
  const { t } = useTranslation();
  const tabs = useTabStore((s) => s.tabs);
  const activeTabId = useTabStore((s) => s.activeTabId);
  const visibleTabs = tabs;
  const activateTab = useTabStore((s) => s.activateTab);
  const closeTab = useTabStore((s) => s.closeTab);
  const reorderTab = useTabStore((s) => s.reorderTab);
  const moveTabTo = useTabStore((s) => s.moveTabTo);
  const tabData = useTerminalStore((s) => s.tabData);

  const width = useLayoutStore((s) => s.leftPanelWidth);
  const collapsed = isCollapsed({ leftPanelWidth: width });

  const filterOpen = useLayoutStore((s) => s.filterOpen);
  const setFilterOpen = useLayoutStore((s) => s.setFilterOpen);
  const requestOpenFilter = useLayoutStore((s) => s.requestOpenFilter);
  const [query, setQuery] = useState("");
  const dragKeyRef = useRef<string | null>(null);

  const matchedWithLabel = useMemo(
    () =>
      visibleTabs
        .map((tab) => ({ tab, displayLabel: resolveTabLabel(tab, t) }))
        .filter(({ displayLabel }) => filterMatches(displayLabel, query)),
    [visibleTabs, query, t]
  );

  // 稳定 Context value —— 否则每次父组件 render 都会强制所有 SideTabItem 消费者重渲
  // 必须使用完整 tabs：SideTabItem 的 globalIndex/total 与 moveTabTo/closeLeftTabs 需要相对全量列表的索引
  const dragContextValue = useMemo(
    () => ({ dragKeyRef, reorder: reorderTab, moveTo: moveTabTo, tabs }),
    [reorderTab, moveTabTo, tabs]
  );

  const resolveMeta = (
    tab: Tab
  ): {
    Icon: React.ComponentType<{ className?: string; style?: React.CSSProperties }>;
    iconStyle?: React.CSSProperties;
    indicatorColor?: string;
    extra?: React.ReactNode;
  } => {
    if (tab.type === "terminal") {
      const data = tabData[tab.id];
      const paneValues = data ? Object.values(data.panes) : [];
      const allDisconnected = paneValues.length > 0 && paneValues.every((p) => !p.connected);
      const Icon = tab.icon ? getIconComponent(tab.icon) : Server;
      const color = tab.icon ? getIconColor(tab.icon) : undefined;
      return {
        Icon,
        iconStyle: color ? { color } : undefined,
        indicatorColor: color,
        extra: allDisconnected ? <span className="h-1.5 w-1.5 rounded-full bg-destructive shrink-0" /> : undefined,
      };
    }
    if (tab.type === "ai") {
      return { Icon: MessageSquare };
    }
    if (tab.type === "query" || tab.type === "info") {
      const Icon = tab.icon
        ? getIconComponent(tab.icon)
        : tab.type === "info" && (tab.meta as InfoTabMeta).targetType === "group"
          ? Folder
          : Server;
      const color = tab.icon ? getIconColor(tab.icon) : undefined;
      return { Icon, iconStyle: color ? { color } : undefined, indicatorColor: color };
    }
    // page
    const pm = getBuiltinPageMeta(tab);
    if (pm) return { Icon: pm.icon };
    const Icon = tab.icon ? getIconComponent(tab.icon) : Server;
    const color = tab.icon ? getIconColor(tab.icon) : undefined;
    return { Icon, iconStyle: color ? { color } : undefined, indicatorColor: color };
  };

  return (
    <div data-tab-panel className="flex flex-col h-full bg-sidebar">
      {!collapsed && (
        <div className="flex items-center gap-1 px-2 py-1.5 border-b border-panel-divider shrink-0">
          <span className="text-xs font-medium uppercase text-muted-foreground tracking-wide flex-1">
            {t("sideTabs.title")}
          </span>
          <span className="text-xs text-muted-foreground">
            {query
              ? t("sideTabs.countFiltered", { filtered: matchedWithLabel.length, total: visibleTabs.length })
              : t("sideTabs.count", { count: visibleTabs.length })}
          </span>
          <button
            type="button"
            onClick={() => setFilterOpen(!filterOpen)}
            className={cn(
              "shrink-0 rounded-sm p-1",
              filterOpen ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground"
            )}
            aria-label={t("shortcut.panel.filter")}
          >
            <Search className="h-3.5 w-3.5" />
          </button>
          <TabPanelMenu mode="side" onOpenFilter={requestOpenFilter} />
        </div>
      )}

      {!collapsed && filterOpen && (
        <TabFilterInput
          autoFocus
          value={query}
          onChange={setQuery}
          onClose={() => {
            setFilterOpen(false);
            setQuery("");
          }}
          onEnter={() => {
            if (matchedWithLabel[0]) activateTab(matchedWithLabel[0].tab.id);
          }}
        />
      )}

      <SideTabDragContext.Provider value={dragContextValue}>
        <div className="flex-1 overflow-y-auto py-1 px-1">
          {matchedWithLabel.length === 0 ? (
            <p className="px-3 py-2 text-xs text-muted-foreground text-center">
              {visibleTabs.length === 0 ? t("sideTabs.noTabs") : t("sideTabs.emptyHint")}
            </p>
          ) : (
            matchedWithLabel.map(({ tab, displayLabel }) => {
              const segments = highlightMatch(displayLabel, query);
              const meta = resolveMeta(tab);
              return (
                <SideTabItem
                  key={tab.id}
                  tab={tab}
                  isActive={tab.id === activeTabId}
                  collapsed={collapsed}
                  labelSegments={segments}
                  icon={meta.Icon}
                  iconStyle={meta.iconStyle}
                  indicatorColor={meta.indicatorColor}
                  extra={meta.extra}
                  onActivate={() => activateTab(tab.id)}
                  onClose={() => closeTab(tab.id)}
                />
              );
            })
          )}
        </div>
      </SideTabDragContext.Provider>
    </div>
  );
}
