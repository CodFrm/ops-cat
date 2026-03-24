import { createContext, useContext, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { X, Settings, KeyRound, MessageSquare, ScrollText, ArrowRightLeft, Server, Folder } from "lucide-react";
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
import { useAIStore } from "@/stores/aiStore";
import { useQueryStore } from "@/stores/queryStore";
import { useSFTPStore } from "@/stores/sftpStore";
import { cn } from "@/lib/utils";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";
import { asset_entity, group_entity } from "../../../wailsjs/go/models";

const AI_TAB_PREFIX = "ai:";
const QUERY_TAB_PREFIX = "query:";

interface TabBarEntry {
  key: string;
  close: () => void;
}

interface TabBarContextValue {
  entries: TabBarEntry[];
  dragKeyRef: React.RefObject<string | null>;
  reorder: (fromKey: string, toKey: string) => void;
  moveTo: (key: string, toIndex: number) => void;
}

const TabBarContext = createContext<TabBarContextValue>({
  entries: [],
  dragKeyRef: { current: null },
  reorder: () => {},
  moveTo: () => {},
});

const pageTabMeta: Record<string, { icon: typeof Settings; labelKey: string }> = {
  settings: { icon: Settings, labelKey: "nav.settings" },
  forward: { icon: ArrowRightLeft, labelKey: "nav.forward" },
  sshkeys: { icon: KeyRound, labelKey: "nav.sshKeys" },
  audit: { icon: ScrollText, labelKey: "nav.audit" },
};

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

function TabItem({
  tabKey, icon: Icon, iconStyle, label, title,
  isActive, onClick, onClose, extra,
}: TabItemProps) {
  const { t } = useTranslation();
  const { entries, dragKeyRef, reorder, moveTo } = useContext(TabBarContext);
  const noTabStyle = { "--wails-draggable": "no-drag" } as React.CSSProperties;
  const globalIndex = entries.findIndex((e) => e.key === tabKey);
  const total = entries.length;

  return (
    <ContextMenu>
      <ContextMenuTrigger>
        <div
          className={cn(
            "relative flex items-center gap-1.5 px-3 py-2 text-sm shrink-0 cursor-pointer transition-colors duration-150",
            isActive
              ? "text-foreground"
              : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
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
          onDragEnd={() => { dragKeyRef.current = null; }}
        >
          <Icon className="h-3.5 w-3.5 shrink-0" style={iconStyle} />
          <span className="max-w-24 truncate">{label}</span>
          {extra}
          <button
            className="ml-1.5 rounded-sm p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors duration-150"
            onClick={(e) => { e.stopPropagation(); onClose(); }}
          >
            <X className="h-3 w-3" />
          </button>
          {isActive && (
            <span className="absolute bottom-0 left-1 right-1 h-0.5 rounded-full bg-primary" />
          )}
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuItem onClick={onClose}>
          {t("tab.close")}
        </ContextMenuItem>
        <ContextMenuItem
          onClick={() => entries.forEach((e, i) => { if (i !== globalIndex) e.close(); })}
          disabled={total <= 1}
        >
          {t("tab.closeOthers")}
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem
          onClick={() => entries.slice(0, globalIndex).forEach((e) => e.close())}
          disabled={globalIndex <= 0}
        >
          {t("tab.closeLeft")}
        </ContextMenuItem>
        <ContextMenuItem
          onClick={() => entries.slice(globalIndex + 1).forEach((e) => e.close())}
          disabled={globalIndex >= total - 1}
        >
          {t("tab.closeRight")}
        </ContextMenuItem>
        {total > 1 && (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem
              onClick={() => moveTo(tabKey, globalIndex - 1)}
              disabled={globalIndex <= 0}
            >
              {t("tab.moveLeft")}
            </ContextMenuItem>
            <ContextMenuItem
              onClick={() => moveTo(tabKey, globalIndex + 1)}
              disabled={globalIndex >= total - 1}
            >
              {t("tab.moveRight")}
            </ContextMenuItem>
            <ContextMenuItem
              onClick={() => moveTo(tabKey, 0)}
              disabled={globalIndex <= 0}
            >
              {t("tab.moveToStart")}
            </ContextMenuItem>
            <ContextMenuItem
              onClick={() => moveTo(tabKey, total - 1)}
              disabled={globalIndex >= total - 1}
            >
              {t("tab.moveToEnd")}
            </ContextMenuItem>
          </>
        )}
      </ContextMenuContent>
    </ContextMenu>
  );
}

interface MainPanelProps {
  activePage?: string;
  selectedAsset: asset_entity.Asset | null;
  selectedGroup: group_entity.Group | null;
  onEditAsset: (asset: asset_entity.Asset) => void;
  onDeleteAsset: (id: number) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
  openPageTabs: string[];
  activePageTab: string | null;
  onActivatePageTab: (page: string) => void;
  onClosePageTab: (page: string) => void;
  onTerminalTabClick: () => void;
}

export function MainPanel({
  activePage: _activePage,
  selectedAsset,
  selectedGroup,
  onEditAsset,
  onDeleteAsset,
  onConnectAsset,
  openPageTabs,
  activePageTab,
  onActivatePageTab,
  onClosePageTab,
  onTerminalTabClick,
}: MainPanelProps) {
  const { t } = useTranslation();
  const isFullscreen = useFullscreen();
  const { tabs, activeTabId, assetInfoOpen, infoTabs, setActiveTab, removeTab, closeAssetInfo, openAssetInfo, closeInfoTab, connectingAssetIds } = useTerminalStore();
  const { assets, groups } = useAssetStore();
  const { fileManagerOpenTabs, fileManagerWidth, setFileManagerWidth } = useSFTPStore();
  const aiOpenTabs = useAIStore((s) => s.openTabs);
  const queryOpenTabs = useQueryStore((s) => s.openTabs);
  const closeQueryTab = useQueryStore((s) => s.closeQueryTab);

  const dragKeyRef = useRef<string | null>(null);
  const [tabOrder, setTabOrder] = useState<string[]>([]);

  const isHome = !activePageTab;
  const isAITab = activePageTab?.startsWith(AI_TAB_PREFIX) || false;
  const activeAITabId = isAITab ? activePageTab!.slice(AI_TAB_PREFIX.length) : null;
  const isQueryTab = activePageTab?.startsWith(QUERY_TAB_PREFIX) || false;
  const activeQueryTabId = isQueryTab ? activePageTab! : null;

  const showTerminal = isHome && activeTabId && tabs.some((tab) => tab.id === activeTabId);
  const activeInfoTab = isHome && activeTabId ? infoTabs.find((t) => t.id === activeTabId) : null;
  const showAssetInfo = isHome && !showTerminal && !activeInfoTab && assetInfoOpen && selectedAsset;
  const showGroupInfo = isHome && !showTerminal && !activeInfoTab && !showAssetInfo && assetInfoOpen && selectedGroup;
  const hasTabs = assetInfoOpen || infoTabs.length > 0 || tabs.length > 0 || aiOpenTabs.length > 0 || queryOpenTabs.length > 0 || openPageTabs.length > 0;

  // Build tab entry map (key → close handler)
  const entryMap = new Map<string, TabBarEntry>();
  if (assetInfoOpen && (selectedGroup || selectedAsset)) {
    entryMap.set("asset-info", { key: "asset-info", close: closeAssetInfo });
  }
  infoTabs.forEach((it) => entryMap.set(`info:${it.id}`, { key: `info:${it.id}`, close: () => closeInfoTab(it.id) }));
  tabs.forEach((t) => entryMap.set(`term:${t.id}`, { key: `term:${t.id}`, close: () => removeTab(t.id) }));
  aiOpenTabs.forEach((t) => entryMap.set(`ai:${t.id}`, { key: `ai:${t.id}`, close: () => onClosePageTab(AI_TAB_PREFIX + t.id) }));
  queryOpenTabs.forEach((t) => entryMap.set(`query:${t.id}`, { key: `query:${t.id}`, close: () => { closeQueryTab(t.id); onClosePageTab(QUERY_TAB_PREFIX + t.assetId); } }));
  openPageTabs.forEach((id) => entryMap.set(`page:${id}`, { key: `page:${id}`, close: () => onClosePageTab(id) }));

  // Compute render order: preserve custom drag order, append new tabs at end
  const currentKeySet = new Set(entryMap.keys());
  const tabOrderSet = new Set(tabOrder);
  const renderOrder = [
    ...tabOrder.filter((k) => currentKeySet.has(k)),
    ...[...entryMap.keys()].filter((k) => !tabOrderSet.has(k)),
  ];
  const tabBarEntries = renderOrder.map((k) => entryMap.get(k)!);

  const reorderTab = (fromKey: string, toKey: string) => {
    const fromIdx = renderOrder.indexOf(fromKey);
    const toIdx = renderOrder.indexOf(toKey);
    if (fromIdx === -1 || toIdx === -1 || fromIdx === toIdx) return;
    const next = [...renderOrder];
    next.splice(fromIdx, 1);
    next.splice(toIdx, 0, fromKey);
    setTabOrder(next);
  };

  const moveTabTo = (key: string, toIdx: number) => {
    const fromIdx = renderOrder.indexOf(key);
    if (fromIdx === -1 || fromIdx === toIdx) return;
    const clamped = Math.max(0, Math.min(toIdx, renderOrder.length - 1));
    const next = [...renderOrder];
    next.splice(fromIdx, 1);
    next.splice(clamped, 0, key);
    setTabOrder(next);
  };

  const tabBarCtx: TabBarContextValue = { entries: tabBarEntries, dragKeyRef, reorder: reorderTab, moveTo: moveTabTo };

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
            className={`flex items-center border-b overflow-x-auto bg-background ${isFullscreen ? "pt-2" : "pt-10"}`}
            style={{ "--wails-draggable": "drag" } as React.CSSProperties}
          >
            {renderOrder.map((key) => {
              if (key === "asset-info") {
                if (selectedGroup && !selectedAsset) {
                  return (
                    <TabItem
                      key={key}
                      tabKey={key}
                      icon={selectedGroup.Icon ? getIconComponent(selectedGroup.Icon) : Folder}
                      iconStyle={selectedGroup.Icon ? { color: getIconColor(selectedGroup.Icon) } : undefined}
                      label={selectedGroup.Name}
                      isActive={!!showGroupInfo}
                      onClick={() => { openAssetInfo(); onTerminalTabClick(); }}
                      onClose={closeAssetInfo}
                    />
                  );
                }
                if (selectedAsset) {
                  return (
                    <TabItem
                      key={key}
                      tabKey={key}
                      icon={selectedAsset.Icon ? getIconComponent(selectedAsset.Icon) : Server}
                      iconStyle={selectedAsset.Icon ? { color: getIconColor(selectedAsset.Icon) } : undefined}
                      label={selectedAsset.Name}
                      isActive={!!showAssetInfo}
                      onClick={() => { openAssetInfo(); onTerminalTabClick(); }}
                      onClose={closeAssetInfo}
                    />
                  );
                }
                return null;
              }

              if (key.startsWith("info:")) {
                const infoTab = infoTabs.find((it) => `info:${it.id}` === key);
                if (!infoTab) return null;
                const InfoIcon = infoTab.icon ? getIconComponent(infoTab.icon) : (infoTab.type === 'group' ? Folder : Server);
                const infoIconStyle = infoTab.icon ? { color: getIconColor(infoTab.icon) } : undefined;
                return (
                  <TabItem
                    key={key}
                    tabKey={key}
                    icon={InfoIcon}
                    iconStyle={infoIconStyle}
                    label={infoTab.name}
                    isActive={isHome && activeTabId === infoTab.id}
                    onClick={() => { setActiveTab(infoTab.id); onTerminalTabClick(); }}
                    onClose={() => closeInfoTab(infoTab.id)}
                  />
                );
              }

              if (key.startsWith("term:")) {
                const tab = tabs.find((t) => `term:${t.id}` === key);
                if (!tab) return null;
                const paneValues = Object.values(tab.panes);
                const allDisconnected = paneValues.length > 0 && paneValues.every((p) => !p.connected);
                return (
                  <TabItem
                    key={key}
                    tabKey={key}
                    icon={tab.assetIcon ? getIconComponent(tab.assetIcon) : Server}
                    iconStyle={tab.assetIcon ? { color: getIconColor(tab.assetIcon) } : undefined}
                    label={tab.assetName}
                    isActive={isHome && activeTabId === tab.id}
                    onClick={() => { setActiveTab(tab.id); onTerminalTabClick(); }}
                    onClose={() => removeTab(tab.id)}
                    extra={allDisconnected ? <span className="h-1.5 w-1.5 rounded-full bg-destructive shrink-0" /> : undefined}
                  />
                );
              }

              if (key.startsWith("ai:")) {
                const aiTab = aiOpenTabs.find((t) => `ai:${t.id}` === key);
                if (!aiTab) return null;
                const pageTabId = AI_TAB_PREFIX + aiTab.id;
                return (
                  <TabItem
                    key={key}
                    tabKey={key}
                    icon={MessageSquare}
                    label={aiTab.title}
                    isActive={activePageTab === pageTabId}
                    onClick={() => onActivatePageTab(pageTabId)}
                    onClose={() => onClosePageTab(pageTabId)}
                  />
                );
              }

              if (key.startsWith("query:")) {
                const qTab = queryOpenTabs.find((t) => `query:${t.id}` === key);
                if (!qTab) return null;
                const qPageTabId = QUERY_TAB_PREFIX + qTab.assetId;
                return (
                  <TabItem
                    key={key}
                    tabKey={key}
                    icon={qTab.assetIcon ? getIconComponent(qTab.assetIcon) : Server}
                    iconStyle={qTab.assetIcon ? { color: getIconColor(qTab.assetIcon) } : undefined}
                    label={qTab.assetName}
                    isActive={activePageTab === qPageTabId}
                    onClick={() => onActivatePageTab(qPageTabId)}
                    onClose={() => { closeQueryTab(qTab.id); onClosePageTab(qPageTabId); }}
                  />
                );
              }

              if (key.startsWith("page:")) {
                const pageId = key.slice(5);
                const meta = pageTabMeta[pageId];
                if (!meta) return null;
                return (
                  <TabItem
                    key={key}
                    tabKey={key}
                    icon={meta.icon}
                    label={t(meta.labelKey)}
                    isActive={activePageTab === pageId}
                    onClick={() => onActivatePageTab(pageId)}
                    onClose={() => onClosePageTab(pageId)}
                  />
                );
              }

              return null;
            })}
          </div>
        </TabBarContext.Provider>
      )}

      {/* Content area */}
      <div className="flex-1 relative min-h-0 overflow-hidden">
        {/* Home: terminal content — use visibility to preserve xterm layout */}
        <div
          className="absolute inset-0"
          style={{
            visibility: isHome ? "visible" : "hidden",
            pointerEvents: isHome ? "auto" : "none",
          }}
        >
          {tabs.map((tab) => {
            const isActive = isHome && activeTabId === tab.id;
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
                    <SplitPane
                      node={tab.splitTree}
                      tabId={tab.id}
                      isTabActive={isActive}
                      activePaneId={tab.activePaneId}
                      showFocusRing={tab.splitTree.type === "split"}
                      path={[]}
                    />
                  </div>
                  {tab.activePaneId && (
                    <FileManagerPanel
                      tabId={tab.id}
                      sessionId={tab.activePaneId}
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

          {showAssetInfo && (
            <AssetDetail
              asset={selectedAsset}
              isConnecting={connectingAssetIds.has(selectedAsset.ID)}
              onEdit={() => onEditAsset(selectedAsset)}
              onDelete={() => onDeleteAsset(selectedAsset.ID)}
              onConnect={() => onConnectAsset(selectedAsset)}
            />
          )}

          {showGroupInfo && (
            <GroupDetail group={selectedGroup!} />
          )}

          {activeInfoTab && (() => {
            if (activeInfoTab.type === 'asset') {
              const asset = assets.find((a) => a.ID === activeInfoTab.targetId);
              if (!asset) return null;
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
              const group = groups.find((g) => g.ID === activeInfoTab.targetId);
              if (!group) return null;
              return <GroupDetail group={group} />;
            }
          })()}

          {!showTerminal && !showAssetInfo && !showGroupInfo && !activeInfoTab && (
            <div className="flex items-center justify-center h-full bg-gradient-to-br from-background via-background to-primary/5">
              <div className="text-center space-y-4">
                <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-primary/10">
                  <img src={logoLight} alt="ops-cat" className="h-10 w-10 rounded-lg dark:hidden" />
                  <img src={logoDark} alt="ops-cat" className="h-10 w-10 rounded-lg hidden dark:block" />
                </div>
                <div className="space-y-1">
                  <h2 className="text-2xl font-semibold tracking-tight">
                    {t("app.title")}
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    {t("app.subtitle")}
                  </p>
                </div>
                <p className="text-xs text-muted-foreground/60">
                  {t("app.hint")}
                </p>
              </div>
            </div>
          )}
        </div>

        {/* AI conversation tabs content */}
        {aiOpenTabs.map((aiTab) => {
          const isActive = activeAITabId === aiTab.id;
          return (
            <div
              key={aiTab.id}
              className="absolute inset-0 bg-background"
              style={{
                visibility: isActive ? "visible" : "hidden",
                pointerEvents: isActive ? "auto" : "none",
              }}
            >
              <AIChatContent tabId={aiTab.id} />
            </div>
          );
        })}

        {/* Query panel tabs content */}
        {queryOpenTabs.map((qTab) => {
          const qPageTabId = QUERY_TAB_PREFIX + qTab.assetId;
          const isActive = activeQueryTabId === qPageTabId;
          return (
            <div
              key={qTab.id}
              className="absolute inset-0 bg-background"
              style={{
                visibility: isActive ? "visible" : "hidden",
                pointerEvents: isActive ? "auto" : "none",
              }}
            >
              {qTab.assetType === "database" ? (
                <DatabasePanel tabId={qTab.id} />
              ) : (
                <RedisPanel tabId={qTab.id} />
              )}
            </div>
          );
        })}

        {/* Page tabs content */}
        {activePageTab === "settings" && (
          <div className="absolute inset-0 bg-background">
            <SettingsPage />
          </div>
        )}
        {activePageTab === "sshkeys" && (
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
        )}
        {activePageTab === "audit" && (
          <div className="absolute inset-0 bg-background">
            <AuditLogPage />
          </div>
        )}
        {activePageTab === "forward" && (
          <div className="absolute inset-0 bg-background">
            <PortForwardPage />
          </div>
        )}
      </div>
    </div>
  );
}
