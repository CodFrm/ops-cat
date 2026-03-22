import { useState, useCallback } from "react";
import { toast } from "sonner";
import { ThemeProvider } from "@/components/theme-provider";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { Sidebar } from "@/components/layout/Sidebar";
import { AssetTree } from "@/components/layout/AssetTree";
import { MainPanel } from "@/components/layout/MainPanel";
import { AIPanel } from "@/components/layout/AIPanel";
import { WindowControls } from "@/components/layout/WindowControls";
import { AssetForm } from "@/components/asset/AssetForm";
import { GroupDialog } from "@/components/asset/GroupDialog";

import { useAssetStore } from "@/stores/assetStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useKeyboardShortcuts } from "@/hooks/useKeyboardShortcuts";
import { asset_entity, group_entity } from "../wailsjs/go/models";

function App() {
  const [activePage, setActivePage] = useState("home");
  const [assetTreeCollapsed, setAssetTreeCollapsed] = useState(
    () => localStorage.getItem("sidebar_collapsed") === "true"
  );
  const [aiPanelCollapsed, setAiPanelCollapsed] = useState(
    () => localStorage.getItem("ai_panel_collapsed") === "true"
  );

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

  useKeyboardShortcuts({ onToggleAIPanel: toggleAIPanel, onToggleSidebar: toggleSidebar });

  // 资产表单
  const [assetFormOpen, setAssetFormOpen] = useState(false);
  const [editingAsset, setEditingAsset] = useState<asset_entity.Asset | null>(null);
  const [defaultGroupId, setDefaultGroupId] = useState(0);

  // 分组对话框
  const [groupDialogOpen, setGroupDialogOpen] = useState(false);
  const [editingGroup, setEditingGroup] = useState<group_entity.Group | null>(null);

const { assets, selectedAssetId, selectAsset, deleteAsset, getAssetPath } = useAssetStore();
  const { connect } = useTerminalStore();
  const selectedAsset = assets.find((a) => a.ID === selectedAssetId) || null;

  const handleAddAsset = (groupId?: number) => {
    setEditingAsset(null);
    setDefaultGroupId(groupId ?? 0);
    setAssetFormOpen(true);
  };

  const handleEditAsset = (asset: asset_entity.Asset) => {
    setEditingAsset(asset);
    setAssetFormOpen(true);
  };

  const handleSelectAsset = (asset: asset_entity.Asset) => {
    selectAsset(asset.ID);
    setActivePage("home");
  };

  const handleDeleteAsset = async (id: number) => {
    await deleteAsset(id);
  };

  const handleConnectAsset = async (asset: asset_entity.Asset) => {
    const assetPath = getAssetPath(asset);
    try {
      await connect(asset.ID, assetPath, "", 80, 24);
    } catch (e) {
      toast.error(`${assetPath}: ${String(e)}`);
    }
  };


  return (
    <ThemeProvider defaultTheme="system">
      <TooltipProvider>
        <div className="flex h-screen w-screen overflow-hidden bg-background">
          <WindowControls />
          <Sidebar
            activePage={activePage}
            onPageChange={setActivePage}
            sidebarCollapsed={assetTreeCollapsed}
            onToggleSidebar={toggleSidebar}
          />
          <div
            className="overflow-hidden shrink-0 transition-[width] duration-200"
            style={{ width: assetTreeCollapsed ? 0 : "14rem" }}
          >
            <AssetTree
              collapsed={false}
              onAddAsset={handleAddAsset}
              onAddGroup={() => {
                setEditingGroup(null);
                setGroupDialogOpen(true);
              }}
              onEditGroup={(group) => {
                setEditingGroup(group);
                setGroupDialogOpen(true);
              }}
              onEditAsset={handleEditAsset}
              onConnectAsset={handleConnectAsset}
              onSelectAsset={handleSelectAsset}
            />
          </div>
          <MainPanel
            activePage={activePage}
            selectedAsset={selectedAsset}
            onEditAsset={handleEditAsset}
            onDeleteAsset={handleDeleteAsset}
            onConnectAsset={handleConnectAsset}
          />
          <AIPanel
            collapsed={aiPanelCollapsed}
            onToggle={() => setAiPanelCollapsed(!aiPanelCollapsed)}
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
<Toaster richColors />
      </TooltipProvider>
    </ThemeProvider>
  );
}

export default App;
