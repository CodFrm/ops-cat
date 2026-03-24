import { useState, useCallback, useRef, useEffect } from "react";
import { toast } from "sonner";
import { ThemeProvider } from "@/components/theme-provider";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { Sidebar } from "@/components/layout/Sidebar";
import { AssetTree } from "@/components/layout/AssetTree";
import { MainPanel } from "@/components/layout/MainPanel";
import { ConversationListPanel } from "@/components/ai/ConversationListPanel";
import { WindowControls } from "@/components/layout/WindowControls";
import { AssetForm } from "@/components/asset/AssetForm";
import { GroupDialog } from "@/components/asset/GroupDialog";
import { PermissionDialog } from "@/components/ai/PermissionDialog";
import { OpsctlApprovalDialog } from "@/components/approval/OpsctlApprovalDialog";
import { PlanApprovalDialog } from "@/components/approval/PlanApprovalDialog";

import { useAssetStore } from "@/stores/assetStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useAIStore } from "@/stores/aiStore";
import { useKeyboardShortcuts } from "@/hooks/useKeyboardShortcuts";
import { asset_entity, group_entity } from "../wailsjs/go/models";

const AI_TAB_PREFIX = "ai:";

function App() {
  const [openPageTabs, setOpenPageTabs] = useState<string[]>([]);
  const [activePageTab, setActivePageTab] = useState<string | null>(null);

  const activePage = activePageTab || "home";

  const handlePageChange = useCallback((page: string) => {
    if (page === "home") {
      setActivePageTab(null);
    } else if (page.startsWith(AI_TAB_PREFIX)) {
      const aiTabId = page.slice(AI_TAB_PREFIX.length);
      useAIStore.getState().setActiveAITab(aiTabId);
      setActivePageTab(page);
    } else {
      if (!openPageTabs.includes(page)) {
        setOpenPageTabs((prev) => [...prev, page]);
      }
      setActivePageTab(page);
    }
  }, [openPageTabs]);

  const closePageTab = useCallback((pageId: string) => {
    if (pageId.startsWith(AI_TAB_PREFIX)) {
      const aiTabId = pageId.slice(AI_TAB_PREFIX.length);
      useAIStore.getState().closeConversationTab(aiTabId);
      setActivePageTab((prev) => (prev === pageId ? null : prev));
    } else {
      setOpenPageTabs((prev) => prev.filter((id) => id !== pageId));
      setActivePageTab((prev) => (prev === pageId ? null : prev));
    }
  }, []);

  const handleTerminalTabClick = useCallback(() => {
    setActivePageTab(null);
  }, []);

  const handleOpenConversation = useCallback((tabId: string) => {
    setActivePageTab(AI_TAB_PREFIX + tabId);
  }, []);

  const [sidebarHidden, setSidebarHidden] = useState(
    () => localStorage.getItem("sidebar_hidden") === "true"
  );
  const [assetTreeCollapsed, setAssetTreeCollapsed] = useState(
    () => localStorage.getItem("sidebar_collapsed") === "true"
  );
  const [aiPanelCollapsed, setAiPanelCollapsed] = useState(
    () => localStorage.getItem("ai_panel_collapsed") === "true"
  );
  const [assetTreeWidth, setAssetTreeWidth] = useState(() => {
    const saved = localStorage.getItem("asset_tree_width");
    return saved ? Math.max(160, Math.min(480, Number(saved))) : 224; // 14rem = 224px
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
    onPageChange: handlePageChange,
    onClosePageTab: closePageTab,
    openPageTabs,
    activePageTab,
  });

  // 启动时自动激活 AI store 中已打开的第一个 tab
  const aiActiveTabId = useAIStore((s) => s.activeAITabId);
  useEffect(() => {
    if (aiActiveTabId && !activePageTab) {
      setActivePageTab(AI_TAB_PREFIX + aiActiveTabId);
    }
  }, [aiActiveTabId]); // eslint-disable-line react-hooks/exhaustive-deps

  // 资产表单
  const [assetFormOpen, setAssetFormOpen] = useState(false);
  const [editingAsset, setEditingAsset] = useState<asset_entity.Asset | null>(null);
  const [defaultGroupId, setDefaultGroupId] = useState(0);

  // 分组对话框
  const [groupDialogOpen, setGroupDialogOpen] = useState(false);
  const [editingGroup, setEditingGroup] = useState<group_entity.Group | null>(null);
const { assets, groups, selectedAssetId, selectedGroupId, selectAsset, selectGroup, deleteAsset, getAsset, getAssetPath } = useAssetStore();
  const { connect, openAssetInfo } = useTerminalStore();
  const selectedAsset = assets.find((a) => a.ID === selectedAssetId) || null;
  const selectedGroup = groups.find((g) => g.ID === selectedGroupId) || null;

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
    setActivePageTab(null);
    openAssetInfo();
  };

  const handleDeleteAsset = async (id: number) => {
    await deleteAsset(id);
  };

  const handleConnectAsset = async (asset: asset_entity.Asset) => {
    const assetPath = getAssetPath(asset);
    let metadata: { host: string; port: number; username: string; forwardedPorts: { type: string; localHost: string; localPort: number; remoteHost: string; remotePort: number }[] } | undefined;
    try {
      const cfg = JSON.parse(asset.Config || "{}");
      metadata = {
        host: cfg.host || "",
        port: cfg.port || 22,
        username: cfg.username || "",
        forwardedPorts: (cfg.forwarded_ports || []).map((p: { type?: string; local_host?: string; local_port?: number; remote_host?: string; remote_port?: number }) => ({
          type: p.type || "local",
          localHost: p.local_host || "",
          localPort: p.local_port || 0,
          remoteHost: p.remote_host || "",
          remotePort: p.remote_port || 0,
        })),
      };
    } catch { /* ignore parse errors */ }
    try {
      await connect(asset.ID, assetPath, "", 80, 24, metadata);
      setActivePageTab(null);
    } catch (e) {
      toast.error(`${assetPath}: ${String(e)}`);
    }
  };


  return (
    <ThemeProvider defaultTheme="system">
      <TooltipProvider>
        <div className="flex h-screen w-screen overflow-hidden bg-background">
          <WindowControls />
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
                setActivePageTab(null);
                openAssetInfo();
              }}
              onEditAsset={handleEditAsset}
              onCopyAsset={handleCopyAsset}
              onConnectAsset={handleConnectAsset}
              onSelectAsset={handleSelectAsset}
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
            activePage={activePage}
            selectedAsset={selectedAsset}
            selectedGroup={selectedGroup}
            onEditAsset={handleEditAsset}
            onDeleteAsset={handleDeleteAsset}
            onConnectAsset={handleConnectAsset}
            openPageTabs={openPageTabs}
            activePageTab={activePageTab}
            onActivatePageTab={handlePageChange}
            onClosePageTab={closePageTab}
            onTerminalTabClick={handleTerminalTabClick}
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
        <GroupDialog
          open={groupDialogOpen}
          onOpenChange={setGroupDialogOpen}
          editGroup={editingGroup}
        />
<PermissionDialog />
<OpsctlApprovalDialog />
<PlanApprovalDialog />
<Toaster richColors />
      </TooltipProvider>
    </ThemeProvider>
  );
}

export default App;
