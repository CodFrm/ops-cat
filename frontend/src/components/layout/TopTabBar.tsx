import { createContext, useContext, useRef } from "react";
import { useTranslation } from "react-i18next";
import { X, MessageSquare, Server, Folder } from "lucide-react";
import { useFullscreen } from "@/hooks/useFullscreen";
import { useTabDragAndDrop } from "@/hooks/useTabDragAndDrop";
import { useTabStore, type Tab, type InfoTabMeta } from "@/stores/tabStore";
import { useTerminalStore } from "@/stores/terminalStore";
import {
  cn,
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@opskat/ui";
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";
import { TabPanelMenu } from "./TabPanelMenu";
import { TabFilterPopover } from "./TabFilterPopover";
import { useLayoutStore } from "@/stores/layoutStore";
import { getBuiltinPageMeta } from "./pageTabMeta";

interface TabBarContextValue {
  tabs: Tab[];
  dragKeyRef: React.RefObject<string | null>;
  reorder: (fromId: string, toId: string) => void;
  moveTo: (id: string, toIndex: number) => void;
}

const TabBarContext = createContext<TabBarContextValue>({
  tabs: [],
  dragKeyRef: { current: null },
  reorder: () => {},
  moveTo: () => {},
});

interface TabItemProps {
  tabKey: string;
  icon: React.ComponentType<{ className?: string; style?: React.CSSProperties }>;
  iconStyle?: React.CSSProperties;
  label: string;
  title?: string;
  isActive: boolean;
  onClick: () => void;
  onClose: () => void;
  extra?: React.ReactNode;
  indicatorColor?: string;
}

function TabItem({
  tabKey,
  icon: Icon,
  iconStyle,
  label,
  title,
  isActive,
  onClick,
  onClose,
  extra,
  indicatorColor,
}: TabItemProps) {
  const { t } = useTranslation();
  const { tabs, dragKeyRef, reorder, moveTo } = useContext(TabBarContext);
  const dragProps = useTabDragAndDrop(tabKey, { dragKeyRef, reorder });
  const noTabStyle = { "--wails-draggable": "no-drag" } as React.CSSProperties;
  const globalIndex = tabs.findIndex((tab) => tab.id === tabKey);
  const total = tabs.length;

  return (
    <ContextMenu>
      <ContextMenuTrigger className="contents">
        <div
          className={cn(
            "relative flex items-center py-2 text-sm cursor-pointer select-none transition-colors duration-150",
            "min-w-0 max-w-[200px] gap-1.5 px-3",
            isActive ? "text-foreground" : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
          )}
          style={noTabStyle}
          title={title ?? label}
          onClick={onClick}
          {...dragProps}
        >
          <Icon className="h-3.5 w-3.5 shrink-0" style={iconStyle} />
          <span className="truncate min-w-0">{label}</span>
          {extra}
          <button
            className="ml-auto shrink-0 rounded-sm p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors duration-150"
            onClick={(e) => {
              e.stopPropagation();
              onClose();
            }}
          >
            <X className="h-3 w-3" />
          </button>
          {(isActive || indicatorColor) && (
            <span
              className={cn("absolute bottom-0 left-1 right-1 h-0.5 rounded-full", !indicatorColor && "bg-primary")}
              style={indicatorColor ? { backgroundColor: indicatorColor } : undefined}
            />
          )}
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem onClick={onClose}>{t("tab.close")}</ContextMenuItem>
        <ContextMenuItem onClick={() => useTabStore.getState().closeOtherTabs(tabKey)} disabled={total <= 1}>
          {t("tab.closeOthers")}
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem onClick={() => useTabStore.getState().closeLeftTabs(tabKey)} disabled={globalIndex <= 0}>
          {t("tab.closeLeft")}
        </ContextMenuItem>
        <ContextMenuItem
          onClick={() => useTabStore.getState().closeRightTabs(tabKey)}
          disabled={globalIndex >= total - 1}
        >
          {t("tab.closeRight")}
        </ContextMenuItem>
        {total > 1 && (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem onClick={() => moveTo(tabKey, globalIndex - 1)} disabled={globalIndex <= 0}>
              {t("tab.moveLeft")}
            </ContextMenuItem>
            <ContextMenuItem onClick={() => moveTo(tabKey, globalIndex + 1)} disabled={globalIndex >= total - 1}>
              {t("tab.moveRight")}
            </ContextMenuItem>
            <ContextMenuItem onClick={() => moveTo(tabKey, 0)} disabled={globalIndex <= 0}>
              {t("tab.moveToStart")}
            </ContextMenuItem>
            <ContextMenuItem onClick={() => moveTo(tabKey, total - 1)} disabled={globalIndex >= total - 1}>
              {t("tab.moveToEnd")}
            </ContextMenuItem>
          </>
        )}
      </ContextMenuContent>
    </ContextMenu>
  );
}

export function TopTabBar() {
  const { t } = useTranslation();
  const isFullscreen = useFullscreen();

  const tabs = useTabStore((s) => s.tabs);
  const activeTabId = useTabStore((s) => s.activeTabId);
  const activateTab = useTabStore((s) => s.activateTab);
  const closeTab = useTabStore((s) => s.closeTab);
  const reorderTab = useTabStore((s) => s.reorderTab);
  const moveTabTo = useTabStore((s) => s.moveTabTo);

  const tabData = useTerminalStore((s) => s.tabData);

  const filterOpen = useLayoutStore((s) => s.filterOpen);
  const setFilterOpen = useLayoutStore((s) => s.setFilterOpen);
  const requestOpenFilter = useLayoutStore((s) => s.requestOpenFilter);

  const dragKeyRef = useRef<string | null>(null);

  const tabBarCtx: TabBarContextValue = {
    tabs,
    dragKeyRef,
    reorder: reorderTab,
    moveTo: moveTabTo,
  };

  function renderTabItem(tab: Tab) {
    const isActive = tab.id === activeTabId;

    switch (tab.type) {
      case "terminal": {
        const data = tabData[tab.id];
        const paneValues = data ? Object.values(data.panes) : [];
        const allDisconnected = paneValues.length > 0 && paneValues.every((p) => !p.connected);
        const TabIcon = tab.icon ? getIconComponent(tab.icon) : Server;
        const iconStyle = tab.icon ? { color: getIconColor(tab.icon) } : undefined;
        return (
          <TabItem
            key={tab.id}
            tabKey={tab.id}
            icon={TabIcon}
            iconStyle={iconStyle}
            label={tab.label}
            isActive={isActive}
            onClick={() => activateTab(tab.id)}
            onClose={() => closeTab(tab.id)}
            extra={allDisconnected ? <span className="h-1.5 w-1.5 rounded-full bg-destructive shrink-0" /> : undefined}
            indicatorColor={iconStyle?.color}
          />
        );
      }

      case "ai": {
        return (
          <TabItem
            key={tab.id}
            tabKey={tab.id}
            icon={MessageSquare}
            label={tab.label}
            isActive={isActive}
            onClick={() => activateTab(tab.id)}
            onClose={() => closeTab(tab.id)}
          />
        );
      }

      case "query": {
        const TabIcon = tab.icon ? getIconComponent(tab.icon) : Server;
        const iconStyle = tab.icon ? { color: getIconColor(tab.icon) } : undefined;
        return (
          <TabItem
            key={tab.id}
            tabKey={tab.id}
            icon={TabIcon}
            iconStyle={iconStyle}
            label={tab.label}
            isActive={isActive}
            onClick={() => activateTab(tab.id)}
            onClose={() => closeTab(tab.id)}
            indicatorColor={iconStyle?.color}
          />
        );
      }

      case "page": {
        const pageMeta = getBuiltinPageMeta(tab);
        if (pageMeta) {
          return (
            <TabItem
              key={tab.id}
              tabKey={tab.id}
              icon={pageMeta.icon}
              label={t(pageMeta.labelKey)}
              isActive={isActive}
              onClick={() => activateTab(tab.id)}
              onClose={() => closeTab(tab.id)}
            />
          );
        }
        // Extension page tab — use tab.icon and tab.label directly
        const TabIcon = tab.icon ? getIconComponent(tab.icon) : Server;
        const iconStyle = tab.icon ? { color: getIconColor(tab.icon) } : undefined;
        return (
          <TabItem
            key={tab.id}
            tabKey={tab.id}
            icon={TabIcon}
            iconStyle={iconStyle}
            label={tab.label}
            isActive={isActive}
            onClick={() => activateTab(tab.id)}
            onClose={() => closeTab(tab.id)}
            indicatorColor={iconStyle?.color}
          />
        );
      }

      case "info": {
        const meta = tab.meta as InfoTabMeta;
        const TabIcon = tab.icon ? getIconComponent(tab.icon) : meta.targetType === "group" ? Folder : Server;
        const iconStyle = tab.icon ? { color: getIconColor(tab.icon) } : undefined;
        return (
          <TabItem
            key={tab.id}
            tabKey={tab.id}
            icon={TabIcon}
            iconStyle={iconStyle}
            label={tab.label}
            isActive={isActive}
            onClick={() => activateTab(tab.id)}
            onClose={() => closeTab(tab.id)}
            indicatorColor={iconStyle?.color}
          />
        );
      }

      default:
        return null;
    }
  }

  return (
    <TabBarContext.Provider value={tabBarCtx}>
      <div
        data-top-tabbar
        className={`flex items-center border-b overflow-hidden bg-background ${isFullscreen ? "pt-0" : "pt-8"}`}
        style={{ "--wails-draggable": "drag" } as React.CSSProperties}
      >
        <div className="flex items-center min-w-0 flex-1">{tabs.map((tab) => renderTabItem(tab))}</div>
        <TabFilterPopover open={filterOpen} onOpenChange={setFilterOpen} tabs={tabs}>
          <div
            className="flex items-center shrink-0 px-1"
            style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
          >
            <TabPanelMenu mode="top" onOpenFilter={requestOpenFilter} />
          </div>
        </TabFilterPopover>
      </div>
    </TabBarContext.Provider>
  );
}
