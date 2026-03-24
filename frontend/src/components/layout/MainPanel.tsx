import { useTranslation } from "react-i18next";
import { X, TerminalSquare, Cat, Settings, KeyRound, MessageSquare, ScrollText, ArrowRightLeft } from "lucide-react";
import { useFullscreen } from "@/hooks/useFullscreen";
import { AssetDetail } from "@/components/asset/AssetDetail";
import { GroupDetail } from "@/components/asset/GroupDetail";
import { SplitPane } from "@/components/terminal/SplitPane";
import { SessionToolbar } from "@/components/terminal/SessionToolbar";
import { TerminalToolbar } from "@/components/terminal/TerminalToolbar";
import { FileManagerPanel } from "@/components/terminal/FileManagerPanel";
import { SettingsPage } from "@/components/settings/SettingsPage";
import { SSHKeyManager } from "@/components/settings/SSHKeyManager";
import { AuditLogPage } from "@/components/audit/AuditLogPage";
import { PortForwardPage } from "@/components/forward/PortForwardPage";
import { AIChatContent } from "@/components/ai/AIChatContent";
import { useTerminalStore } from "@/stores/terminalStore";
import { useAIStore } from "@/stores/aiStore";
import { useSFTPStore } from "@/stores/sftpStore";
import { cn } from "@/lib/utils";
import { asset_entity, group_entity } from "../../../wailsjs/go/models";

const AI_TAB_PREFIX = "ai:";

