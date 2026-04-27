import {
  useState,
  useEffect,
  useRef,
  useCallback,
  useMemo,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { useTranslation } from "react-i18next";
import { createPortal } from "react-dom";
import {
  Database,
  RefreshCw,
  Loader2,
  Search,
  Key,
  AlertCircle,
  Copy,
  Trash2,
  List,
  FolderTree,
  Plus,
  ChevronRight,
  ChevronDown,
  Folder,
  FolderOpen,
} from "lucide-react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { toast } from "sonner";
import { Button, Input, ConfirmDialog } from "@opskat/ui";
import { useQueryStore } from "@/stores/queryStore";
import { useTabStore, type QueryTabMeta } from "@/stores/tabStore";
import { RedisDeleteKeys } from "../../../wailsjs/go/app/App";
import { RedisCreateKeyDialog } from "./RedisCreateKeyDialog";

interface RedisKeyBrowserProps {
  tabId: string;
}

const KEY_ROW_HEIGHT = 28;
const DEFAULT_SEPARATOR = ":";
const MAX_TREE_PREFETCH_KEYS = 20_000;

// --- Tree logic ---

interface TreeNode {
  name: string; // segment name (e.g. "user")
  fullKey: string | null; // non-null = leaf node (actual Redis key)
  children: Map<string, TreeNode>;
  keyCount: number; // total leaf keys under this node
}

function buildKeyTree(keys: string[], separator: string): TreeNode {
  const root: TreeNode = { name: "", fullKey: null, children: new Map(), keyCount: 0 };
  const sep = separator || DEFAULT_SEPARATOR;
  for (const key of keys) {
    const parts = key.split(sep);
    let node = root;
    for (let i = 0; i < parts.length; i++) {
      const segment = parts[i];
      const isLast = i === parts.length - 1;
      if (isLast) {
        // Leaf — always create a unique entry with full key
        const existing = node.children.get(segment);
        if (existing && existing.fullKey === null) {
          // A folder already exists with this name; add as a leaf child with the full key
          // Use the full key as child key to avoid conflicts
          node.children.set(key, { name: segment, fullKey: key, children: new Map(), keyCount: 1 });
        } else {
          node.children.set(segment, { name: segment, fullKey: key, children: new Map(), keyCount: 1 });
        }
      } else {
        if (!node.children.has(segment)) {
          node.children.set(segment, { name: segment, fullKey: null, children: new Map(), keyCount: 0 });
        }
        node = node.children.get(segment)!;
      }
    }
    // Update counts up the path
    let n = root;
    for (let i = 0; i < parts.length - 1; i++) {
      if (n.children.has(parts[i])) {
        n = n.children.get(parts[i])!;
        n.keyCount++;
      }
    }
    root.keyCount++;
  }
  return root;
}

interface FlatTreeRow {
  depth: number;
  name: string;
  fullKey: string | null; // null = folder
  keyCount: number;
  isExpanded: boolean;
  nodeId: string; // unique ID for expansion tracking (prefix path)
}

function flattenTree(root: TreeNode, expandedSet: Set<string>, separator: string): FlatTreeRow[] {
  const result: FlatTreeRow[] = [];
  const sep = separator || DEFAULT_SEPARATOR;
  const walk = (node: TreeNode, depth: number, prefix: string) => {
    // Sort children: folders first, then leaves
    const entries = Array.from(node.children.values()).sort((a, b) => {
      const aIsFolder = a.fullKey === null ? 0 : 1;
      const bIsFolder = b.fullKey === null ? 0 : 1;
      if (aIsFolder !== bIsFolder) return aIsFolder - bIsFolder;
      return a.name.localeCompare(b.name);
    });
    for (const child of entries) {
      const nodeId = prefix ? `${prefix}${sep}${child.name}` : child.name;
      const isExpanded = expandedSet.has(nodeId);
      result.push({
        depth,
        name: child.name,
        fullKey: child.fullKey,
        keyCount: child.keyCount,
        isExpanded,
        nodeId,
      });
      if (child.fullKey === null && isExpanded) {
        walk(child, depth + 1, nodeId);
      }
    }
  };
  walk(root, 0, "");
  return result;
}

interface RedisDbSelectorProps {
  currentDb: number;
  dbOptions: number[];
  dbKeyCounts: Record<number, number>;
  disabled?: boolean;
  onChange: (db: number) => void;
}

function RedisDbSelector({ currentDb, dbOptions, dbKeyCounts, disabled, onChange }: RedisDbSelectorProps) {
  const buttonRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const optionRefs = useRef<Record<number, HTMLButtonElement | null>>({});
  const [open, setOpen] = useState(false);
  const [activeDb, setActiveDb] = useState(currentDb);
  const [menuStyle, setMenuStyle] = useState<CSSProperties>({});

  const currentCount = dbKeyCounts[currentDb];

  const updateMenuPosition = useCallback(() => {
    const rect = buttonRef.current?.getBoundingClientRect();
    if (!rect) return;
    const estimatedHeight = Math.min(dbOptions.length, 10) * 32 + 8;
    setMenuStyle({
      left: rect.left,
      top: Math.max(8, rect.top - estimatedHeight - 4),
      width: rect.width,
    });
  }, [dbOptions.length]);

  const openMenu = useCallback(() => {
    if (disabled) return;
    setActiveDb(currentDb);
    updateMenuPosition();
    setOpen(true);
  }, [currentDb, disabled, updateMenuPosition]);

  const selectDb = useCallback(
    (db: number) => {
      setOpen(false);
      if (db !== currentDb) {
        onChange(db);
      }
    },
    [currentDb, onChange]
  );

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (buttonRef.current?.contains(target) || menuRef.current?.contains(target)) return;
      setOpen(false);
    };
    const onResize = () => updateMenuPosition();
    document.addEventListener("pointerdown", onPointerDown, true);
    window.addEventListener("resize", onResize);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown, true);
      window.removeEventListener("resize", onResize);
    };
  }, [open, updateMenuPosition]);

  useEffect(() => {
    if (!open) return;
    optionRefs.current[activeDb]?.scrollIntoView?.({ block: "nearest" });
  }, [activeDb, open]);

  const moveActive = (step: number) => {
    const index = Math.max(0, dbOptions.indexOf(activeDb));
    const nextIndex = Math.min(dbOptions.length - 1, Math.max(0, index + step));
    setActiveDb(dbOptions[nextIndex]);
  };

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        aria-haspopup="listbox"
        aria-expanded={open}
        className="flex h-7 min-w-0 flex-1 items-center justify-between gap-2 rounded-md border border-input bg-transparent px-2 text-left text-xs shadow-xs outline-none transition-[color,box-shadow] hover:bg-accent focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50"
        disabled={disabled}
        onClick={() => {
          if (open) {
            setOpen(false);
          } else {
            openMenu();
          }
        }}
        onKeyDown={(event: ReactKeyboardEvent<HTMLButtonElement>) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            if (!open) {
              openMenu();
              return;
            }
            moveActive(event.key === "ArrowDown" ? 1 : -1);
          } else if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            if (open) {
              selectDb(activeDb);
            } else {
              openMenu();
            }
          } else if (event.key === "Escape") {
            setOpen(false);
          }
        }}
      >
        <span className="min-w-0 truncate">
          db{currentDb}
          {currentCount !== undefined && currentCount > 0 ? (
            <span className="ml-1 text-muted-foreground">({currentCount})</span>
          ) : null}
        </span>
        <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
      </button>

      {open &&
        createPortal(
          <div
            ref={menuRef}
            data-testid="redis-db-menu"
            role="listbox"
            className="z-50 overflow-y-auto rounded-md border bg-popover p-1 text-xs text-popover-foreground shadow-md"
            style={{ position: "fixed", maxHeight: "320px", ...menuStyle }}
          >
            {dbOptions.map((db) => {
              const count = dbKeyCounts[db];
              const selected = db === currentDb;
              const active = db === activeDb;
              return (
                <button
                  key={db}
                  ref={(node) => {
                    optionRefs.current[db] = node;
                  }}
                  type="button"
                  role="option"
                  aria-selected={selected}
                  className={`flex h-8 w-full items-center justify-between gap-2 rounded-sm px-2 text-left font-mono outline-none ${
                    selected
                      ? "bg-primary text-primary-foreground"
                      : active
                        ? "bg-accent text-accent-foreground"
                        : "hover:bg-accent hover:text-accent-foreground"
                  }`}
                  onMouseEnter={() => setActiveDb(db)}
                  onClick={() => selectDb(db)}
                >
                  <span>db{db}</span>
                  {count !== undefined && count > 0 ? (
                    <span className={selected ? "text-primary-foreground/80" : "text-muted-foreground"}>{count}</span>
                  ) : null}
                </button>
              );
            })}
          </div>,
          document.body
        )}
    </>
  );
}

