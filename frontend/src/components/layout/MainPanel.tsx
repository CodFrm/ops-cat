import { createContext, useCallback, useContext, useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  X,
  Settings,
  KeyRound,
  MessageSquare,
  ScrollText,
  ArrowRightLeft,
  Server,
  Folder,
  Loader2,
} from "lucide-react";
import logoLight from "@/assets/images/logo.png";
import logoDark from "@/assets/images/logo-dark.png";
import { useFullscreen } from "@/hooks/useFullscreen";
import { AssetDetail } from "@/components/asset/AssetDetail";
import { GroupDetail } from "@/components/asset/GroupDetail";
import { SplitPane } from "@/components/terminal/SplitPane";
import { SessionToolbar } from "@/components/terminal/SessionToolbar";
import { TerminalToolbar } from "@/components/terminal/TerminalToolbar";
import { FileManagerPanel } from "@/components/terminal/FileManagerPanel";
import { SettingsPage } from "@/components/settings/SettingsPage";
import { CredentialManager } from "@/components/settings/CredentialManager";
import { AuditLogPage } from "@/components/audit/AuditLogPage";
import { PortForwardPage } from "@/components/forward/PortForwardPage";
import { AIChatContent } from "@/components/ai/AIChatContent";
import { DatabasePanel } from "@/components/query/DatabasePanel";
import { RedisPanel } from "@/components/query/RedisPanel";
import { useTerminalStore } from "@/stores/terminalStore";
import { useAssetStore } from "@/stores/assetStore";
import { useTabStore, type Tab, type QueryTabMeta, type PageTabMeta, type InfoTabMeta } from "@/stores/tabStore";
import { useSFTPStore } from "@/stores/sftpStore";
import {
  cn,
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@opskat/ui";
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";
import { asset_entity } from "../../../wailsjs/go/models";
import { ExtensionPage } from "@/extension";

const pageTabMeta: Record<string, { icon: typeof Settings; labelKey: string }> = {
  settings: { icon: Settings, labelKey: "nav.settings" },
  forward: { icon: ArrowRightLeft, labelKey: "nav.forward" },
  sshkeys: { icon: KeyRound, labelKey: "nav.sshKeys" },
  audit: { icon: ScrollText, labelKey: "nav.audit" },
};

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
}

