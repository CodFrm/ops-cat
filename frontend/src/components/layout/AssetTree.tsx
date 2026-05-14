import React, { useEffect, useMemo, useRef, useState } from "react";
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
import { getAssetTreeMoveBeforeId, type AssetTreeSortableItem } from "@/lib/assetTreeReorder";
import { getAssetType } from "@/lib/assetTypes";
import { getAssetTypeOptions, matchSelectedTypes } from "@/lib/assetTypes/options";
import { AssetTypeFilterButton } from "@/components/asset/AssetTypeFilterButton";
import { useAssetStore } from "@/stores/assetStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useExtensionStore } from "@/extension";
import { useActiveAssetIds } from "@/hooks/useActiveAssetIds";
import { MoveAsset, MoveGroup, ReorderAsset, ReorderGroup } from "../../../wailsjs/go/app/App";
import { asset_entity, group_entity } from "../../../wailsjs/go/models";
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

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

const FILTER_LS_KEY = "asset_tree_type_filter";
const HIDE_EMPTY_LS_KEY = "asset_tree_hide_empty_groups";

function loadFilter(): string[] {
  try {
    const raw = localStorage.getItem(FILTER_LS_KEY);
    if (!raw) return [];
    if (raw === '"all"' || raw === "all") return [];
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed) && parsed.every((v) => typeof v === "string")) {
      return parsed as string[];
    }
    return [];
  } catch {
    return [];
  }
}

function saveFilter(value: string[]) {
  localStorage.setItem(FILTER_LS_KEY, JSON.stringify(value));
}

function loadHideEmpty(): boolean {
  return localStorage.getItem(HIDE_EMPTY_LS_KEY) === "true";
}