// --- Component ---

export function RedisKeyBrowser({ tabId }: RedisKeyBrowserProps) {
  const { t } = useTranslation();
  const state = useQueryStore((s) => s.redisStates[tabId]);
  const scanKeys = useQueryStore((s) => s.scanKeys);
  const selectRedisDb = useQueryStore((s) => s.selectRedisDb);
  const selectKey = useQueryStore((s) => s.selectKey);
  const setKeyFilter = useQueryStore((s) => s.setKeyFilter);
  const loadDbKeyCounts = useQueryStore((s) => s.loadDbKeyCounts);
  const removeKey = useQueryStore((s) => s.removeKey);
  const tab = useTabStore((s) => s.tabs.find((tb) => tb.id === tabId));
  const tabMeta = tab?.meta as QueryTabMeta | undefined;
  const keySeparator = tabMeta?.redisKeySeparator || DEFAULT_SEPARATOR;
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  // View mode: "list" or "tree". Redis keys are hierarchical in most real datasets,
  // so match Redis GUI conventions by opening in tree mode first.
  const [viewMode, setViewMode] = useState<"list" | "tree">("tree");
  const [treeExpanded, setTreeExpanded] = useState<Set<string>>(new Set());

  // Context menu state
  const [ctxMenu, setCtxMenu] = useState<{ x: number; y: number; key: string } | null>(null);
  const ctxMenuRef = useRef<HTMLDivElement>(null);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  // Build tree data
  const keyTree = useMemo(() => {
    if (viewMode !== "tree" || !state) return null;
    return buildKeyTree(state.keys, keySeparator);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [viewMode, state?.keys, keySeparator]);

  const flatRows = useMemo(() => {
    if (!keyTree) return [];
    return flattenTree(keyTree, treeExpanded, keySeparator);
  }, [keyTree, treeExpanded, keySeparator]);

  const virtualizer = useVirtualizer({
    count: viewMode === "tree" ? flatRows.length : (state?.keys.length ?? 0),
    getScrollElement: () => scrollRef.current,
    estimateSize: () => KEY_ROW_HEIGHT,
    overscan: 20,
  });
  const currentDbTotal = state ? state.dbKeyCounts[state.currentDb] : undefined;
  const keyFilterIsDefault = !state || state.keyFilter.trim() === "" || state.keyFilter === "*";
  const treeCountsIncomplete = Boolean(
    state &&
      (state.hasMore || (keyFilterIsDefault && currentDbTotal !== undefined && currentDbTotal > state.keys.length))
  );

  useEffect(() => {
    scanKeys(tabId, true);
    loadDbKeyCounts(tabId);
  }, [tabId, scanKeys, loadDbKeyCounts]);

  useEffect(() => {
    if (!state || viewMode !== "tree" || !keyFilterIsDefault || state.loadingKeys || !state.hasMore) return;
    if (currentDbTotal === undefined || currentDbTotal > MAX_TREE_PREFETCH_KEYS || state.keys.length >= currentDbTotal) return;
    const timer = window.setTimeout(() => {
      scanKeys(tabId, false);
    }, 80);
    return () => window.clearTimeout(timer);
  }, [
    currentDbTotal,
    keyFilterIsDefault,
    scanKeys,
    state?.currentDb,
    state?.hasMore,
    state?.keys.length,
    state?.keyFilter,
    state?.loadingKeys,
    state?.scanCursor,
    tabId,
    viewMode,
  ]);

  // Reset tree expansion when DB changes
  useEffect(() => {
    setTreeExpanded(new Set());
  }, [state?.currentDb]);

  // Close context menu on outside click / escape
  useEffect(() => {
    if (!ctxMenu) return;
    const close = () => setCtxMenu(null);
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    const onPointer = (e: PointerEvent) => {
      if (ctxMenuRef.current?.contains(e.target as Node)) return;
      close();
    };
    const timer = setTimeout(() => {
      document.addEventListener("pointerdown", onPointer, true);
    }, 50);
    document.addEventListener("keydown", onKey);
    return () => {
      clearTimeout(timer);
      document.removeEventListener("pointerdown", onPointer, true);
      document.removeEventListener("keydown", onKey);
    };
  }, [ctxMenu]);

  const handleDbChange = useCallback(
    (db: number) => {
      selectRedisDb(tabId, db);
    },
    [tabId, selectRedisDb]
  );

  const handleFilterChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const pattern = e.target.value || "*";
      setKeyFilter(tabId, pattern);

      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
      debounceRef.current = setTimeout(() => {
        scanKeys(tabId, true);
      }, 300);
    },
    [tabId, setKeyFilter, scanKeys]
  );

  const handleRefresh = useCallback(() => {
    scanKeys(tabId, true);
    loadDbKeyCounts(tabId);
  }, [tabId, scanKeys, loadDbKeyCounts]);

  const handleSelectKey = useCallback(
    (key: string) => {
      selectKey(tabId, key);
    },
    [tabId, selectKey]
  );

  const toggleTreeNode = useCallback((nodeId: string) => {
    setTreeExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(nodeId)) {
        next.delete(nodeId);
      } else {
        next.add(nodeId);
      }
      return next;
    });
  }, []);

  const handleCopyKeyName = useCallback(() => {
    if (!ctxMenu) return;
    navigator.clipboard.writeText(ctxMenu.key);
    toast.success(t("query.copied"));
    setCtxMenu(null);
  }, [ctxMenu, t]);

  const handleDeleteFromCtx = useCallback(() => {
    if (!ctxMenu) return;
    setDeleteTarget(ctxMenu.key);
    setCtxMenu(null);
  }, [ctxMenu]);

  const confirmDelete = useCallback(async () => {
    if (!deleteTarget || !tabMeta || !state) return;
    try {
      await RedisDeleteKeys(tabMeta.assetId, state.currentDb, [deleteTarget]);
      removeKey(tabId, deleteTarget);
      loadDbKeyCounts(tabId);
    } catch (err) {
      toast.error(String(err));
    }
    setDeleteTarget(null);
  }, [deleteTarget, tabMeta, state, tabId, removeKey, loadDbKeyCounts]);

  if (!state) return null;

  const handleScroll = () => {
    if (!state.hasMore || state.loadingKeys) return;
    const el = scrollRef.current;
    if (!el) return;
    const distanceToBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    if (distanceToBottom <= KEY_ROW_HEIGHT * 4) {
      scanKeys(tabId, false);
    }
  };

  const handleCreatedKey = async (key: string) => {
    await scanKeys(tabId, true);
    await loadDbKeyCounts(tabId);
    await selectKey(tabId, key);
  };

  const dbOptions = Array.from({ length: Math.max(16, state.currentDb + 1) }, (_, i) => i);

  return (
    <div className="flex h-full flex-col">
      {/* Browser actions */}
      <div className="flex items-center gap-1 border-b px-2 py-1.5">
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={() => setViewMode((v) => (v === "list" ? "tree" : "list"))}
          title={viewMode === "list" ? t("query.treeView") : t("query.listView")}
        >
          {viewMode === "list" ? <FolderTree className="size-3.5" /> : <List className="size-3.5" />}
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={() => setCreateDialogOpen(true)} title={t("query.createRedisKey")}>
          <Plus className="size-3.5" />
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={handleRefresh} disabled={state.loadingKeys} title={t("query.refreshTree")}>
          <RefreshCw className={`size-3.5 ${state.loadingKeys ? "animate-spin" : ""}`} />
        </Button>
      </div>

      {/* Filter input */}
      <div className="border-b px-2 py-1.5">
        <div className="relative">
          <Search className="absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="h-7 pl-7 text-xs"
            placeholder={t("query.filterKeys")}
            value={state.keyFilter === "*" ? "" : state.keyFilter}
            onChange={handleFilterChange}
          />
        </div>
      </div>

      {/* Key count */}
      <div className="border-b px-2 py-1 text-xs text-muted-foreground">
        {t("query.keyCount", { count: state.keys.length })}
      </div>

      {/* Error message */}
      {state.error && (
        <div className="flex items-start gap-2 border-b border-destructive/20 bg-destructive/10 px-2 py-2 text-xs text-destructive">
          <AlertCircle className="size-3.5 shrink-0 mt-0.5" />
          <span className="break-all">{state.error}</span>
        </div>
      )}

      {/* Virtualized key list / tree */}
      <div
        ref={scrollRef}
        data-testid="redis-key-tree"
        data-counts-incomplete={treeCountsIncomplete ? "true" : "false"}
        className="flex-1 overflow-y-auto"
        onScroll={handleScroll}
      >
        <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
          {virtualizer.getVirtualItems().map((virtualRow) => {
            if (viewMode === "tree") {
              const row = flatRows[virtualRow.index];
              if (!row) return null;
              const isLeaf = row.fullKey !== null;
              return (
                <button
                  key={virtualRow.key}
                  className={`absolute left-0 flex w-full items-center gap-1 text-left text-xs hover:bg-accent ${
                    isLeaf && state.selectedKey === row.fullKey ? "bg-accent text-accent-foreground" : ""
                  }`}
                  style={{
                    top: virtualRow.start,
                    height: virtualRow.size,
                    paddingLeft: `${row.depth * 16 + 8}px`,
                    paddingRight: "8px",
                  }}
                  onClick={() => {
                    if (isLeaf) {
                      handleSelectKey(row.fullKey!);
                    } else {
                      toggleTreeNode(row.nodeId);
                    }
                  }}
                  onContextMenu={
                    isLeaf
                      ? (e) => {
                          e.preventDefault();
                          e.stopPropagation();
                          setCtxMenu({ x: e.clientX, y: e.clientY, key: row.fullKey! });
                        }
                      : undefined
                  }
                >
                  {isLeaf ? (
                    <Key className="size-3 shrink-0 text-muted-foreground" />
                  ) : row.isExpanded ? (
                    <>
                      <ChevronDown className="size-3 shrink-0 text-muted-foreground" />
                      <FolderOpen className="size-3 shrink-0 text-muted-foreground" />
                    </>
                  ) : (
                    <>
                      <ChevronRight className="size-3 shrink-0 text-muted-foreground" />
                      <Folder className="size-3 shrink-0 text-muted-foreground" />
                    </>
                  )}
                  <span className="truncate font-mono">{row.name}</span>
                  {!isLeaf && (
                    <span className="ml-auto shrink-0 text-muted-foreground text-[10px]">
                      {row.keyCount}
                      {treeCountsIncomplete ? "+" : ""}
                    </span>
                  )}
                </button>
              );
            }

            // Flat list mode
            const key = state.keys[virtualRow.index];
            return (
              <button
                key={key}
                className={`absolute left-0 flex w-full items-center gap-1.5 px-2 text-left text-xs font-mono hover:bg-accent ${
                  state.selectedKey === key ? "bg-accent text-accent-foreground" : ""
                }`}
                style={{
                  top: virtualRow.start,
                  height: virtualRow.size,
                }}
                onClick={() => handleSelectKey(key)}
                onContextMenu={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  setCtxMenu({ x: e.clientX, y: e.clientY, key });
                }}
              >
                <Key className="size-3 shrink-0 text-muted-foreground" />
                <span className="truncate">{key}</span>
              </button>
            );
          })}
        </div>

        {state.loadingKeys && (
          <div className="flex items-center justify-center py-3">
            <Loader2 className="size-4 animate-spin text-muted-foreground" />
          </div>
        )}
      </div>

      {/* DB selector */}
      <div data-testid="redis-db-footer" className="flex items-center gap-1 border-t px-2 py-1.5">
        <Database className="size-3.5 shrink-0 text-muted-foreground" />
        <RedisDbSelector
          currentDb={state.currentDb}
          dbOptions={dbOptions}
          dbKeyCounts={state.dbKeyCounts}
          onChange={handleDbChange}
        />
      </div>

      {tabMeta && (
        <RedisCreateKeyDialog
          open={createDialogOpen}
          assetId={tabMeta.assetId}
          db={state.currentDb}
          onOpenChange={setCreateDialogOpen}
          onCreated={handleCreatedKey}
        />
      )}

      {/* Key context menu */}
      {ctxMenu &&
        createPortal(
          <div
            ref={ctxMenuRef}
            className="z-50 min-w-[8rem] overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md animate-in fade-in-0 zoom-in-95"
            style={{ position: "fixed", top: ctxMenu.y + 2, left: ctxMenu.x + 2 }}
          >
            <div
              role="menuitem"
              className="relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none hover:bg-accent hover:text-accent-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0"
              onClick={handleCopyKeyName}
            >
              <Copy className="size-3.5" />
              {t("query.copyKeyName")}
            </div>
            <div
              role="menuitem"
              className="relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none hover:bg-accent hover:text-accent-foreground text-destructive [&_svg]:pointer-events-none [&_svg]:shrink-0"
              onClick={handleDeleteFromCtx}
            >
              <Trash2 className="size-3.5" />
              {t("query.deleteKey")}
            </div>
          </div>,
          document.body
        )}

      {/* Delete key confirm */}
      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("query.deleteKey")}
        description={t("query.deleteKeyConfirmDesc", { name: deleteTarget ?? "" })}
        cancelText={t("action.cancel")}
        confirmText={t("action.delete")}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