const pageTabMeta: Record<string, { icon: typeof Settings; labelKey: string }> = {
  settings: { icon: Settings, labelKey: "nav.settings" },
  forward: { icon: ArrowRightLeft, labelKey: "nav.forward" },
  sshkeys: { icon: KeyRound, labelKey: "nav.sshKeys" },
  audit: { icon: ScrollText, labelKey: "nav.audit" },
};

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
  const { tabs, activeTabId, assetInfoOpen, setActiveTab, removeTab, closeAssetInfo, openAssetInfo, connectingAssetIds } = useTerminalStore();
  const { fileManagerOpenTabs, fileManagerWidth, setFileManagerWidth } = useSFTPStore();
  const aiOpenTabs = useAIStore((s) => s.openTabs);

  const noTabStyle = { "--wails-draggable": "no-drag" } as React.CSSProperties;

  const isHome = !activePageTab;
  const isAITab = activePageTab?.startsWith(AI_TAB_PREFIX) || false;
  const activeAITabId = isAITab ? activePageTab!.slice(AI_TAB_PREFIX.length) : null;

  const showTerminal = isHome && activeTabId && tabs.some((tab) => tab.id === activeTabId);
  const showAssetInfo = isHome && !showTerminal && assetInfoOpen && selectedAsset;
  const showGroupInfo = isHome && !showTerminal && !showAssetInfo && assetInfoOpen && selectedGroup;
  const hasTabs = assetInfoOpen || tabs.length > 0 || aiOpenTabs.length > 0 || openPageTabs.length > 0;

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
        <div
          className={`flex items-center border-b overflow-x-auto bg-background ${isFullscreen ? "pt-2" : "pt-10"}`}
          style={{ "--wails-draggable": "drag" } as React.CSSProperties}
        >
          {/* Asset info tabs */}
          {assetInfoOpen && selectedGroup && !selectedAsset && (
            <div
              className={cn(
                "relative flex items-center gap-1.5 px-3 py-2 text-sm shrink-0 cursor-pointer transition-colors duration-150",
                showGroupInfo
                  ? "text-foreground"
                  : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
              )}
              style={noTabStyle}
              onClick={() => { openAssetInfo(); onTerminalTabClick(); }}
            >
              {selectedGroup.Name}
              <button
                className="ml-1.5 rounded-sm p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors duration-150"
                onClick={(e) => {
                  e.stopPropagation();
                  closeAssetInfo();
                }}
              >
                <X className="h-3 w-3" />
              </button>
              {showGroupInfo && (
                <span className="absolute bottom-0 left-1 right-1 h-0.5 rounded-full bg-primary" />
              )}
            </div>
          )}
          {assetInfoOpen && selectedAsset && (
            <div
              className={cn(
                "relative flex items-center gap-1.5 px-3 py-2 text-sm shrink-0 cursor-pointer transition-colors duration-150",
                showAssetInfo
                  ? "text-foreground"
                  : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
              )}
              style={noTabStyle}
              onClick={() => { openAssetInfo(); onTerminalTabClick(); }}
            >
              {selectedAsset.Name}
              <button
                className="ml-1.5 rounded-sm p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors duration-150"
                onClick={(e) => {
                  e.stopPropagation();
                  closeAssetInfo();
                }}
              >
                <X className="h-3 w-3" />
              </button>
              {showAssetInfo && (
                <span className="absolute bottom-0 left-1 right-1 h-0.5 rounded-full bg-primary" />
              )}
            </div>
          )}

          {/* Terminal tabs */}
          {tabs.map((tab) => {
            const paneValues = Object.values(tab.panes);
            const allDisconnected = paneValues.length > 0 && paneValues.every(
              (p) => !p.connected
            );
            const isActive = isHome && activeTabId === tab.id;
            return (
              <div
                key={tab.id}
                className={cn(
                  "relative flex items-center gap-1.5 px-3 py-2 text-sm shrink-0 cursor-pointer transition-colors duration-150",
                  isActive
                    ? "text-foreground"
                    : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
                )}
                style={noTabStyle}
                onClick={() => { setActiveTab(tab.id); onTerminalTabClick(); }}
              >
                <TerminalSquare className="h-3.5 w-3.5" />
                <span className="max-w-24 truncate">{tab.assetName}</span>
                {allDisconnected && (
                  <span className="h-1.5 w-1.5 rounded-full bg-destructive shrink-0" />
                )}
                <button
                  className="ml-1.5 rounded-sm p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors duration-150"
                  onClick={(e) => {
                    e.stopPropagation();
                    removeTab(tab.id);
                  }}
                >
                  <X className="h-3 w-3" />
                </button>
                {isActive && (
                  <span className="absolute bottom-0 left-1 right-1 h-0.5 rounded-full bg-primary" />
                )}
              </div>
            );
          })}

          {/* AI conversation tabs */}
          {aiOpenTabs.map((aiTab) => {
            const pageTabId = AI_TAB_PREFIX + aiTab.id;
            const isActive = activePageTab === pageTabId;
            return (
              <div
                key={aiTab.id}
                className={cn(
                  "relative flex items-center gap-1.5 px-3 py-2 text-sm shrink-0 cursor-pointer transition-colors duration-150",
                  isActive
                    ? "text-foreground"
                    : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
                )}
                style={noTabStyle}
                onClick={() => onActivatePageTab(pageTabId)}
              >
                <MessageSquare className="h-3.5 w-3.5" />
                <span className="max-w-24 truncate">{aiTab.title}</span>
                <button
                  className="ml-1.5 rounded-sm p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors duration-150"
                  onClick={(e) => {
                    e.stopPropagation();
                    onClosePageTab(pageTabId);
                  }}
                >
                  <X className="h-3 w-3" />
                </button>
                {isActive && (
                  <span className="absolute bottom-0 left-1 right-1 h-0.5 rounded-full bg-primary" />
                )}
              </div>
            );
          })}

          {/* Page tabs (settings, sshkeys) */}
          {openPageTabs.map((pageId) => {
            const meta = pageTabMeta[pageId];
            if (!meta) return null;
            const Icon = meta.icon;
            return (
              <div
                key={pageId}
                className={cn(
                  "relative flex items-center gap-1.5 px-3 py-2 text-sm shrink-0 cursor-pointer transition-colors duration-150",
                  activePageTab === pageId
                    ? "text-foreground"
                    : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
                )}
                style={noTabStyle}
                onClick={() => onActivatePageTab(pageId)}
              >
                <Icon className="h-3.5 w-3.5" />
                <span>{t(meta.labelKey)}</span>
                <button
                  className="ml-1.5 rounded-sm p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors duration-150"
                  onClick={(e) => {
                    e.stopPropagation();
                    onClosePageTab(pageId);
                  }}
                >
                  <X className="h-3 w-3" />
                </button>
                {activePageTab === pageId && (
                  <span className="absolute bottom-0 left-1 right-1 h-0.5 rounded-full bg-primary" />
                )}
              </div>
            );
          })}
        </div>
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
                  {fileManagerOpenTabs[tab.id] && tab.activePaneId && (
                    <FileManagerPanel
                      tabId={tab.id}
                      sessionId={tab.activePaneId}
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

          {!showTerminal && !showAssetInfo && !showGroupInfo && (
            <div className="flex items-center justify-center h-full bg-gradient-to-br from-background via-background to-primary/5">
              <div className="text-center space-y-4">
                <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-primary/10">
                  <Cat className="h-8 w-8 text-primary" />
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
                <SSHKeyManager />
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