function saveHideEmpty(value: boolean) {
  localStorage.setItem(HIDE_EMPTY_LS_KEY, value ? "true" : "false");
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
  const extensions = useExtensionStore((s) => s.extensions);
  const activeAssetIds = useActiveAssetIds();
  const [filter, setFilter] = useState("");
  const [selectedTypes, setSelectedTypes] = useState<string[]>(loadFilter);
  const [hideEmptyGroups, setHideEmptyGroups] = useState<boolean>(loadHideEmpty);
  const [deleteConfirm, setDeleteConfirm] = useState<{
    id: number;
    assetCount: number;
  } | null>(null);
  const [deleteAssetConfirm, setDeleteAssetConfirm] = useState<asset_entity.Asset | null>(null);

  useEffect(() => {
    fetchAssets();
    fetchGroups();
  }, [fetchAssets, fetchGroups]);

  useEffect(() => {
    saveFilter(selectedTypes);
  }, [selectedTypes]);

  useEffect(() => {
    saveHideEmpty(hideEmptyGroups);
  }, [hideEmptyGroups]);

  const typeOptions = useMemo(() => getAssetTypeOptions(extensions), [extensions]);

  const typeFilteredAssets = matchSelectedTypes(assets, selectedTypes, typeOptions);
  const filteredAssets = filter
    ? filterAssets(typeFilteredAssets, groups, { query: filter }).map((r) => r.asset)
    : typeFilteredAssets;

  // Group assets by GroupID
  const groupedAssets = new Map<number, asset_entity.Asset[]>();
  for (const asset of filteredAssets) {
    const gid = asset.GroupID || 0;
    if (!groupedAssets.has(gid)) groupedAssets.set(gid, []);
    groupedAssets.get(gid)!.push(asset);
  }

  const rawChildGroups = (parentId: number) => groups.filter((g) => (g.ParentID || 0) === parentId);

  const countAssetsInGroup = (groupId: number): number => {
    let count = (groupedAssets.get(groupId) || []).length;
    for (const child of rawChildGroups(groupId)) {
      count += countAssetsInGroup(child.ID);
    }
    return count;
  };

  const childGroups = (parentId: number) => {
    const all = rawChildGroups(parentId);
    return hideEmptyGroups ? all.filter((g) => countAssetsInGroup(g.ID) > 0) : all;
  };

  const visibleRootGroups = childGroups(0);

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

  // 5px 移动门槛 → 单击/双击不被误识别成拖动
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));

  // 拖动用的扁平 id 列表：DFS 顺序展开所有可见分组与资产
  const sortableIds = useMemo(() => {
    const ids: string[] = [];
    const walk = (parentId: number) => {
      for (const g of childGroups(parentId)) {
        ids.push(`group-${g.ID}`);
        walk(g.ID);
        for (const a of groupedAssets.get(g.ID) || []) {
          ids.push(`asset-${a.ID}`);
        }
      }
    };
    walk(0);
    if ((groupedAssets.get(0) || []).length > 0) {
      ids.push("group-0");
      for (const a of groupedAssets.get(0) || []) {
        ids.push(`asset-${a.ID}`);
      }
    }
    return ids;
    // childGroups / groupedAssets 每次渲染都是新引用，但底层依赖 groups/assets/hideEmptyGroups
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [groups, assets, hideEmptyGroups, selectedTypes, filter]);

  const handleDragEnd = async (e: DragEndEvent) => {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const activeStr = String(active.id);
    const overStr = String(over.id);
    const [activeKind, activeIdStr] = activeStr.split("-");
    const [overKind, overIdStr] = overStr.split("-");
    const activeId = Number(activeIdStr);
    const overId = Number(overIdStr);
    if (!Number.isFinite(activeId) || !Number.isFinite(overId)) return;

    try {
      if (activeKind === "asset") {
        if (overKind === "asset") {
          const overAsset = assets.find((a) => a.ID === overId);
          const targetGroupID = overAsset?.GroupID ?? 0;
          const assetGroupById = new Map(assets.map((a) => [a.ID, a.GroupID ?? 0] as const));
          const beforeID = getAssetTreeMoveBeforeId({
            sortableIds,
            activeSortableId: activeStr,
            overSortableId: overStr,
            targetKind: "asset",
            targetContainerId: targetGroupID,
            getContainerId: (item: AssetTreeSortableItem) =>
              item.kind === "asset" ? assetGroupById.get(item.id) : undefined,
          });
          if (beforeID === null) return;
          await ReorderAsset(activeId, targetGroupID, beforeID);
        } else if (overKind === "group") {
          // 拖到分组（含未分组桶 id=0）→ 追加到该分组末尾
          await ReorderAsset(activeId, overId, 0);
        } else {
          return;
        }
      } else if (activeKind === "group") {
        if (activeId === 0) return; // 未分组桶不可拖
        if (overKind === "group") {
          if (overId === 0) {
            // 拖到未分组桶 → 不支持把分组放进“未分组”里
            return;
          }
          const overGroup = groups.find((g) => g.ID === overId);
          const targetParentID = overGroup?.ParentID ?? 0;
          const groupParentById = new Map(groups.map((g) => [g.ID, g.ParentID ?? 0] as const));
          const beforeID = getAssetTreeMoveBeforeId({
            sortableIds,
            activeSortableId: activeStr,
            overSortableId: overStr,
            targetKind: "group",
            targetContainerId: targetParentID,
            getContainerId: (item: AssetTreeSortableItem) =>
              item.kind === "group" ? groupParentById.get(item.id) : undefined,
          });
          if (beforeID === null) return;
          await ReorderGroup(activeId, targetParentID, beforeID);
        } else if (overKind === "asset") {
          const overAsset = assets.find((a) => a.ID === overId);
          if (!overAsset || overAsset.GroupID === 0) return;
          // 拖到资产所在分组下，作为该分组末位子分组
          await ReorderGroup(activeId, overAsset.GroupID, 0);
        }
      }
      await refresh();
    } catch (err) {
      toast.error(String(err));
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

  if (collapsed) return null;

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
        <div className="flex items-center gap-1">
          <div className="relative flex-1">
            <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground" />
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder={t("asset.search")}
              className="h-7 w-full rounded-md border border-sidebar-border bg-sidebar pl-7 pr-2 text-xs outline-none focus:border-ring focus:ring-1 focus:ring-ring/50 placeholder:text-muted-foreground/60 transition-colors duration-150"
            />
          </div>
          <AssetTypeFilterButton
            value={selectedTypes}
            options={typeOptions}
            onChange={setSelectedTypes}
            hideEmptyGroups={hideEmptyGroups}
            onHideEmptyGroupsChange={setHideEmptyGroups}
          />
        </div>
      </div>
      <ScrollArea className="flex-1 min-h-0">
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={sortableIds} strategy={verticalListSortingStrategy}>
            <ContextMenu>
              <ContextMenuTrigger className="block min-h-full">
                <div className="p-2 space-y-0.5">
                  {visibleRootGroups.map((group) => (
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
                      onAddAsset={onAddAsset}
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
                      onAddAsset={onAddAsset}
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
                  {filteredAssets.length === 0 && (
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
          </SortableContext>
        </DndContext>
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
  onAddAsset: (groupId: number) => void;
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
  const isUngrouped = group.ID === 0;

  const sortable = useSortable({
    id: `group-${group.ID}`,
    disabled: isUngrouped ? { draggable: true, droppable: false } : false,
  });
  const groupRowStyle: React.CSSProperties = {
    paddingLeft: `${8 + depth * 12}px`,
    transform: CSS.Transform.toString(sortable.transform),
    transition: sortable.transition,
    opacity: sortable.isDragging ? 0.5 : undefined,
  };

  const groupRow = (
    <div
      // dnd-kit's setNodeRef/attributes/listeners are callbacks, not React refs — react-hooks/refs misfires here
      // eslint-disable-next-line react-hooks/refs
      ref={sortable.setNodeRef}
      // eslint-disable-next-line react-hooks/refs
      {...(!isUngrouped ? sortable.attributes : {})}
      // eslint-disable-next-line react-hooks/refs
      {...(!isUngrouped ? sortable.listeners : {})}
      className="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium hover:bg-sidebar-accent cursor-pointer transition-colors duration-150"
      style={groupRowStyle}
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
      {!isUngrouped ? (
        <ContextMenu>
          <ContextMenuTrigger>{groupRow}</ContextMenuTrigger>
          <ContextMenuContent>
            <ContextMenuItem onClick={() => onAddAsset(group.ID)}>
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
      {expanded && (
        <div className="tree-group-content">
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
          {assets.map((asset) => (
            <AssetRow
              key={asset.ID}
              asset={asset}
              depth={depth}
              selectedAssetId={selectedAssetId}
              activeAssetIds={activeAssetIds}
              connectingAssetIds={connectingAssetIds}
              clickTimerRef={clickTimerRef}
              onSelectAsset={onSelectAsset}
              onEditAsset={onEditAsset}
              onCopyAsset={onCopyAsset}
              onConnectAsset={onConnectAsset}
              onConnectAssetInNewTab={onConnectAssetInNewTab}
              onDeleteAsset={onDeleteAsset}
              onMoveAsset={onMoveAsset}
              onOpenInfoTab={onOpenInfoTab}
              t={t}
            />
          ))}
          {assets.length === 0 && children.length === 0 && (
            <div
              className="pr-2 py-1 text-xs text-muted-foreground cursor-pointer hover:underline"
              style={{ paddingLeft: `${20 + (depth + 1) * 12}px` }}
              onClick={() => onAddAsset(group.ID)}
            >
              + {t("asset.addAsset")}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function AssetRow({
  asset,
  depth,
  selectedAssetId,
  activeAssetIds,
  connectingAssetIds,
  clickTimerRef,
  onSelectAsset,
  onEditAsset,
  onCopyAsset,
  onConnectAsset,
  onConnectAssetInNewTab,
  onDeleteAsset,
  onMoveAsset,
  onOpenInfoTab,
  t,
}: {
  asset: asset_entity.Asset;
  depth: number;
  selectedAssetId: number | null;
  activeAssetIds: Set<number>;
  connectingAssetIds: Set<number>;
  clickTimerRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>;
  onSelectAsset: (asset: asset_entity.Asset) => void;
  onEditAsset: (asset: asset_entity.Asset) => void;
  onCopyAsset: (asset: asset_entity.Asset) => void;
  onConnectAsset: (asset: asset_entity.Asset) => void;
  onConnectAssetInNewTab?: (asset: asset_entity.Asset) => void;
  onDeleteAsset: (asset: asset_entity.Asset) => void;
  onMoveAsset: (id: number, direction: string) => void;
  onOpenInfoTab?: (type: "asset" | "group", id: number, name: string, icon?: string) => void;
  t: (key: string) => string;
}) {
  const AssetIcon = asset.Icon ? getIconComponent(asset.Icon) : Server;
  const isConnecting = connectingAssetIds.has(asset.ID);
  const sortable = useSortable({ id: `asset-${asset.ID}` });
  const style: React.CSSProperties = {
    paddingLeft: `${20 + (depth + 1) * 12}px`,
    transform: CSS.Transform.toString(sortable.transform),
    transition: sortable.transition,
    opacity: sortable.isDragging ? 0.5 : undefined,
  };

  return (
    <ContextMenu>
      <ContextMenuTrigger>
        <div
          // dnd-kit's setNodeRef/attributes/listeners are callbacks, not React refs — react-hooks/refs misfires here
          // eslint-disable-next-line react-hooks/refs
          ref={sortable.setNodeRef}
          // eslint-disable-next-line react-hooks/refs
          {...sortable.attributes}
          // eslint-disable-next-line react-hooks/refs
          {...sortable.listeners}
          className={`flex items-center gap-1.5 rounded-md pr-2 py-1.5 text-sm cursor-pointer select-none transition-colors duration-150 ${
            selectedAssetId === asset.ID
              ? "bg-sidebar-accent text-sidebar-accent-foreground"
              : "hover:bg-sidebar-accent"
          }`}
          style={style}
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
            {t("action.editPermission")}
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
}
