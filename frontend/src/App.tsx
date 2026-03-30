import { useState, useCallback, useRef, useEffect } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { ThemeProvider } from "@/components/theme-provider";
import { TooltipProvider, Toaster } from "@opskat/ui";
import { Sidebar } from "@/components/layout/Sidebar";
import { AssetTree } from "@/components/layout/AssetTree";
import { MainPanel } from "@/components/layout/MainPanel";
import { ConversationListPanel } from "@/components/ai/ConversationListPanel";
import { WindowControls } from "@/components/layout/WindowControls";
import { EdgeRevealStrip } from "@/components/layout/EdgeRevealStrip";
import { AssetForm } from "@/components/asset/AssetForm";
import { GroupDialog } from "@/components/asset/GroupDialog";
import { PermissionDialog } from "@/components/ai/PermissionDialog";
import { OpsctlApprovalDialog } from "@/components/approval/OpsctlApprovalDialog";
import { ErrorBoundary } from "@/components/ErrorBoundary";

import { useAssetStore } from "@/stores/assetStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore, type InfoTabMeta } from "@/stores/tabStore";
import { useExtensionStore, initExtensions } from "@/extension";
import { useKeyboardShortcuts } from "@/hooks/useKeyboardShortcuts";
import { asset_entity, group_entity } from "../wailsjs/go/models";
import { EventsOn, WindowToggleMaximise } from "../wailsjs/runtime/runtime";

