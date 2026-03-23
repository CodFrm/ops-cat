import { useTranslation } from "react-i18next";
import { X, TerminalSquare, Cat } from "lucide-react";
import { useFullscreen } from "@/hooks/useFullscreen";
import { AssetDetail } from "@/components/asset/AssetDetail";
import { GroupDetail } from "@/components/asset/GroupDetail";
import { SplitPane } from "@/components/terminal/SplitPane";
import { TerminalToolbar } from "@/components/terminal/TerminalToolbar";
import { SettingsPage } from "@/components/settings/SettingsPage";
import { SSHKeyManager } from "@/components/settings/SSHKeyManager";
import { useTerminalStore } from "@/stores/terminalStore";
import { cn } from "@/lib/utils";
import { asset_entity, group_entity } from "../../../wailsjs/go/models";

interface MainPanelProps {
  activePage: string;
  selectedAsset: asset_entity.Asset | null;
  selectedGroup: group_entity.Group | null;
  onEditAsset: (asset: asset_entity.Asset) => void;
  onDeleteAsset: (id: number) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
}

export function MainPanel({
  activePage,
  selectedAsset,
  selectedGroup,
  onEditAsset,
  onDeleteAsset,
  onConnectAsset,
}: MainPanelProps) {
  const { t } = useTranslation();
  const isFullscreen = useFullscreen();
  const { tabs, activeTabId, assetInfoOpen, setActiveTab, removeTab, closeAssetInfo, openAssetInfo, connectingAssetIds } = useTerminalStore();

  const dragRegion = (
    <div
      className={`${isFullscreen ? "h-2" : "h-10"} w-full shrink-0`}
      style={{ "--wails-draggable": "drag" } as React.CSSProperties}
    />
  );

  const isHome = activePage === "home";
  const showTerminal = isHome && activeTabId && tabs.some((tab) => tab.id === activeTabId);
  const showAssetInfo = isHome && !showTerminal && assetInfoOpen && selectedAsset;
  const showGroupInfo = isHome && !showTerminal && !showAssetInfo && assetInfoOpen && selectedGroup;

  return (
    <div className="flex flex-1 flex-col min-w-0">
      {/* Drag region for frameless window */}
      {dragRegion}

      {/* Tab bar — always rendered so terminals stay mounted */}
      {(assetInfoOpen || tabs.length > 0) && (
        <div
          className="flex items-center border-b overflow-x-auto bg-background"
          style={{ display: isHome ? undefined : "none" }}
        >
          {assetInfoOpen && selectedGroup && !selectedAsset && (
            <div
              className={cn(
                "relative flex items-center gap-1.5 px-3 py-2 text-sm shrink-0 cursor-pointer transition-colors duration-150",
                showGroupInfo
                  ? "text-foreground"
                  : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
              )}
              onClick={() => openAssetInfo()}
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
              onClick={() => openAssetInfo()}
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
          {tabs.map((tab) => {
            const allDisconnected = Object.values(tab.panes).every(
              (p) => !p.connected
            );
            return (
              <div
                key={tab.id}
                className={cn(
                  "relative flex items-center gap-1.5 px-3 py-2 text-sm shrink-0 cursor-pointer transition-colors duration-150",
                  activeTabId === tab.id
                    ? "text-foreground"
                    : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
                )}
                onClick={() => setActiveTab(tab.id)}
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
                {activeTabId === tab.id && (
                  <span className="absolute bottom-0 left-1 right-1 h-0.5 rounded-full bg-primary" />
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* Content area — all pages overlap here, terminal always mounted to avoid xterm flash */}
      <div className="flex-1 relative min-h-0 overflow-hidden">
        {/* Home: terminal content — use visibility instead of display to preserve xterm layout */}
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
                <div className="flex-1 min-h-0 overflow-hidden">
                  <SplitPane
                    node={tab.splitTree}
                    tabId={tab.id}
                    isTabActive={isActive}
                    activePaneId={tab.activePaneId}
                    showFocusRing={tab.splitTree.type === "split"}
                    path={[]}
                  />
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

        {/* Settings page */}
        {activePage === "settings" && (
          <div className="absolute inset-0 bg-background">
            <SettingsPage />
          </div>
        )}

        {/* SSH Keys page */}
        {activePage === "sshkeys" && (
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
      </div>
    </div>
  );
}
