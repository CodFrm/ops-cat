import React, { useEffect, useRef, useState } from "react";
import { useFullscreen } from "@/hooks/useFullscreen";
import {
  ChevronRight,
  ChevronDown,
  Folder,
  Server,
  Plus,
  FolderPlus,
  Search,
  Loader2,
  Eye,
  ArrowUp,
  ArrowDown,
  ChevronsUp,
  Pencil,
  Copy,
  Trash2,
  TerminalSquare,
  ExternalLink,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import {
  Button,
  ScrollArea,
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogCancel,
  AlertDialogAction,
  ConfirmDialog,
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@opskat/ui";
import { getIconComponent, getIconColor } from "@/components/asset/IconPicker";
import { filterAssets } from "@/lib/assetSearch";
import { getAssetType } from "@/lib/assetTypes";
import { useAssetStore } from "@/stores/assetStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useActiveAssetIds } from "@/hooks/useActiveAssetIds";
import { MoveAsset, MoveGroup } from "../../../wailsjs/go/app/App";
import { asset_entity, group_entity } from "../../../wailsjs/go/models";

interface AssetTreeProps {
  collapsed: boolean;
  sidebarHidden?: boolean;
  onShowSidebar?: () => void;
  onAddAsset: (groupId?: number) => void;
  onAddGroup: () => void;
  onEditGroup: (group: group_entity.Group) => void;
  onGroupDetail: (group: group_entity.Group) => void;
  onEditAsset: (asset: asset_entity.Asset) => void;
  onCopyAsset: (asset: asset_entity.Asset) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
  onConnectAssetInNewTab?: (asset: asset_entity.Asset) => void;
  onSelectAsset: (asset: asset_entity.Asset) => void;
  onOpenInfoTab?: (type: "asset" | "group", id: number, name: string, icon?: string) => void;
}

export function AssetTree({
  collapsed,
  sidebarHidden,
  onShowSidebar,
  onAddAsset,
  onAddGroup,
  onEditGroup,
  onGroupDetail,
  onEditAsset,
  onCopyAsset,
  onConnectAsset,
  onConnectAssetInNewTab,
  onSelectAsset,
  onOpenInfoTab,
}: AssetTreeProps) {
  const { t } = useTranslation();
  const isFullscreen = useFullscreen();
  const { assets, groups, selectedAssetId, fetchAssets, fetchGroups, deleteAsset, deleteGroup, refresh } =
    useAssetStore();
  const connectingAssetIds = useTerminalStore((s) => s.connectingAssetIds);
  const activeAssetIds = useActiveAssetIds();
  const [filter, setFilter] = useState("");
  const [deleteConfirm, setDeleteConfirm] = useState<{
    id: number;
    assetCount: number;
  } | null>(null);
  const [deleteAssetConfirm, setDeleteAssetConfirm] = useState<asset_entity.Asset | null>(null);

  useEffect(() => {
    fetchAssets();
    fetchGroups();
  }, [fetchAssets, fetchGroups]);

  if (collapsed) return null;

  const filteredAssets = filter ? filterAssets(assets, groups, { query: filter }).map((r) => r.asset) : assets;

  // Group assets by GroupID
  const groupedAssets = new Map<number, asset_entity.Asset[]>();
  for (const asset of filteredAssets) {
    const gid = asset.GroupID || 0;
    if (!groupedAssets.has(gid)) groupedAssets.set(gid, []);
    groupedAssets.get(gid)!.push(asset);
  }

  const childGroups = (parentId: number) => groups.filter((g) => (g.ParentID || 0) === parentId);

  const countAssetsInGroup = (groupId: number): number => {
    let count = (groupedAssets.get(groupId) || []).length;
    for (const child of childGroups(groupId)) {
      count += countAssetsInGroup(child.ID);
    }
    return count;
  };

  const handleDeleteGroup = (id: number) => {
    const directAssetCount = (groupedAssets.get(id) || []).length;
    if (directAssetCount > 0) {
      setDeleteConfirm({ id, assetCount: directAssetCount });
    } else {
      deleteGroup(id, false).catch((e) => toast.error(String(e)));
    }
  };

  const handleMoveAsset = async (id: number, direction: string) => {
    try {
      await MoveAsset(id, direction);
      await refresh();
    } catch (e) {
      toast.error(String(e));
    }
  };

  const handleMoveGroup = async (id: number, direction: string) => {
    try {
      await MoveGroup(id, direction);
      await refresh();
    } catch (e) {
      toast.error(String(e));
    }
  };

  const handleConfirmDelete = async (deleteAssets: boolean) => {
    if (!deleteConfirm) return;
    try {
      await deleteGroup(deleteConfirm.id, deleteAssets);
    } catch (e) {
      toast.error(String(e));
    }
    setDeleteConfirm(null);
  };

  return (
    <div className="flex h-full w-full flex-col border-r border-panel-divider bg-sidebar">
      {/* Drag region for frameless window */}
      <div
        className={`${isFullscreen ? "h-0" : "h-8"} w-full shrink-0`}
        style={{ "--wails-draggable": "drag" } as React.CSSProperties}
      />
      <div className="flex flex-col gap-1.5 px-3 pb-2 border-b border-panel-divider">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1">
            {sidebarHidden && (
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6"
                onClick={onShowSidebar}
                title={t("panel.showSidebar")}
              >
                <Eye className="h-3.5 w-3.5" />
              </Button>
            )}
            <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {t("asset.title")}
            </span>
          </div>
          <div className="flex gap-0.5">
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6"
              onClick={() => onAddGroup()}
              title={t("asset.addGroup")}
            >
              <FolderPlus className="h-3.5 w-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6"
              onClick={() => onAddAsset()}
              title={t("asset.addAsset")}
            >
              <Plus className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground" />
          <input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder={t("asset.search") || "Search..."}
            className="h-7 w-full rounded-md border border-sidebar-border bg-sidebar pl-7 pr-2 text-xs outline-none focus:border-ring focus:ring-1 focus:ring-ring/50 placeholder:text-muted-foreground/60 transition-colors duration-150"
          />
        </div>
      </div>
      <ScrollArea className="flex-1 min-h-0">
        <ContextMenu>
          <ContextMenuTrigger className="block min-h-full">
            <div className="p-2 space-y-0.5">
              {childGroups(0).map((group) => (
                <GroupItem
                  key={group.ID}
                  group={group}
                  assets={groupedAssets.get(group.ID) || []}
                  allGroupedAssets={groupedAssets}
                  childGroups={childGroups}
                  countAssetsInGroup={countAssetsInGroup}
                  selectedAssetId={selectedAssetId}
                  activeAssetIds={activeAssetIds}
                  connectingAssetIds={connectingAssetIds}
                  onSelectAsset={onSelectAsset}
                  onAddAsset={() => onAddAsset(group.ID)}
                  onEditAsset={onEditAsset}
                  onCopyAsset={onCopyAsset}
                  onConnectAsset={onConnectAsset}
                  onConnectAssetInNewTab={onConnectAssetInNewTab}
                  onEditGroup={onEditGroup}
                  onGroupDetail={onGroupDetail}
                  onDeleteGroup={handleDeleteGroup}
                  onDeleteAsset={(asset: asset_entity.Asset) => setDeleteAssetConfirm(asset)}
                  onMoveAsset={handleMoveAsset}
                  onMoveGroup={handleMoveGroup}
                  onOpenInfoTab={onOpenInfoTab}
                  depth={0}
                  t={t}
                />
              ))}
              {(groupedAssets.get(0) || []).length > 0 && (
                <GroupItem
                  group={
                    new group_entity.Group({
                      ID: 0,
                      Name: t("asset.ungrouped"),
                    })
                  }
                  assets={groupedAssets.get(0) || []}
                  allGroupedAssets={groupedAssets}
                  childGroups={() => []}
                  countAssetsInGroup={() => (groupedAssets.get(0) || []).length}
                  selectedAssetId={selectedAssetId}
                  activeAssetIds={activeAssetIds}
                  connectingAssetIds={connectingAssetIds}
                  onSelectAsset={onSelectAsset}
                  onAddAsset={() => onAddAsset(0)}
                  onEditAsset={onEditAsset}
                  onCopyAsset={onCopyAsset}
                  onConnectAsset={onConnectAsset}
                  onConnectAssetInNewTab={onConnectAssetInNewTab}
                  onEditGroup={onEditGroup}
                  onGroupDetail={onGroupDetail}
                  onDeleteGroup={handleDeleteGroup}
                  onDeleteAsset={(asset) => setDeleteAssetConfirm(asset)}
                  onMoveAsset={handleMoveAsset}
                  onMoveGroup={handleMoveGroup}
                  onOpenInfoTab={onOpenInfoTab}
                  depth={0}
                  t={t}
                />
              )}
              {filteredAssets.length === 0 && groups.length === 0 && (
                <p className="text-xs text-muted-foreground text-center py-4">{t("asset.addAsset")}</p>
              )}
            </div>
          </ContextMenuTrigger>
          <ContextMenuContent>
            <ContextMenuItem onClick={() => onAddAsset()}>
              <Plus className="h-3.5 w-3.5 mr-1.5" />
              {t("asset.addAsset")}
            </ContextMenuItem>
            <ContextMenuItem onClick={() => onAddGroup()}>
              <FolderPlus className="h-3.5 w-3.5 mr-1.5" />
              {t("asset.addGroup")}
            </ContextMenuItem>
          </ContextMenuContent>
        </ContextMenu>
      </ScrollArea>
      <AlertDialog open={!!deleteConfirm} onOpenChange={(open) => !open && setDeleteConfirm(null)}>
        <AlertDialogContent onOverlayClick={() => setDeleteConfirm(null)}>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("asset.deleteGroupTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("asset.deleteGroupDesc", { count: deleteConfirm?.assetCount })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("action.cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={() => handleConfirmDelete(false)}>
              {t("asset.moveToUngrouped")}
            </AlertDialogAction>
            <AlertDialogAction variant="destructive" onClick={() => handleConfirmDelete(true)}>
              {t("asset.deleteAssets")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <ConfirmDialog
        open={!!deleteAssetConfirm}
        onOpenChange={(open) => !open && setDeleteAssetConfirm(null)}
        title={t("asset.deleteAssetTitle")}
        description={t("asset.deleteAssetDesc", { name: deleteAssetConfirm?.Name })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={() => {
          if (deleteAssetConfirm) {
            deleteAsset(deleteAssetConfirm.ID);
          }
          setDeleteAssetConfirm(null);
        }}
      />
    </div>
  );
}

function DynamicIcon({ icon, className, style }: { icon?: string; className?: string; style?: React.CSSProperties }) {
  return React.createElement(icon ? getIconComponent(icon) : Folder, { className, style });
}

function GroupItem({
  group,
  assets,
  allGroupedAssets,
  childGroups,
  countAssetsInGroup,
  selectedAssetId,
  activeAssetIds,
  connectingAssetIds,
  onSelectAsset,
  onAddAsset,
  onEditAsset,
  onCopyAsset,
  onConnectAsset,
  onConnectAssetInNewTab,
  onEditGroup,
  onGroupDetail,
  onDeleteGroup,
  onDeleteAsset,
  onMoveAsset,
  onMoveGroup,
  onOpenInfoTab,
  depth,
  t,
}: {
  group: group_entity.Group;
  assets: asset_entity.Asset[];
  allGroupedAssets: Map<number, asset_entity.Asset[]>;
  childGroups: (parentId: number) => group_entity.Group[];
  countAssetsInGroup: (groupId: number) => number;
  selectedAssetId: number | null;
  activeAssetIds: Set<number>;
  connectingAssetIds: Set<number>;
  onSelectAsset: (asset: asset_entity.Asset) => void;
  onAddAsset: () => void;
  onEditAsset: (asset: asset_entity.Asset) => void;
  onCopyAsset: (asset: asset_entity.Asset) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
  onConnectAssetInNewTab?: (asset: asset_entity.Asset) => void;
  onEditGroup: (group: group_entity.Group) => void;
  onGroupDetail: (group: group_entity.Group) => void;
  onDeleteGroup: (id: number) => void;
  onDeleteAsset: (asset: asset_entity.Asset) => void;
  onMoveAsset: (id: number, direction: string) => void;
  onMoveGroup: (id: number, direction: string) => void;
  onOpenInfoTab?: (type: "asset" | "group", id: number, name: string, icon?: string) => void;
  depth: number;
  t: (key: string) => string;
}) {
  const expanded = useAssetStore((s) => !s.collapsedGroupIds.includes(group.ID));
  const toggleGroupCollapsed = useAssetStore((s) => s.toggleGroupCollapsed);
  const clickTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const children = group.ID > 0 ? childGroups(group.ID) : [];
  const totalCount = countAssetsInGroup(group.ID);

  const groupRow = (
    <div
      className="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium hover:bg-sidebar-accent cursor-pointer transition-colors duration-150"
      style={{ paddingLeft: `${8 + depth * 12}px` }}
      onClick={() => toggleGroupCollapsed(group.ID)}
    >
      {expanded ? (
        <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      ) : (
        <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      )}
      <DynamicIcon
        icon={group.Icon}
        className="h-3.5 w-3.5 shrink-0 text-muted-foreground"
        style={group.Icon ? { color: getIconColor(group.Icon) } : undefined}
      />
      <span className="truncate text-sidebar-foreground">{group.Name}</span>
      <span className="ml-auto text-xs text-muted-foreground">{totalCount}</span>
    </div>
  );

  return (
    <div>
      {group.ID > 0 ? (
        <ContextMenu>
          <ContextMenuTrigger>{groupRow}</ContextMenuTrigger>
          <ContextMenuContent>
            <ContextMenuItem onClick={() => onAddAsset()}>
              <Plus className="h-3.5 w-3.5 mr-1.5" />
              {t("asset.addAsset")}
            </ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem onClick={() => onGroupDetail(group)}>
              <Eye className="h-3.5 w-3.5 mr-1.5" />
              {t("asset.groupDetail")}
            </ContextMenuItem>
            {onOpenInfoTab && (
              <ContextMenuItem onClick={() => onOpenInfoTab("group", group.ID, group.Name, group.Icon)}>
                <ExternalLink className="h-3.5 w-3.5 mr-1.5" />
                {t("action.openInTab")}
              </ContextMenuItem>
            )}
            <ContextMenuItem onClick={() => onEditGroup(group)}>
              <Pencil className="h-3.5 w-3.5 mr-1.5" />
              {t("action.edit")}
            </ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem onClick={() => onMoveGroup(group.ID, "up")}>
              <ArrowUp className="h-3.5 w-3.5 mr-1.5" />
              {t("asset.moveUp")}
            </ContextMenuItem>
            <ContextMenuItem onClick={() => onMoveGroup(group.ID, "down")}>
              <ArrowDown className="h-3.5 w-3.5 mr-1.5" />
              {t("asset.moveDown")}
            </ContextMenuItem>
            <ContextMenuItem onClick={() => onMoveGroup(group.ID, "top")}>
              <ChevronsUp className="h-3.5 w-3.5 mr-1.5" />
              {t("asset.moveTop")}
            </ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem className="text-destructive" onClick={() => onDeleteGroup(group.ID)}>
              <Trash2 className="h-3.5 w-3.5 mr-1.5" />
              {t("action.delete")}
            </ContextMenuItem>
          </ContextMenuContent>
        </ContextMenu>
      ) : (
        groupRow
      )}
      <div className="tree-group-content" data-collapsed={!expanded ? "true" : undefined}>
        <div>
          {children.map((child) => (
            <GroupItem
              key={child.ID}
              group={child}
              assets={allGroupedAssets.get(child.ID) || []}
              allGroupedAssets={allGroupedAssets}
              childGroups={childGroups}
              countAssetsInGroup={countAssetsInGroup}
              selectedAssetId={selectedAssetId}
              activeAssetIds={activeAssetIds}
              connectingAssetIds={connectingAssetIds}
              onSelectAsset={onSelectAsset}
              onAddAsset={onAddAsset}
              onEditAsset={onEditAsset}
              onCopyAsset={onCopyAsset}
              onConnectAsset={onConnectAsset}
              onConnectAssetInNewTab={onConnectAssetInNewTab}
              onEditGroup={onEditGroup}
              onGroupDetail={onGroupDetail}
              onDeleteGroup={onDeleteGroup}
              onDeleteAsset={onDeleteAsset}
              onMoveAsset={onMoveAsset}
              onMoveGroup={onMoveGroup}
              onOpenInfoTab={onOpenInfoTab}
              depth={depth + 1}
              t={t}
            />
          ))}
          {assets.map((asset) => {
            const AssetIcon = asset.Icon ? getIconComponent(asset.Icon) : Server;
            const isConnecting = connectingAssetIds.has(asset.ID);
            return (
              <ContextMenu key={asset.ID}>
                <ContextMenuTrigger>
                  <div
                    className={`flex items-center gap-1.5 rounded-md pr-2 py-1.5 text-sm cursor-pointer select-none transition-colors duration-150 ${
                      selectedAssetId === asset.ID
                        ? "bg-sidebar-accent text-sidebar-accent-foreground"
                        : "hover:bg-sidebar-accent"
                    }`}
                    style={{ paddingLeft: `${20 + (depth + 1) * 12}px` }}
                    onClick={() => {
                      if (clickTimerRef.current) clearTimeout(clickTimerRef.current);
                      clickTimerRef.current = setTimeout(() => {
                        clickTimerRef.current = null;
                        onSelectAsset(asset);
                      }, 200);
                    }}
                    onDoubleClick={() => {
                      if (clickTimerRef.current) {
                        clearTimeout(clickTimerRef.current);
                        clickTimerRef.current = null;
                      }
                      onSelectAsset(asset);
                      const def = getAssetType(asset.Type);
                      if (def?.canConnect && (def.connectAction === "query" || !isConnecting)) {
                        onConnectAsset(asset);
                      }
                    }}
                  >
                    {isConnecting ? (
                      <Loader2 className="h-3.5 w-3.5 shrink-0 text-muted-foreground animate-spin" />
                    ) : (
                      <AssetIcon
                        className="h-3.5 w-3.5 shrink-0 text-muted-foreground"
                        style={asset.Icon ? { color: getIconColor(asset.Icon) } : undefined}
                      />
                    )}
                    {activeAssetIds.has(asset.ID) && <span className="h-1.5 w-1.5 rounded-full bg-success shrink-0" />}
                    <span className="truncate text-sidebar-foreground">{asset.Name}</span>
                  </div>
                </ContextMenuTrigger>
                <ContextMenuContent>
                  {getAssetType(asset.Type)?.canConnect && (
                    <ContextMenuItem onClick={() => onConnectAsset(asset)} disabled={isConnecting}>
                      {isConnecting ? (
                        <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                      ) : (
                        <TerminalSquare className="h-3.5 w-3.5 mr-1.5" />
                      )}
                      {t("asset.connect")}
                    </ContextMenuItem>
                  )}
                  {getAssetType(asset.Type)?.canConnectInNewTab && onConnectAssetInNewTab && (
                    <ContextMenuItem onClick={() => onConnectAssetInNewTab(asset)} disabled={isConnecting}>
                      <ExternalLink className="h-3.5 w-3.5 mr-1.5" />
                      {t("asset.connectInNewTab")}
                    </ContextMenuItem>
                  )}
                  {onOpenInfoTab && (
                    <ContextMenuItem onClick={() => onOpenInfoTab("asset", asset.ID, asset.Name, asset.Icon)}>
                      <Eye className="h-3.5 w-3.5 mr-1.5" />
                      {t("action.openInTab")}
                    </ContextMenuItem>
                  )}
                  <ContextMenuItem onClick={() => onEditAsset(asset)}>
                    <Pencil className="h-3.5 w-3.5 mr-1.5" />
                    {t("action.edit")}
                  </ContextMenuItem>
                  <ContextMenuItem onClick={() => onCopyAsset(asset)}>
                    <Copy className="h-3.5 w-3.5 mr-1.5" />
                    {t("action.copy")}
                  </ContextMenuItem>
                  <ContextMenuSeparator />
                  <ContextMenuItem onClick={() => onMoveAsset(asset.ID, "up")}>
                    <ArrowUp className="h-3.5 w-3.5 mr-1.5" />
                    {t("asset.moveUp")}
                  </ContextMenuItem>
                  <ContextMenuItem onClick={() => onMoveAsset(asset.ID, "down")}>
                    <ArrowDown className="h-3.5 w-3.5 mr-1.5" />
                    {t("asset.moveDown")}
                  </ContextMenuItem>
                  <ContextMenuItem onClick={() => onMoveAsset(asset.ID, "top")}>
                    <ChevronsUp className="h-3.5 w-3.5 mr-1.5" />
                    {t("asset.moveTop")}
                  </ContextMenuItem>
                  <ContextMenuSeparator />
                  <ContextMenuItem className="text-destructive" onClick={() => onDeleteAsset(asset)}>
                    <Trash2 className="h-3.5 w-3.5 mr-1.5" />
                    {t("action.delete")}
                  </ContextMenuItem>
                </ContextMenuContent>
              </ContextMenu>
            );
          })}
          {assets.length === 0 && children.length === 0 && (
            <div
              className="pr-2 py-1 text-xs text-muted-foreground cursor-pointer hover:underline"
              style={{ paddingLeft: `${20 + (depth + 1) * 12}px` }}
              onClick={onAddAsset}
            >
              + {t("asset.addAsset")}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