function TabItem({ tabKey, icon: Icon, iconStyle, label, title, isActive, onClick, onClose, extra }: TabItemProps) {
  const { t } = useTranslation();
  const { tabs, dragKeyRef, reorder, moveTo } = useContext(TabBarContext);
  const noTabStyle = { "--wails-draggable": "no-drag" } as React.CSSProperties;
  const globalIndex = tabs.findIndex((tab) => tab.id === tabKey);
  const total = tabs.length;

  return (
    <ContextMenu>
      <ContextMenuTrigger>
        <div
          className={cn(
            "relative flex items-center gap-1.5 px-3 py-2 text-sm shrink-0 cursor-pointer transition-colors duration-150",
            isActive ? "text-foreground" : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
          )}
          style={noTabStyle}
          title={title ?? label}
          onClick={onClick}
          draggable
          onDragStart={(e) => {
            dragKeyRef.current = tabKey;
            e.dataTransfer.effectAllowed = "move";
          }}
          onDragOver={(e) => {
            if (dragKeyRef.current) {
              e.preventDefault();
              e.dataTransfer.dropEffect = "move";
            }
          }}
          onDragEnter={(e) => {
            e.preventDefault();
            if (!dragKeyRef.current || dragKeyRef.current === tabKey) return;
            reorder(dragKeyRef.current, tabKey);
          }}
          onDragEnd={() => {
            dragKeyRef.current = null;
          }}
        >
          <Icon className="h-3.5 w-3.5 shrink-0" style={iconStyle} />
          <span className="max-w-24 truncate">{label}</span>
          {extra}
          <button
            className="ml-1.5 rounded-sm p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors duration-150"
            onClick={(e) => {
              e.stopPropagation();
              onClose();
            }}
          >
            <X className="h-3 w-3" />
          </button>
          {isActive && <span className="absolute bottom-0 left-1 right-1 h-0.5 rounded-full bg-primary" />}
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

interface MainPanelProps {
  onEditAsset: (asset: asset_entity.Asset) => void;
  onDeleteAsset: (id: number) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
}

export function MainPanel({ onEditAsset, onDeleteAsset, onConnectAsset }: MainPanelProps) {
  const { t } = useTranslation();
  const isFullscreen = useFullscreen();

  const tabs = useTabStore((s) => s.tabs);
  const activeTabId = useTabStore((s) => s.activeTabId);
  const activateTab = useTabStore((s) => s.activateTab);
  const closeTab = useTabStore((s) => s.closeTab);
  const reorderTab = useTabStore((s) => s.reorderTab);
  const moveTabTo = useTabStore((s) => s.moveTabTo);

  const tabData = useTerminalStore((s) => s.tabData);
  const connectingAssetIds = useTerminalStore((s) => s.connectingAssetIds);

  const { assets, groups, initialized } = useAssetStore();
  const { fileManagerOpenTabs, fileManagerWidth, setFileManagerWidth } = useSFTPStore();

  const dragKeyRef = useRef<string | null>(null);
  const tabBarRef = useRef<HTMLDivElement>(null);
  const wheelListenerRef = useRef<((e: WheelEvent) => void) | null>(null);

  const tabBarCallbackRef = useCallback((el: HTMLDivElement | null) => {
    if (tabBarRef.current && wheelListenerRef.current) {
      tabBarRef.current.removeEventListener("wheel", wheelListenerRef.current);
      wheelListenerRef.current = null;
    }
    tabBarRef.current = el;
    if (el) {
      const onWheel = (e: WheelEvent) => {
        if (e.deltaY !== 0) {
          e.preventDefault();
          el.scrollLeft += e.deltaY;
        }
      };
      wheelListenerRef.current = onWheel;
      el.addEventListener("wheel", onWheel, { passive: false });
    }
  }, []);

  const activeTab = tabs.find((tab) => tab.id === activeTabId) ?? null;
  const hasTabs = tabs.length > 0;

  // Collect all terminal tabs for visibility-based rendering
  const terminalTabs = tabs.filter((tab) => tab.type === "terminal");
  // Collect AI tabs for visibility-based rendering
  const aiTabs = tabs.filter((tab) => tab.type === "ai");
  // Collect query tabs for visibility-based rendering
  const queryTabs = tabs.filter((tab) => tab.type === "query");

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
          />
        );
      }

      case "page": {
        const meta = tab.meta as PageTabMeta;
        const pageMeta = pageTabMeta[meta.pageId];
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
          />
        );
      }

      default:
        return null;
    }
  }

  function renderActiveContent() {
    if (!activeTab) return null;

    switch (activeTab.type) {
      case "page": {
        const meta = activeTab.meta as PageTabMeta;
        switch (meta.pageId) {
          case "settings":
            return (
              <div className="absolute inset-0 bg-background">
                <SettingsPage />
              </div>
            );
          case "sshkeys":
            return (
              <div className="absolute inset-0 bg-background flex flex-col">
                <div className="px-4 py-3 border-b">
                  <h2 className="font-semibold">{t("nav.sshKeys")}</h2>
                </div>
                <div className="flex-1 overflow-y-auto p-4">
                  <div className="max-w-4xl mx-auto">
                    <CredentialManager />
                  </div>
                </div>
              </div>
            );
          case "audit":
            return (
              <div className="absolute inset-0 bg-background">
                <AuditLogPage />
              </div>
            );
          case "forward":
            return (
              <div className="absolute inset-0 bg-background">
                <PortForwardPage />
              </div>
            );
          default:
            if (meta.extensionName) {
              return <ExtensionPage extensionName={meta.extensionName} pageId={meta.pageId} assetId={meta.assetId} />;
            }
            return null;
        }
      }

      case "info": {
        const meta = activeTab.meta as InfoTabMeta;
        if (meta.targetType === "asset") {
          const asset = assets.find((a) => a.ID === meta.targetId);
          if (!asset) {
            if (!initialized) {
              return (
                <div className="flex items-center justify-center h-full">
                  <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                </div>
              );
            }
            return null;
          }
          return (
            <AssetDetail
              asset={asset}
              isConnecting={connectingAssetIds.has(asset.ID)}
              onEdit={() => onEditAsset(asset)}
              onDelete={() => onDeleteAsset(asset.ID)}
              onConnect={() => onConnectAsset(asset)}
            />
          );
        } else {
          const group = groups.find((g) => g.ID === meta.targetId);
          if (!group) {
            if (!initialized) {
              return (
                <div className="flex items-center justify-center h-full">
                  <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                </div>
              );
            }
            return null;
          }
          return <GroupDetail group={group} />;
        }
      }

      default:
        // terminal, ai, query are rendered via visibility pattern below
        return null;
    }
  }

  return (
    <div className="flex flex-1 flex-col min-w-0">
      {/* When no tabs, show standalone drag region */}
      {!hasTabs && (
        <div
          className={`${isFullscreen ? "h-2" : "h-10"} w-full shrink-0`}
          style={{ "--wails-draggable": "drag" } as React.CSSProperties}
        />
      )}

      {/* Tab bar with integrated drag region */}
      {hasTabs && (
        <TabBarContext.Provider value={tabBarCtx}>
          <div
            ref={tabBarCallbackRef}
            className={`flex items-center border-b overflow-x-auto bg-background ${isFullscreen ? "pt-2" : "pt-10"}`}
            style={{ "--wails-draggable": "drag" } as React.CSSProperties}
          >
            {tabs.map((tab) => renderTabItem(tab))}
          </div>
        </TabBarContext.Provider>
      )}

      {/* Content area */}
      <div className="flex-1 relative min-h-0 overflow-hidden">
        {/* Terminal tabs: visibility-based to preserve xterm state */}
        {terminalTabs.map((tab) => {
          const data = tabData[tab.id];
          const isActive = activeTabId === tab.id;
          return (
            <div
              key={tab.id}
              className="absolute inset-0 flex flex-col"
              style={{
                visibility: isActive ? "visible" : "hidden",
                pointerEvents: isActive ? "auto" : "none",
              }}
            >
              <SessionToolbar tabId={tab.id} />
              <div className="flex-1 min-h-0 overflow-hidden flex">
                <div className="flex-1 min-w-0 overflow-hidden">
                  {data && (
                    <SplitPane
                      node={data.splitTree}
                      tabId={tab.id}
                      isTabActive={isActive}
                      activePaneId={data.activePaneId}
                      showFocusRing={data.splitTree.type === "split"}
                      path={[]}
                    />
                  )}
                </div>
                {data?.activePaneId && (
                  <FileManagerPanel
                    tabId={tab.id}
                    sessionId={data.activePaneId}
                    isOpen={!!fileManagerOpenTabs[tab.id]}
                    width={fileManagerWidth}
                    onWidthChange={setFileManagerWidth}
                  />
                )}
              </div>
              <TerminalToolbar tabId={tab.id} />
            </div>
          );
        })}

        {/* AI tabs: visibility-based */}
        {aiTabs.map((tab) => {
          const isActive = activeTabId === tab.id;
          return (
            <div
              key={tab.id}
              className="absolute inset-0 bg-background"
              style={{
                visibility: isActive ? "visible" : "hidden",
                pointerEvents: isActive ? "auto" : "none",
              }}
            >
              <AIChatContent tabId={tab.id} />
            </div>
          );
        })}

        {/* Query tabs: visibility-based */}
        {queryTabs.map((tab) => {
          const isActive = activeTabId === tab.id;
          const meta = tab.meta as QueryTabMeta;
          return (
            <div
              key={tab.id}
              className="absolute inset-0 bg-background"
              style={{
                visibility: isActive ? "visible" : "hidden",
                pointerEvents: isActive ? "auto" : "none",
              }}
            >
              {meta.assetType === "database" ? <DatabasePanel tabId={tab.id} /> : <RedisPanel tabId={tab.id} />}
            </div>
          );
        })}

        {/* Page and info tabs: rendered only when active */}
        {activeTab && (activeTab.type === "page" || activeTab.type === "info") && (
          <div className="absolute inset-0 bg-background">{renderActiveContent()}</div>
        )}

        {/* Welcome screen when no active tab */}
        {!activeTab && (
          <div className="absolute inset-0 flex items-center justify-center bg-gradient-to-br from-background via-background to-primary/5">
            <div className="text-center space-y-4">
              <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-primary/10">
                <img src={logoLight} alt="opskat" className="h-10 w-10 rounded-lg dark:hidden" />
                <img src={logoDark} alt="opskat" className="h-10 w-10 rounded-lg hidden dark:block" />
              </div>
              <div className="space-y-1">
                <h2 className="text-2xl font-semibold tracking-tight">{t("app.title")}</h2>
                <p className="text-sm text-muted-foreground">{t("app.subtitle")}</p>
              </div>
              <p className="text-xs text-muted-foreground/60">{t("app.hint")}</p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
