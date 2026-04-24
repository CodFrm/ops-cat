import { useState, useCallback, useRef, useEffect } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { ThemeProvider } from "@/components/theme-provider";
import { TooltipProvider, Toaster } from "@opskat/ui";
import { Sidebar } from "@/components/layout/Sidebar";
import { AssetTree } from "@/components/layout/AssetTree";
import { MainPanel } from "@/components/layout/MainPanel";
import { SideAssistantPanel } from "@/components/ai/SideAssistantPanel";
import { WindowControls } from "@/components/layout/WindowControls";
import { EdgeRevealStrip } from "@/components/layout/EdgeRevealStrip";
import { useLayoutStore } from "@/stores/layoutStore";
import { LeftPanel } from "@/components/layout/LeftPanel";
import { SideTabList } from "@/components/layout/SideTabList";
import { AssetForm } from "@/components/asset/AssetForm";
import { GroupDialog } from "@/components/asset/GroupDialog";
import { PermissionDialog } from "@/components/ai/PermissionDialog";
import { OpsctlApprovalDialog } from "@/components/approval/OpsctlApprovalDialog";
import { ErrorBoundary } from "@/components/ErrorBoundary";

import { useAssetStore } from "@/stores/assetStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useQueryStore } from "@/stores/queryStore";
import { getAssetType, normalizeAssetSection } from "@/lib/assetTypes";
import { useTabStore, type InfoTabMeta, type PageTabMeta, type QueryTabMeta, type Tab } from "@/stores/tabStore";
import { useExtensionStore } from "@/extension";
import { bootstrapExtensions } from "@/extension/init";
import { useKeyboardShortcuts } from "@/hooks/useKeyboardShortcuts";
import { asset_entity, group_entity } from "../wailsjs/go/models";
import { EventsOn, WindowToggleMaximise } from "../wailsjs/runtime/runtime";

type HomeSection = "home" | "database" | "ssh" | "redis" | "mongodb";

function tabBelongsToSection(tab: Tab, section: HomeSection): boolean {
  if (section === "home") return true;

  if (tab.type === "terminal") {
    return section === "ssh";
  }

  if (tab.type === "query") {
    const assetType = normalizeAssetSection((tab.meta as QueryTabMeta).assetType);
    return assetType === section;
  }

  if (tab.type === "info") {
    const meta = tab.meta as InfoTabMeta;
    if (meta.targetType !== "asset") return false;
    const asset = useAssetStore.getState().assets.find((a) => a.ID === meta.targetId);
    if (!asset) return false;
    return normalizeAssetSection(asset.Type) === section;
  }

  return false;
}

