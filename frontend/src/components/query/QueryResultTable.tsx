import { useState, useRef, useEffect, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { createPortal } from "react-dom";
import { Loader2, Copy, ArrowUp, ArrowDown, ArrowUpDown, Filter, Search } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "@opskat/ui";
import { toast } from "sonner";

export interface CellEdit {
  rowIdx: number;
  col: string;
  value: unknown; // new value
}

export type SortDir = "asc" | "desc" | null;

export interface RenderCellContext {
  rowIdx: number;
  col: string;
}

interface QueryResultTableProps {
  columns: string[];
  rows: Record<string, unknown>[];
  loading?: boolean;
  error?: string;
  editable?: boolean;
  edits?: Map<string, unknown>; // key: "rowIdx:col"
  onCellEdit?: (edit: CellEdit) => void;
  showRowNumber?: boolean;
  rowNumberOffset?: number;
  // Controlled sorting (server-side). If provided, clicking a header calls onSortChange
  // instead of mutating local state; the local sort fallback is disabled.
  sortColumn?: string | null;
  sortDir?: SortDir;
  onSortChange?: (col: string | null, dir: SortDir) => void;
  // When true, each header shows a filter icon that opens a checkbox list of the
  // current-page distinct values. Filtering is fully client-side.
  enableColumnFilter?: boolean;
  // Override the display-mode cell rendering. Does not affect edit-mode (input).
  // When provided, the returned node replaces the default NULL / String(value) span.
  renderCell?: (value: unknown, ctx: RenderCellContext) => React.ReactNode;
}

// Serialize a cell value for title tooltip / clipboard / filter keys. Objects
// stringify as JSON (not "[object Object]"); primitives via String(). Exported
// for unit tests.
export function cellValueToText(v: unknown): string {
  if (v == null) return "";
  if (typeof v === "object") {
    try {
      return JSON.stringify(v);
    } catch {
      return String(v);
    }
  }
  return String(v);
}

// Sentinel key used to represent NULL / undefined values in the column-filter
// Set so they don't collide with the literal string "null" etc.
const NULL_KEY = "__opskat_null_sentinel__";

function valueKey(v: unknown): string {
  if (v == null) return NULL_KEY;
  return cellValueToText(v);
}

function cellKey(rowIdx: number, col: string) {
  return `${rowIdx}:${col}`;
}

function compareValues(a: unknown, b: unknown): number {
  if (a == null && b == null) return 0;
  if (a == null) return -1;
  if (b == null) return 1;
  // Try numeric comparison
  const na = Number(a);
  const nb = Number(b);
  if (!isNaN(na) && !isNaN(nb)) return na - nb;
  return String(a).localeCompare(String(b));
}

export function QueryResultTable({
  columns,
  rows,
  loading,
  error,
  editable,
  edits,
  onCellEdit,
  showRowNumber,
  rowNumberOffset = 0,
  sortColumn: controlledSortCol,
  sortDir: controlledSortDir,
  onSortChange,
  enableColumnFilter,
  renderCell,
}: QueryResultTableProps) {
  const { t } = useTranslation();

  const [editingCell, setEditingCell] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Sort state — controlled if onSortChange is provided, otherwise local
  const isControlledSort = !!onSortChange;
  const [localSortCol, setLocalSortCol] = useState<string | null>(null);
  const [localSortDir, setLocalSortDir] = useState<SortDir>(null);
  const sortCol = isControlledSort ? (controlledSortCol ?? null) : localSortCol;
  const sortDir = isControlledSort ? (controlledSortDir ?? null) : localSortDir;

  // Column resize state
  const [colWidths, setColWidths] = useState<Record<string, number>>({});

  // Column filter popover — which column's popover is open
  const [openFilterCol, setOpenFilterCol] = useState<string | null>(null);

  // Per-column client-side filter. When a column has an entry, only values whose
  // key (see valueKey) is in the Set pass through. A column without an entry is
  // treated as "no filter" (all rows pass).
  const [columnFilters, setColumnFilters] = useState<Map<string, Set<string>>>(new Map());

  // Reset all filters whenever the underlying columns/rows change (new query /
  // page / refresh), otherwise stale keys could silently hide everything.
  useEffect(() => {
    setColumnFilters(new Map());
  }, [columns, rows]);

  // Distinct values per column, memoized so the popover doesn't recompute while
  // checkboxes are being toggled.
  const columnDistincts = useMemo(() => {
    const map = new Map<string, { value: unknown; key: string; count: number }[]>();
    for (const col of columns) {
      const counts = new Map<string, { value: unknown; key: string; count: number }>();
      for (const row of rows) {
        const v = row[col];
        const k = valueKey(v);
        const hit = counts.get(k);
        if (hit) hit.count += 1;
        else counts.set(k, { value: v == null ? null : v, key: k, count: 1 });
      }
      map.set(
        col,
        Array.from(counts.values()).sort((a, b) => b.count - a.count)
      );
    }
    return map;
  }, [columns, rows]);

  // Apply client-side filters to produce the surviving row indices.
  const filteredIndices = useMemo(() => {
    if (columnFilters.size === 0) return rows.map((_, i) => i);
    const out: number[] = [];
    outer: for (let i = 0; i < rows.length; i++) {
      for (const [c, allowed] of columnFilters) {
        if (!allowed.has(valueKey(rows[i][c]))) continue outer;
      }
      out.push(i);
    }
    return out;
  }, [rows, columnFilters]);

  const setColumnFilterForCol = useCallback((col: string, allowed: Set<string> | null) => {
    setColumnFilters((prev) => {
      const next = new Map(prev);
      if (allowed === null) next.delete(col);
      else next.set(col, allowed);
      return next;
    });
  }, []);

  // Selected cell state — click-to-focus + arrow key navigation
  const [selectedCell, setSelectedCell] = useState<{ origIdx: number; col: string } | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // Context menu state
  const [ctxMenu, setCtxMenu] = useState<{ x: number; y: number; value: unknown } | null>(null);
  const ctxMenuRef = useRef<HTMLDivElement>(null);

  // Reset local sort and column widths when columns change
  useEffect(() => {
    setLocalSortCol(null);
    setLocalSortDir(null);
    setColWidths({});
    setSelectedCell(null);
    setEditingCell(null);
  }, [columns]);

  // Reset selection / editing when row set changes (paging, refresh, filter)
  useEffect(() => {
    setSelectedCell(null);
    setEditingCell(null);
  }, [rows]);

  // Sorted row indices (only for uncontrolled/local sort). Controlled sort is
  // server-side, so rows are already in the requested order. Always based on
  // the client-side-filtered index set so hidden rows stay hidden after sorting.
  const sortedIndices = useMemo(() => {
    if (isControlledSort || !sortCol || !sortDir) return filteredIndices;
    return [...filteredIndices].sort((a, b) => {
      const cmp = compareValues(rows[a][sortCol], rows[b][sortCol]);
      return sortDir === "asc" ? cmp : -cmp;
    });
  }, [rows, sortCol, sortDir, isControlledSort, filteredIndices]);

  const toggleSort = useCallback(
    (col: string) => {
      let nextCol: string | null;
      let nextDir: SortDir;
      if (sortCol !== col) {
        nextCol = col;
        nextDir = "asc";
      } else if (sortDir === "asc") {
        nextCol = col;
        nextDir = "desc";
      } else {
        nextCol = null;
        nextDir = null;
      }
      if (isControlledSort) {
        onSortChange?.(nextCol, nextDir);
      } else {
        setLocalSortCol(nextCol);
        setLocalSortDir(nextDir);
      }
    },
    [sortCol, sortDir, isControlledSort, onSortChange]
  );

  // Sorting is enabled whenever we have an onSortChange callback (server-side)
  // or when we're in read-only mode (local client-side sort on the current page).
  const canSort = isControlledSort || !editable;

  // Column resize handler — 拖拽期间只改 DOM（rAF 合批），松手一次性 setState，避免整表 60Hz 重渲
  const handleResizeStart = useCallback((e: React.MouseEvent, col: string) => {
    e.preventDefault();
    e.stopPropagation();
    const startX = e.clientX;
    const th = (e.target as HTMLElement).closest("th") as HTMLElement | null;
    if (!th) return;
    const startWidth = th.offsetWidth;

    let pending = startWidth;
    let rafId: number | null = null;
    const flushToDom = () => {
      rafId = null;
      th.style.width = `${pending}px`;
    };

    const onMouseMove = (me: MouseEvent) => {
      pending = Math.max(50, startWidth + me.clientX - startX);
      if (rafId == null) rafId = requestAnimationFrame(flushToDom);
    };

    const onMouseUp = () => {
      if (rafId != null) {
        cancelAnimationFrame(rafId);
        rafId = null;
      }
      document.removeEventListener("mousemove", onMouseMove);
      document.removeEventListener("mouseup", onMouseUp);
      document.body.style.removeProperty("cursor");
      document.body.style.removeProperty("user-select");
      // 清掉 inline width，让 React 通过 setColWidths 接管最终宽度
      th.style.width = "";
      setColWidths((prev) => ({ ...prev, [col]: pending }));
    };

    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
    document.addEventListener("mousemove", onMouseMove);
    document.addEventListener("mouseup", onMouseUp);
  }, []);

  // Focus input when editing starts
  useEffect(() => {
    if (editingCell && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [editingCell]);

  // Close context menu on outside click / escape
  useEffect(() => {
    if (!ctxMenu) return;
    const close = () => setCtxMenu(null);
    const onKey = (e: KeyboardEvent) => {
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

  const commitEdit = useCallback(
    (rowIdx: number, col: string, newValue: string) => {
      const original = rows[rowIdx]?.[col];
      const originalStr = original == null ? "" : String(original);
      if (newValue !== originalStr) {
        onCellEdit?.({
          rowIdx,
          col,
          value: newValue === "" && original == null ? null : newValue,
        });
      }
      setEditingCell(null);
    },
    [rows, onCellEdit]
  );

  const handleCopyCell = useCallback(() => {
    if (!ctxMenu) return;
    navigator.clipboard.writeText(cellValueToText(ctxMenu.value));
    toast.success(t("query.copied"));
    setCtxMenu(null);
  }, [ctxMenu, t]);

  const handleCellContextMenu = useCallback((e: React.MouseEvent, origIdx: number, col: string, value: unknown) => {
    e.preventDefault();
    e.stopPropagation();
    setSelectedCell({ origIdx, col });
    containerRef.current?.focus();
    setCtxMenu({ x: e.clientX, y: e.clientY, value });
  }, []);

  const handleCellClick = useCallback((origIdx: number, col: string) => {
    setSelectedCell({ origIdx, col });
    containerRef.current?.focus();
  }, []);

  // Arrow key navigation + Enter/F2 to edit + Escape to deselect/cancel
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      // When editing, the input owns key events — only Escape handled here already
      // (the input's onKeyDown calls setEditingCell(null) on Escape).
      if (editingCell) return;
      if (!selectedCell) return;

      const colIdx = columns.indexOf(selectedCell.col);
      const displayIdx = sortedIndices.indexOf(selectedCell.origIdx);
      if (colIdx === -1 || displayIdx === -1) return;

      let nextDisplayIdx = displayIdx;
      let nextColIdx = colIdx;

      switch (e.key) {
        case "ArrowUp":
          nextDisplayIdx = Math.max(0, displayIdx - 1);
          break;
        case "ArrowDown":
          nextDisplayIdx = Math.min(sortedIndices.length - 1, displayIdx + 1);
          break;
        case "ArrowLeft":
          nextColIdx = Math.max(0, colIdx - 1);
          break;
        case "ArrowRight":
          nextColIdx = Math.min(columns.length - 1, colIdx + 1);
          break;
        case "Enter":
        case "F2":
          if (editable) {
            e.preventDefault();
            setEditingCell(cellKey(selectedCell.origIdx, selectedCell.col));
          }
          return;
        case "Escape":
          e.preventDefault();
          setSelectedCell(null);
          return;
        default:
          return;
      }

      e.preventDefault();
      setSelectedCell({
        origIdx: sortedIndices[nextDisplayIdx],
        col: columns[nextColIdx],
      });
    },
    [editingCell, selectedCell, sortedIndices, columns, editable]
  );

  // Scroll the selected cell into view when navigating
  useEffect(() => {
    if (!selectedCell || !containerRef.current) return;
    const key = cellKey(selectedCell.origIdx, selectedCell.col);
    const el = containerRef.current.querySelector<HTMLElement>(`[data-cell-key="${CSS.escape(key)}"]`);
    el?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [selectedCell]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return <div className="px-3 py-4 text-xs text-destructive whitespace-pre-wrap font-mono">{error}</div>;
  }

  if (columns.length === 0) {
    return (
      <div className="flex items-center justify-center py-8 text-xs text-muted-foreground">{t("query.noResult")}</div>
    );
  }

  return (
    <div className="flex-1 min-h-0 flex flex-col">
      <div
        ref={containerRef}
        tabIndex={0}
        onKeyDown={handleKeyDown}
        className="flex-1 overflow-auto min-h-0 query-table-scroll outline-none"
      >
        <table className="border-collapse text-xs font-mono">
          <thead className="bg-muted sticky top-0">
            <tr>
              {showRowNumber && (
                <th className="border border-border px-2 py-1.5 text-center font-semibold text-muted-foreground whitespace-nowrap w-[50px]">
                  #
                </th>
              )}
              {columns.map((col) => {
                const isSorted = sortCol === col;
                const width = colWidths[col];
                return (
                  <th
                    key={col}
                    className="relative border border-border px-2 py-1.5 text-left font-semibold text-muted-foreground whitespace-nowrap select-none"
                    style={width ? { width: `${width}px`, minWidth: `${width}px` } : undefined}
                    title={canSort ? t("query.sortColumn") : col}
                  >
                    <div className="flex items-center gap-1">
                      <span
                        className={`inline-flex items-center gap-1 flex-1 min-w-0 ${canSort ? "cursor-pointer" : ""}`}
                        onClick={() => canSort && toggleSort(col)}
                      >
                        <span className="truncate">{col}</span>
                        {canSort &&
                          (isSorted && sortDir === "asc" ? (
                            <ArrowUp className="h-3 w-3 shrink-0" />
                          ) : isSorted && sortDir === "desc" ? (
                            <ArrowDown className="h-3 w-3 shrink-0" />
                          ) : (
                            <ArrowUpDown className="h-3 w-3 shrink-0 opacity-30" />
                          ))}
                      </span>
                      {enableColumnFilter &&
                        (() => {
                          const curFilter = columnFilters.get(col);
                          const distinctCount = columnDistincts.get(col)?.length ?? 0;
                          // "Active" = user has a non-empty selection that doesn't cover
                          // every distinct value. Empty is normalized to null upstream;
                          // a Set equal to the full distinct set is visually "all
                          // checked" but filters nothing, so don't highlight.
                          const isActive = !!curFilter && curFilter.size > 0 && curFilter.size < distinctCount;
                          return (
                            <Popover
                              open={openFilterCol === col}
                              onOpenChange={(open) => setOpenFilterCol(open ? col : null)}
                            >
                              <PopoverTrigger asChild>
                                <button
                                  type="button"
                                  className={`shrink-0 p-0.5 rounded hover:bg-accent hover:text-accent-foreground ${
                                    isActive ? "text-primary opacity-100" : "opacity-60 hover:opacity-100"
                                  }`}
                                  onClick={(e) => e.stopPropagation()}
                                  title={t("query.filterColumn")}
                                >
                                  <Filter className={`h-3 w-3 ${isActive ? "fill-current" : ""}`} />
                                </button>
                              </PopoverTrigger>
                              <PopoverContent
                                align="start"
                                sideOffset={4}
                                className="w-72 p-0"
                                onClick={(e) => e.stopPropagation()}
                              >
                                <ColumnValuePanel
                                  col={col}
                                  entries={columnDistincts.get(col) ?? []}
                                  selected={columnFilters.get(col) ?? null}
                                  onChange={(next) => setColumnFilterForCol(col, next)}
                                />
                              </PopoverContent>
                            </Popover>
                          );
                        })()}
                    </div>
                    {/* Resize handle */}
                    <div
                      className="absolute right-0 top-0 bottom-0 w-[3px] cursor-col-resize hover:bg-primary/40 z-20"
                      onMouseDown={(e) => handleResizeStart(e, col)}
                      onClick={(e) => e.stopPropagation()}
                    />
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {sortedIndices.map((origIdx, idx) => {
              const row = rows[origIdx];
              return (
                <tr key={origIdx} className={idx % 2 === 0 ? "bg-background" : "bg-muted/40"}>
                  {showRowNumber && (
                    <td className="border border-border px-2 py-1 text-center text-muted-foreground whitespace-nowrap w-[50px]">
                      {rowNumberOffset + origIdx + 1}
                    </td>
                  )}
                  {columns.map((col) => {
                    const ck = cellKey(origIdx, col);
                    const isEdited = edits?.has(ck);
                    const displayValue = isEdited ? edits!.get(ck) : row[col];
                    const isEditing = editingCell === ck;
                    const isSelected = selectedCell?.origIdx === origIdx && selectedCell?.col === col;
                    const width = colWidths[col];

                    const focusClass = isEditing
                      ? "ring-2 ring-inset ring-primary bg-primary/5 relative z-10"
                      : isSelected
                        ? "ring-2 ring-inset ring-primary/60 relative z-10"
                        : "";

                    return (
                      <td
                        key={col}
                        data-cell-key={ck}
                        className={`border border-border px-2 py-1 whitespace-nowrap cursor-default ${
                          isEdited ? "bg-yellow-100 dark:bg-yellow-900/30" : ""
                        } ${focusClass}`}
                        style={
                          width
                            ? { width: `${width}px`, minWidth: `${width}px`, maxWidth: `${width}px` }
                            : { maxWidth: "400px" }
                        }
                        title={displayValue == null ? "NULL" : cellValueToText(displayValue)}
                        onClick={() => handleCellClick(origIdx, col)}
                        onDoubleClick={() => {
                          if (!editable) return;
                          setEditingCell(ck);
                        }}
                        onContextMenu={(e) => handleCellContextMenu(e, origIdx, col, displayValue)}
                      >
                        {isEditing ? (
                          <input
                            ref={inputRef}
                            className="w-full bg-transparent outline-none border-none p-0 m-0 text-xs font-mono"
                            defaultValue={cellValueToText(displayValue)}
                            onBlur={(e) => commitEdit(origIdx, col, e.target.value)}
                            onKeyDown={(e) => {
                              if (e.key === "Enter") {
                                // IME 合成中：让 Enter 作为候选词确认，不提交编辑

                                if (e.nativeEvent.isComposing || e.keyCode === 229) return;
                                commitEdit(origIdx, col, (e.target as HTMLInputElement).value);
                              }
                              if (e.key === "Escape") {
                                setEditingCell(null);
                              }
                            }}
                          />
                        ) : renderCell ? (
                          renderCell(displayValue, { rowIdx: origIdx, col })
                        ) : displayValue == null ? (
                          <span className="text-muted-foreground italic">NULL</span>
                        ) : (
                          <span className="truncate block">{cellValueToText(displayValue)}</span>
                        )}
                      </td>
                    );
                  })}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* Cell context menu */}
      {ctxMenu &&
        createPortal(
          <div
            ref={ctxMenuRef}
            className="z-50 min-w-[8rem] overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md animate-in fade-in-0 zoom-in-95"
            style={{ position: "fixed", top: ctxMenu.y + 2, left: ctxMenu.x + 2 }}
          >
            <div
              role="menuitem"
              className="relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none hover:bg-accent hover:text-accent-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground"
              onClick={handleCopyCell}
            >
              <Copy className="h-3.5 w-3.5" />
              {t("query.copyValue")}
            </div>
          </div>,
          document.body
        )}
    </div>
  );
}

interface ColumnValuePanelEntry {
  value: unknown;
  key: string;
  count: number;
}

interface ColumnValuePanelProps {
  col: string;
  entries: ColumnValuePanelEntry[];
  /** null = no active filter (everything shown). Otherwise only keys in the set pass. */
  selected: Set<string> | null;
  onChange: (next: Set<string> | null) => void;
}

function ColumnValuePanel({ col, entries, selected, onChange }: ColumnValuePanelProps) {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const listRef = useRef<HTMLDivElement>(null);

  // Default is "unchecked" / no active filter. `selected === null` means the
  // user has not touched this column yet — every checkbox renders empty and all
  // rows pass the filter. Once the user checks any value, `selected` becomes a
  // whitelist Set; only rows whose value is in the Set survive.
  const selectedSet = selected ?? new Set<string>();
  const allKeys = useMemo(() => entries.map((e) => e.key), [entries]);
  const allChecked = allKeys.length > 0 && selectedSet.size === allKeys.length;

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return entries;
    return entries.filter((e) => {
      if (e.value == null) return "null".includes(q);
      return cellValueToText(e.value).toLowerCase().includes(q);
    });
  }, [entries, search]);

  const showSearch = entries.length > 5;

  // Commit helper: an empty Set is normalized to `null` so the header Filter
  // icon drops its active indicator and we stop hiding every row.
  const commit = useCallback(
    (next: Set<string>) => {
      if (next.size === 0) onChange(null);
      else onChange(next);
    },
    [onChange]
  );

  const toggleOne = useCallback(
    (key: string) => {
      const next = new Set(selectedSet);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      commit(next);
    },
    [selectedSet, commit]
  );

  const handleSelectAll = useCallback(() => onChange(new Set(allKeys)), [onChange, allKeys]);
  const handleClearAll = useCallback(() => onChange(null), [onChange]);

  if (entries.length === 0) {
    return <div className="px-3 py-6 text-xs text-muted-foreground text-center">{t("query.noResult")}</div>;
  }

  return (
    <div className="flex flex-col max-h-[360px] overflow-hidden">
      {/* Header: column name + distinct count + optional search */}
      <div className="px-3 pt-2.5 pb-2 border-b border-border shrink-0 space-y-1.5">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-semibold truncate" title={col}>
            {col}
          </span>
          <span className="text-[10px] text-muted-foreground shrink-0 tabular-nums">
            {t("query.distinctValues", { count: entries.length })}
          </span>
        </div>
        {showSearch && (
          <div className="relative">
            <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground pointer-events-none" />
            <input
              type="text"
              placeholder={t("query.filterSearchPlaceholder")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              autoFocus
              className="w-full h-7 pl-6 pr-2 text-xs rounded border border-input bg-background focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            />
          </div>
        )}
      </div>

      {/* Value list — default: no background; hover highlights the row. */}
      <div ref={listRef} className="flex-1 min-h-0 overflow-auto py-1">
        {filtered.length === 0 ? (
          <div className="px-3 py-6 text-xs text-muted-foreground text-center">{t("query.filterNoMatch")}</div>
        ) : (
          filtered.map((entry) => {
            const checked = selectedSet.has(entry.key);
            const text = cellValueToText(entry.value);
            const label =
              entry.value == null ? (
                <span className="text-muted-foreground italic">NULL</span>
              ) : entry.value === "" ? (
                <span className="text-muted-foreground italic">{t("query.filterEmptyString")}</span>
              ) : (
                text
              );
            const tooltip = entry.value == null ? "NULL" : entry.value === "" ? "(empty)" : text;
            return (
              <label
                key={entry.key}
                className="group flex items-center gap-2 px-3 py-1 text-xs font-mono cursor-pointer hover:bg-accent hover:text-accent-foreground"
                title={tooltip}
              >
                <input
                  type="checkbox"
                  className="h-3.5 w-3.5 accent-primary shrink-0 cursor-pointer"
                  checked={checked}
                  onChange={() => toggleOne(entry.key)}
                />
                <span className="flex-1 min-w-0 truncate">{label}</span>
                <span className="text-[10px] text-muted-foreground group-hover:text-accent-foreground/70 shrink-0 tabular-nums">
                  {entry.count}
                </span>
              </label>
            );
          })
        )}
      </div>

      {/* Footer: select all / clear + active count hint */}
      <div className="px-3 py-1.5 border-t border-border text-[10px] shrink-0 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="text-primary hover:underline disabled:opacity-40 disabled:no-underline"
            onClick={handleSelectAll}
            disabled={allChecked}
          >
            {t("query.filterSelectAll")}
          </button>
          <span className="text-border">|</span>
          <button
            type="button"
            className="text-primary hover:underline disabled:opacity-40 disabled:no-underline"
            onClick={handleClearAll}
            disabled={selectedSet.size === 0}
          >
            {t("query.filterClearAll")}
          </button>
        </div>
        <span className="text-muted-foreground tabular-nums">
          {t("query.filterSelectedOf", { selected: selectedSet.size, total: entries.length })}
        </span>
      </div>
    </div>
  );
}