function App() {
  const { t } = useTranslation();

  // 监听外部数据变更（opsctl 等），自动刷新 UI
  useEffect(() => {
    const cancel = EventsOn("data:changed", () => {
      useAssetStore.getState().refresh();
    });
    return () => {
      cancel();
    };
  }, []);

  // 监听自动更新检查结果
  useEffect(() => {
    const cancel = EventsOn("update:available", (info: { latestVersion: string }) => {
      toast.info(t("appUpdate.autoUpdateFound", { version: info.latestVersion }));
    });
    return () => {
      cancel();
    };
  }, [t]);

  // 监听系统启动状态
  useEffect(() => {
    const cancel = EventsOn("system:status", (entries: Array<{ level: string }>) => {
      if (!entries || entries.length === 0) return;
      const hasError = entries.some((e) => e.level === "error");
      const message = hasError ? t("systemStatus.toastError") : t("systemStatus.toastWarn");
      const toastFn = hasError ? toast.error : toast.warning;
      toastFn(message, {
        action: {
          label: t("systemStatus.showDetail"),
          onClick: () => {
            const tabStore = useTabStore.getState();
            const existing = tabStore.tabs.find((tab) => tab.id === "settings");
            if (existing) {
              tabStore.activateTab("settings");
            } else {
              tabStore.openTab({
                id: "settings",
                type: "page",
                label: "settings",
                meta: { type: "page", pageId: "settings" },
              });
            }
          },
        },
      });
    });
    return () => cancel();
  }, [t]);

  // Bootstrap extension system
  useEffect(() => {
    initExtensions().catch(console.error);
  }, []);

  // 双击拖拽区域最大化/还原窗口
  useEffect(() => {
    const handleDblClick = (e: MouseEvent) => {
      let el = e.target as HTMLElement | null;
      while (el) {
        const drag = getComputedStyle(el).getPropertyValue("--wails-draggable").trim();
        if (drag === "no-drag") return;
        if (drag === "drag") {
          WindowToggleMaximise();
          return;
        }
        el = el.parentElement;
      }
    };
    window.addEventListener("dblclick", handleDblClick);
    return () => window.removeEventListener("dblclick", handleDblClick);
  }, []);

  const [sidebarHidden, setSidebarHidden] = useState(() => localStorage.getItem("sidebar_hidden") === "true");
  const [assetTreeCollapsed, setAssetTreeCollapsed] = useState(
    () => localStorage.getItem("sidebar_collapsed") === "true"
  );
  const [aiPanelCollapsed, setAiPanelCollapsed] = useState(() => localStorage.getItem("ai_panel_collapsed") === "true");
  const [assetTreeWidth, setAssetTreeWidth] = useState(() => {
    const saved = localStorage.getItem("asset_tree_width");
    return saved ? Math.max(160, Math.min(480, Number(saved))) : 224;
  });
  const [assetTreeResizing, setAssetTreeResizing] = useState(false);
  const assetTreeWidthRef = useRef(assetTreeWidth);

  const toggleAIPanel = useCallback(() => {
    setAiPanelCollapsed((prev) => {
      localStorage.setItem("ai_panel_collapsed", String(!prev));
      return !prev;
    });
  }, []);

  const toggleSidebar = useCallback(() => {
    setAssetTreeCollapsed((prev) => {
      localStorage.setItem("sidebar_collapsed", String(!prev));
      return !prev;
    });
  }, []);

  const toggleSidebarHidden = useCallback(() => {
    setSidebarHidden((prev) => {
      localStorage.setItem("sidebar_hidden", String(!prev));
      return !prev;
    });
  }, []);

  const handleAssetTreeResizeStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    setAssetTreeResizing(true);
    const startX = e.clientX;
    const startWidth = assetTreeWidthRef.current;

    const onMouseMove = (ev: MouseEvent) => {
      const newWidth = Math.max(160, Math.min(480, startWidth + ev.clientX - startX));
      assetTreeWidthRef.current = newWidth;
      setAssetTreeWidth(newWidth);
    };

    const onMouseUp = () => {
      setAssetTreeResizing(false);
      document.removeEventListener("mousemove", onMouseMove);
      document.removeEventListener("mouseup", onMouseUp);
      localStorage.setItem("asset_tree_width", String(assetTreeWidthRef.current));
    };

    document.addEventListener("mousemove", onMouseMove);
    document.addEventListener("mouseup", onMouseUp);
  }, []);

  useKeyboardShortcuts({
    onToggleAIPanel: toggleAIPanel,
    onToggleSidebar: toggleSidebar,
  });

  // 资产表单
  const [assetFormOpen, setAssetFormOpen] = useState(false);
  const [editingAsset, setEditingAsset] = useState<asset_entity.Asset | null>(null);
  const [defaultGroupId, setDefaultGroupId] = useState(0);

  // 分组对话框
  const [groupDialogOpen, setGroupDialogOpen] = useState(false);
  const [editingGroup, setEditingGroup] = useState<group_entity.Group | null>(null);

  const { selectAsset, selectGroup, deleteAsset, getAsset } = useAssetStore();
  const { connect } = useTerminalStore();

  const handleAddAsset = (groupId?: number) => {
    setEditingAsset(null);
    setDefaultGroupId(groupId ?? 0);
    setAssetFormOpen(true);
  };

  const handleEditAsset = (asset: asset_entity.Asset) => {
    setEditingAsset(asset);
    setAssetFormOpen(true);
  };

  const handleCopyAsset = async (asset: asset_entity.Asset) => {
    try {
      const fullAsset = await getAsset(asset.ID);
      const copied = new asset_entity.Asset({
        ...fullAsset,
        ID: 0,
        Name: `${fullAsset.Name} - 副本`,
      });
      setEditingAsset(copied);
      setAssetFormOpen(true);
    } catch (e) {
      toast.error(String(e));
    }
  };

  const handleSelectAsset = (asset: asset_entity.Asset) => {
    selectAsset(asset.ID);
    const tabStore = useTabStore.getState();
    const previewTabId = "info-preview";
    const existing = tabStore.tabs.find((t) => t.id === previewTabId);
    const meta: InfoTabMeta = {
      type: "info",
      targetType: "asset",
      targetId: asset.ID,
      name: asset.Name,
      icon: asset.Icon || undefined,
    };
    if (existing) {
      tabStore.updateTab(previewTabId, {
        label: asset.Name,
        icon: asset.Icon || undefined,
        meta,
      });
      tabStore.activateTab(previewTabId);
    } else {
      tabStore.openTab({
        id: previewTabId,
        type: "info",
        label: asset.Name,
        icon: asset.Icon || undefined,
        meta,
      });
    }
  };

  const handleOpenInfoTab = useCallback((type: "asset" | "group", id: number, name: string, icon?: string) => {
    const tabStore = useTabStore.getState();
    const infoTabId = `info-${type}-${id}`;
    const existing = tabStore.tabs.find((t) => t.id === infoTabId);
    if (existing) {
      tabStore.activateTab(infoTabId);
    } else {
      tabStore.openTab({
        id: infoTabId,
        type: "info",
        label: name,
        icon,
        meta: { type: "info", targetType: type, targetId: id, name, icon },
      });
    }
  }, []);

  const handleDeleteAsset = async (id: number) => {
    await deleteAsset(id);
  };

  const handleConnectAsset = async (asset: asset_entity.Asset) => {
    if (asset.Type === "database" || asset.Type === "redis") {
      useQueryStore.getState().openQueryTab(asset);
      return;
    }

    // Check if this is an extension asset type
    const ext = useExtensionStore.getState().getExtensionForAssetType(asset.Type);
    if (ext) {
      const connectPage = ext.manifest.frontend?.pages.find((p) => p.slot === "asset.connect");
      if (connectPage) {
        useTabStore.getState().openTab({
          id: `ext-${asset.ID}-${connectPage.id}`,
          type: "page",
          label: asset.Name,
          icon: ext.manifest.icon,
          meta: {
            type: "page",
            pageId: connectPage.id,
            extensionName: ext.name,
            assetId: asset.ID,
          },
        });
        return;
      }
    }

    if (asset.Type !== "ssh") return;
    try {
      await connect(asset);
    } catch (e) {
      toast.error(`${asset.Name}: ${String(e)}`);
    }
  };

  const handleConnectAssetInNewTab = async (asset: asset_entity.Asset) => {
    if (asset.Type !== "ssh") return;
    try {
      await connect(asset, "", true);
    } catch (e) {
      toast.error(`${asset.Name}: ${String(e)}`);
    }
  };

  // Sidebar page navigation
  const handlePageChange = useCallback((page: string) => {
    const tabStore = useTabStore.getState();
    if (page === "home") {
      // Activate first non-page tab, or deactivate
      const homeTab = tabStore.tabs.find((t) => t.type === "terminal" || t.type === "info");
      tabStore.activateTab(homeTab?.id || tabStore.tabs[0]?.id || "");
      return;
    }
    // Page tabs: settings, forward, sshkeys, audit
    const existing = tabStore.tabs.find((t) => t.id === page);
    if (existing) {
      tabStore.activateTab(page);
    } else {
      tabStore.openTab({
        id: page,
        type: "page",
        label: page,
        meta: { type: "page", pageId: page },
      });
    }
  }, []);

  const handleOpenConversation = useCallback((tabId: string) => {
    useTabStore.getState().activateTab(tabId);
  }, []);

  // Derive active page for sidebar highlighting
  const activeTab = useTabStore((s) => s.tabs.find((t) => t.id === s.activeTabId));
  const activePage = !activeTab
    ? "home"
    : activeTab.type === "page"
      ? activeTab.id
      : activeTab.type === "terminal" || activeTab.type === "info"
        ? "home"
        : "other";

  return (
    <ThemeProvider defaultTheme="system">
      <ErrorBoundary>
        <TooltipProvider>
          <div className="flex h-screen w-screen overflow-hidden bg-background">
            <WindowControls />
            {sidebarHidden && <EdgeRevealStrip onClick={toggleSidebarHidden} />}
            {!sidebarHidden && (
              <Sidebar
                activePage={activePage}
                onPageChange={handlePageChange}
                sidebarCollapsed={assetTreeCollapsed}
                onToggleSidebar={toggleSidebar}
                onHideSidebar={toggleSidebarHidden}
                aiPanelCollapsed={aiPanelCollapsed}
                onToggleAIPanel={toggleAIPanel}
              />
            )}
            <div
              className="relative overflow-hidden shrink-0 transition-[width] duration-200"
              style={{ width: assetTreeCollapsed ? 0 : assetTreeWidth }}
            >
              <AssetTree
                collapsed={false}
                sidebarHidden={sidebarHidden}
                onShowSidebar={toggleSidebarHidden}
                onAddAsset={handleAddAsset}
                onAddGroup={() => {
                  setEditingGroup(null);
                  setGroupDialogOpen(true);
                }}
                onEditGroup={(group) => {
                  setEditingGroup(group);
                  setGroupDialogOpen(true);
                }}
                onGroupDetail={(group) => {
                  selectGroup(group.ID);
                  selectAsset(null);
                  handleOpenInfoTab("group", group.ID, group.Name, group.Icon || undefined);
                }}
                onEditAsset={handleEditAsset}
                onCopyAsset={handleCopyAsset}
                onConnectAsset={handleConnectAsset}
                onConnectAssetInNewTab={handleConnectAssetInNewTab}
                onSelectAsset={handleSelectAsset}
                onOpenInfoTab={handleOpenInfoTab}
              />
              {/* Resize handle */}
              {!assetTreeCollapsed && (
                <div
                  className="absolute right-0 top-0 bottom-0 w-1 cursor-col-resize z-10 hover:bg-primary/20 active:bg-primary/30 transition-colors"
                  onMouseDown={handleAssetTreeResizeStart}
                />
              )}
            </div>
            {assetTreeResizing && <div className="fixed inset-0 z-50 cursor-col-resize" />}
            <MainPanel
              onEditAsset={handleEditAsset}
              onDeleteAsset={handleDeleteAsset}
              onConnectAsset={handleConnectAsset}
            />
            <ConversationListPanel
              collapsed={aiPanelCollapsed}
              onToggle={() => setAiPanelCollapsed(!aiPanelCollapsed)}
              onOpenConversation={handleOpenConversation}
            />
          </div>

          <AssetForm
            open={assetFormOpen}
            onOpenChange={setAssetFormOpen}
            editAsset={editingAsset}
            defaultGroupId={defaultGroupId}
          />
          <GroupDialog open={groupDialogOpen} onOpenChange={setGroupDialogOpen} editGroup={editingGroup} />
          <PermissionDialog />
          <OpsctlApprovalDialog />
          <Toaster richColors />
        </TooltipProvider>
      </ErrorBoundary>
    </ThemeProvider>
  );
}

export default App;