function App() {
  const { t } = useTranslation();

  // 异步加载数据，不阻塞首屏渲染
  useEffect(() => {
    bootstrapExtensions().catch((err) => console.error("Extension bootstrap failed:", err));
    useAssetStore
      .getState()
      .fetchAssets()
      .catch((err) => console.error("Fetch assets failed:", err));
    useAssetStore
      .getState()
      .fetchGroups()
      .catch((err) => console.error("Fetch groups failed:", err));
  }, []);

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
  const [homeSection, setHomeSection] = useState<HomeSection>("home");

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

  const hideAssetListAfterConnect = useCallback(() => {
    const layout = useLayoutStore.getState();
    if (layout.tabBarLayout === "left") {
      if (layout.leftPanelVisible) {
        layout.toggleVisible();
      }
      return;
    }

    setAssetTreeCollapsed(true);
    localStorage.setItem("sidebar_collapsed", "true");
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
    const def = getAssetType(asset.Type);
    if (def?.connectAction === "query") {
      useQueryStore.getState().openQueryTab(asset);
      hideAssetListAfterConnect();
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
        hideAssetListAfterConnect();
        return;
      }
    }

    if (def?.connectAction !== "terminal") return;
    try {
      await connect(asset);
      hideAssetListAfterConnect();
    } catch (e) {
      toast.error(`${asset.Name}: ${String(e)}`);
    }
  };

  const handleConnectAssetInNewTab = async (asset: asset_entity.Asset) => {
    if (!getAssetType(asset.Type)?.canConnectInNewTab) return;
    try {
      await connect(asset, "", true);
      hideAssetListAfterConnect();
    } catch (e) {
      toast.error(`${asset.Name}: ${String(e)}`);
    }
  };

  // Sidebar page navigation
  const handlePageChange = useCallback(
    (page: string) => {
      const tabStore = useTabStore.getState();
      if (page === "home" || page === "database" || page === "ssh" || page === "redis" || page === "mongodb") {
        const section = page as HomeSection;
        setHomeSection(section);

        const terminalConnectedCount = Object.values(useTerminalStore.getState().tabData).reduce((count, tabData) => {
          return count + Object.values(tabData.panes).filter((pane) => pane.connected).length;
        }, 0);
        const queryConnectionCount = tabStore.tabs.filter((tab) => tab.type === "query").length;
        const extensionConnectionCount = tabStore.tabs.filter((tab) => {
          if (tab.type !== "page") return false;
          const meta = tab.meta as PageTabMeta;
          return Boolean(meta.extensionName && typeof meta.assetId === "number");
        }).length;
        const openAssetConnections = terminalConnectedCount + queryConnectionCount + extensionConnectionCount;

        const layout = useLayoutStore.getState();
        const clickedSameSection = section === homeSection;

        if (clickedSameSection && openAssetConnections > 1) {
          if (layout.tabBarLayout === "left") {
            layout.toggleVisible();
          } else {
            setAssetTreeCollapsed((prev) => {
              const next = !prev;
              localStorage.setItem("sidebar_collapsed", String(next));
              return next;
            });
          }
        } else {
          if (layout.tabBarLayout === "left") {
            if (!layout.leftPanelVisible) {
              layout.toggleVisible();
            }
          } else {
            setAssetTreeCollapsed((prev) => {
              if (!prev) return prev;
              localStorage.setItem("sidebar_collapsed", "false");
              return false;
            });
          }
        }

        const candidateTabs = tabStore.tabs.filter(
          (t) => t.type === "terminal" || t.type === "query" || t.type === "info"
        );
        const target = candidateTabs.find((t) => tabBelongsToSection(t, section));
        if (target) {
          tabStore.activateTab(target.id);
        } else if (section === "home") {
          tabStore.activateTab(tabStore.tabs[0]?.id || "");
        }
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
      hideAssetListAfterConnect();
    },
    [homeSection]
  );

  const tabBarLayout = useLayoutStore((s) => s.tabBarLayout);
  const leftPanelVisible = useLayoutStore((s) => s.leftPanelVisible);
  const activeSidePanel = useLayoutStore((s) => s.activeSidePanel);

  // Derive active page for sidebar highlighting
  const activeTab = useTabStore((s) => s.tabs.find((t) => t.id === s.activeTabId));
  const activePage = activeTab?.type === "page" ? activeTab.id : homeSection;

  return (
    <ThemeProvider defaultTheme="system">
      <ErrorBoundary>
        <TooltipProvider>
          <div className="flex h-screen w-screen overflow-hidden bg-background">
            <WindowControls />
            {tabBarLayout === "left" ? (
              <>
                {sidebarHidden && <EdgeRevealStrip onClick={toggleSidebarHidden} />}
                {!sidebarHidden && (
                  <Sidebar
                    activePage={activePage}
                    onPageChange={handlePageChange}
                    sidebarCollapsed={false}
                    onToggleSidebar={() => useLayoutStore.getState().toggleVisible()}
                    onHideSidebar={toggleSidebarHidden}
                    aiPanelCollapsed={aiPanelCollapsed}
                    onToggleAIPanel={toggleAIPanel}
                  />
                )}
                {leftPanelVisible && (
                  <LeftPanel>
                    {activeSidePanel === "assets" ? (
                      <AssetTree
                        collapsed={false}
                        homeSection={homeSection}
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
                    ) : (
                      <SideTabList homeSection={homeSection} />
                    )}
                  </LeftPanel>
                )}
              </>
            ) : (
              <>
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
                    homeSection={homeSection}
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
              </>
            )}
            <MainPanel
              onEditAsset={handleEditAsset}
              onDeleteAsset={handleDeleteAsset}
              onConnectAsset={handleConnectAsset}
              homeSection={homeSection}
            />
            <SideAssistantPanel collapsed={aiPanelCollapsed} onToggle={toggleAIPanel} />
            {aiPanelCollapsed && <EdgeRevealStrip side="right" onClick={toggleAIPanel} />}
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
