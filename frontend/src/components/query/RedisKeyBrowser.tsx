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
import {
  DEFAULT_REDIS_KEY_SEPARATOR,
  buildKeyTree,
  flattenTree,
  isDefaultRedisKeyFilter,
  makeLocalKeyMatcher,
} from "@/lib/redisKeyTree";
import { RedisDeleteKeys } from "../../../wailsjs/go/app/App";
import { RedisCreateKeyDialog } from "./RedisCreateKeyDialog";

interface RedisKeyBrowserProps {
  tabId: string;
}

const KEY_ROW_HEIGHT = 28;
const MAX_TREE_PREFETCH_KEYS = 20_000;
const EMPTY_REDIS_KEYS: string[] = [];

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
  const keySeparator = tabMeta?.redisKeySeparator || DEFAULT_REDIS_KEY_SEPARATOR;
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
  const committedFilter = state?.keyFilter === "*" ? "" : (state?.keyFilter ?? "");
  const hasRedisState = state != null;
  const redisKeys = state?.keys ?? EMPTY_REDIS_KEYS;
  const redisHasMore = state?.hasMore ?? false;
  const redisLoadingKeys = state?.loadingKeys ?? false;
  const [draftFilter, setDraftFilter] = useState(committedFilter);

  useEffect(() => {
    setDraftFilter(committedFilter);
  }, [committedFilter, state?.currentDb, tabId]);

  const visibleKeys = useMemo(() => {
    if (!hasRedisState) return [];
    const matcher = makeLocalKeyMatcher(draftFilter);
    return redisKeys.filter(matcher);
  }, [draftFilter, hasRedisState, redisKeys]);

  // Build tree data
  const keyTree = useMemo(() => {
    if (viewMode !== "tree" || !state) return null;
    return buildKeyTree(visibleKeys, keySeparator);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [viewMode, visibleKeys, keySeparator]);

  const flatRows = useMemo(() => {
    if (!keyTree) return [];
    return flattenTree(keyTree, treeExpanded, keySeparator);
  }, [keyTree, treeExpanded, keySeparator]);

  const rowCount = viewMode === "tree" ? flatRows.length : visibleKeys.length;
  const virtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => KEY_ROW_HEIGHT,
    initialRect: { width: 0, height: KEY_ROW_HEIGHT * 20 },
    overscan: 20,
  });
  const virtualRows = virtualizer.getVirtualItems();
  const renderRows =
    virtualRows.length > 0
      ? virtualRows
      : Array.from({ length: Math.min(rowCount, 20) }, (_, index) => ({
          index,
          key: index,
          start: index * KEY_ROW_HEIGHT,
          size: KEY_ROW_HEIGHT,
          end: (index + 1) * KEY_ROW_HEIGHT,
          lane: 0,
        }));
  const currentDbTotal = state ? state.dbKeyCounts[state.currentDb] : undefined;
  const keyFilterIsDefault = !state || isDefaultRedisKeyFilter(state.keyFilter);
  const draftFilterIsDefault = isDefaultRedisKeyFilter(draftFilter);
  const isLocalOnlyFilter = draftFilter.trim() !== committedFilter.trim();
  const treeCountsIncomplete = Boolean(
    hasRedisState &&
    draftFilterIsDefault &&
    (redisHasMore || (keyFilterIsDefault && currentDbTotal !== undefined && currentDbTotal > redisKeys.length))
  );

  useEffect(() => {
    scanKeys(tabId, true);
    loadDbKeyCounts(tabId);
  }, [tabId, scanKeys, loadDbKeyCounts]);

  useEffect(() => {
    if (
      !hasRedisState ||
      viewMode !== "tree" ||
      !keyFilterIsDefault ||
      !draftFilterIsDefault ||
      redisLoadingKeys ||
      !redisHasMore
    )
      return;
    if (currentDbTotal === undefined || currentDbTotal > MAX_TREE_PREFETCH_KEYS || redisKeys.length >= currentDbTotal)
      return;
    const timer = window.setTimeout(() => {
      scanKeys(tabId, false);
    }, 80);
    return () => window.clearTimeout(timer);
  }, [
    currentDbTotal,
    hasRedisState,
    keyFilterIsDefault,
    draftFilterIsDefault,
    redisHasMore,
    redisKeys.length,
    redisLoadingKeys,
    scanKeys,
    state?.currentDb,
    state?.keyFilter,
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

  const handleFilterChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setDraftFilter(e.target.value);
  }, []);

  const commitFilterAndScan = useCallback(async () => {
    setKeyFilter(tabId, draftFilter.trim() || "*");
    await scanKeys(tabId, true);
  }, [draftFilter, scanKeys, setKeyFilter, tabId]);

  const handleFilterKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === "Enter") {
        e.preventDefault();
        commitFilterAndScan();
      }
      if (e.key === "Escape") {
        setDraftFilter(committedFilter);
      }
    },
    [commitFilterAndScan, committedFilter]
  );

  const handleRefresh = useCallback(() => {
    setKeyFilter(tabId, draftFilter.trim() || "*");
    scanKeys(tabId, true);
    loadDbKeyCounts(tabId);
  }, [draftFilter, tabId, setKeyFilter, scanKeys, loadDbKeyCounts]);

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
    if (!state.hasMore || state.loadingKeys || isLocalOnlyFilter) return;
    const el = scrollRef.current;
    if (!el) return;
    const distanceToBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    if (distanceToBottom <= KEY_ROW_HEIGHT * 4) {
      scanKeys(tabId, false);
    }
  };

  const handleCreatedKey = async (key: string, createdDb: number) => {
    if (createdDb !== state.currentDb) {
      await selectRedisDb(tabId, createdDb);
    }
    await scanKeys(tabId, true);
    await loadDbKeyCounts(tabId);
    await selectKey(tabId, key);
  };

  const dbOptions = Array.from(
    new Set([
      ...Array.from({ length: Math.max(16, state.currentDb + 1) }, (_, i) => i),
      ...Object.keys(state.dbKeyCounts)
        .map(Number)
        .filter((db) => Number.isInteger(db) && db >= 0),
    ])
  ).sort((a, b) => a - b);

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
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={() => setCreateDialogOpen(true)}
          title={t("query.createRedisKey")}
        >
          <Plus className="size-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={handleRefresh}
          disabled={state.loadingKeys}
          title={t("query.refreshTree")}
        >
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
            value={draftFilter}
            onChange={handleFilterChange}
            onKeyDown={handleFilterKeyDown}
          />
        </div>
      </div>

      {/* Key count */}
      <div className="border-b px-2 py-1 text-xs text-muted-foreground">
        {t("query.keyCount", { count: visibleKeys.length })}
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
          {renderRows.map((virtualRow) => {
            if (viewMode === "tree") {
              const row = flatRows[virtualRow.index];
              if (!row) return null;
              const isKey = row.fullKey !== null;
              const isFolder = row.hasChildren;
              return (
                <div
                  key={virtualRow.key}
                  className={`absolute left-0 flex w-full items-center text-xs hover:bg-accent ${
                    isKey && state.selectedKey === row.fullKey ? "bg-accent text-accent-foreground" : ""
                  }`}
                  style={{
                    top: virtualRow.start,
                    height: virtualRow.size,
                    paddingLeft: `${row.depth * 16 + 4}px`,
                    paddingRight: "8px",
                  }}
                  onContextMenu={
                    isKey
                      ? (e) => {
                          e.preventDefault();
                          e.stopPropagation();
                          setCtxMenu({ x: e.clientX, y: e.clientY, key: row.fullKey! });
                        }
                      : undefined
                  }
                >
                  {isFolder ? (
                    <button
                      type="button"
                      className="flex size-5 shrink-0 items-center justify-center rounded-sm hover:bg-accent"
                      title={`${row.isExpanded ? t("query.collapseFolder") : t("query.expandFolder")} ${row.nodeId}`}
                      onClick={(event) => {
                        event.stopPropagation();
                        toggleTreeNode(row.nodeId);
                      }}
                    >
                      {row.isExpanded ? (
                        <ChevronDown className="size-3 text-muted-foreground" />
                      ) : (
                        <ChevronRight className="size-3 text-muted-foreground" />
                      )}
                    </button>
                  ) : (
                    <span className="size-5 shrink-0" />
                  )}
                  <button
                    type="button"
                    className="flex min-w-0 flex-1 items-center gap-1 text-left"
                    title={isKey ? row.fullKey! : row.nodeId}
                    onClick={() => {
                      if (isKey) {
                        handleSelectKey(row.fullKey!);
                      } else if (isFolder) {
                        toggleTreeNode(row.nodeId);
                      }
                    }}
                  >
                    {isKey ? (
                      <Key className="size-3 shrink-0 text-muted-foreground" />
                    ) : row.isExpanded ? (
                      <FolderOpen className="size-3 shrink-0 text-muted-foreground" />
                    ) : (
                      <Folder className="size-3 shrink-0 text-muted-foreground" />
                    )}
                    <span className="truncate font-mono">{row.name}</span>
                  </button>
                  {isFolder && (
                    <span className="ml-auto shrink-0 text-muted-foreground text-[10px]">
                      {row.keyCount}
                      {treeCountsIncomplete ? "+" : ""}
                    </span>
                  )}
                </div>
              );
            }

            // Flat list mode
            const key = visibleKeys[virtualRow.index];
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
          key={`${createDialogOpen ? "open" : "closed"}:${state.currentDb}`}
          open={createDialogOpen}
          assetId={tabMeta.assetId}
          db={state.currentDb}
          dbOptions={dbOptions}
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
