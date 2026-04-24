import { useTranslation } from "react-i18next";
import { Loader2 } from "lucide-react";
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
import { MongoDBPanel } from "@/components/query/MongoDBPanel";
import { useTerminalStore } from "@/stores/terminalStore";
import { useAssetStore } from "@/stores/assetStore";
import { useTabStore, type QueryTabMeta, type PageTabMeta, type InfoTabMeta } from "@/stores/tabStore";
import { useSFTPStore } from "@/stores/sftpStore";
import { useShortcutStore, formatBinding, type ShortcutAction } from "@/stores/shortcutStore";
import { asset_entity } from "../../../wailsjs/go/models";
import { ExtensionPage } from "@/extension";
import { TopTabBar } from "./TopTabBar";
import { useLayoutStore } from "@/stores/layoutStore";

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

  const tabData = useTerminalStore((s) => s.tabData);
  const connectingAssetIds = useTerminalStore((s) => s.connectingAssetIds);

  const { assets, groups, initialized } = useAssetStore();
  const { fileManagerOpenTabs, fileManagerWidth, setFileManagerWidth } = useSFTPStore();

  const tabBarLayout = useLayoutStore((s) => s.tabBarLayout);
  const shortcuts = useShortcutStore((s) => s.shortcuts);

  const openSettingsTab = () => {
    const tabStore = useTabStore.getState();
    const existing = tabStore.tabs.find((tab) => tab.id === "settings");
    if (existing) {
      tabStore.activateTab("settings");
    } else {
      tabStore.openTab({
        id: "settings",
        type: "page",
        label: t("nav.settings"),
        meta: { type: "page", pageId: "settings" },
      });
    }
  };

  const SHORTCUT_HINTS: ReadonlyArray<readonly [ShortcutAction, string]> = [
    ["panel.ai", "panelAi"],
    ["panel.filter", "panelFilter"],
    ["panel.sidebar", "panelSidebar"],
    ["page.settings", "pageSettings"],
  ] as const;

  const activeTab = tabs.find((tab) => tab.id === activeTabId) ?? null;
  const hasTabs = tabs.length > 0;

  const terminalTabs = tabs.filter((tab) => tab.type === "terminal");
  const aiTabs = tabs.filter((tab) => tab.type === "ai");
  const queryTabs = tabs.filter((tab) => tab.type === "query");

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
      {/* When no tabs, show standalone drag region; also always shown in left layout (TopTabBar absent) */}
      {(!hasTabs || tabBarLayout === "left") && (
        <div
          className={`${isFullscreen ? "h-0" : "h-8"} w-full shrink-0`}
          style={{ "--wails-draggable": "drag" } as React.CSSProperties}
        />
      )}

      {/* Tab bar with integrated drag region (top layout only) */}
      {hasTabs && tabBarLayout === "top" && <TopTabBar />}

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

        {activeTab && activeTab.type === "page" && (
          <div className="absolute inset-0 bg-background">{renderActiveContent()}</div>
        )}
        {/* Query tabs: display-based — sticky thead would leak as a composited
            layer if the parent only toggled visibility. State is in zustand,
            so display:none is safe here. */}
        {queryTabs.map((tab) => {
          const isActive = activeTabId === tab.id;
          const meta = tab.meta as QueryTabMeta;
          return (
            <div
              key={tab.id}
              className="absolute inset-0 bg-background"
              style={{ display: isActive ? "block" : "none" }}
            >
              {meta.assetType === "database" ? (
                <DatabasePanel tabId={tab.id} />
              ) : meta.assetType === "redis" ? (
                <RedisPanel tabId={tab.id} />
              ) : (
                <MongoDBPanel tabId={tab.id} />
              )}
            </div>
          );
        })}

        {/* Page and info tabs: rendered only when active */}
        {activeTab && activeTab.type === "info" && (
          <div className="absolute inset-0 bg-background">{renderActiveContent()}</div>
        )}

        {/* Welcome screen when no active tab */}
        {!activeTab && (
          <div className="absolute inset-0 flex items-center justify-center overflow-y-auto bg-gradient-to-br from-background via-background to-primary/5 p-6">
            <div className="text-center space-y-5">
              <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-primary/10">
                <img src={logoLight} alt="opskat" className="h-10 w-10 rounded-lg dark:hidden" />
                <img src={logoDark} alt="opskat" className="h-10 w-10 rounded-lg hidden dark:block" />
              </div>
              <div className="space-y-1">
                <h2 className="text-2xl font-semibold tracking-tight">{t("app.title")}</h2>
                <p className="text-sm text-muted-foreground">{t("app.subtitle")}</p>
              </div>
              <p className="text-xs text-muted-foreground">{t("app.hint")}</p>

              <div className="mx-auto w-fit rounded-lg border border-border/60 bg-muted/30 px-5 py-4 text-left text-xs text-muted-foreground/80">
                <ul className="flex flex-col gap-2">
                  {(["doubleClick", "click", "rightClick"] as const).map((k) => (
                    <li key={k} className="flex items-center gap-2">
                      <kbd className="inline-flex min-w-[52px] items-center justify-center rounded border border-border bg-background px-1.5 py-0.5 font-mono text-[10px] text-foreground/70">
                        {t(`app.hints.${k}Key`)}
                      </kbd>
                      <span>{t(`app.hints.${k}`)}</span>
                    </li>
                  ))}
                </ul>
                <div className="my-3 h-px w-full bg-border/70" />
                <div className="mb-2 text-[10px] uppercase tracking-wider text-muted-foreground/60">
                  {t("app.shortcuts.title")}
                </div>
                <ul className="flex flex-col gap-2">
                  {SHORTCUT_HINTS.map(([action, key]) => (
                    <li key={action} className="flex items-center gap-2">
                      <kbd className="inline-flex min-w-[52px] items-center justify-center rounded border border-border bg-background px-1.5 py-0.5 font-mono text-[10px] text-foreground/70">
                        {formatBinding(shortcuts[action])}
                      </kbd>
                      <span>{t(`app.shortcuts.${key}`)}</span>
                    </li>
                  ))}
                </ul>
              </div>

              <button
                type="button"
                onClick={openSettingsTab}
                className="text-xs text-muted-foreground/70 transition-colors hover:text-foreground"
              >
                {t("app.shortcuts.all")} →
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
