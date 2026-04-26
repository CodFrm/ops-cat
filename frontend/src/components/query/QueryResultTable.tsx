import { useState, useRef, useEffect, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { createPortal } from "react-dom";
import {
  Loader2,
  Copy,
  ArrowUp,
  ArrowDown,
  Filter,
  FilterX,
  Search,
  ClipboardPaste,
  RefreshCw,
  CircleSlash,
  Type,
  ClipboardType,
  Trash2,
  WandSparkles,
  ClipboardList,
  CalendarClock,
  MoreHorizontal,
  Hash,
} from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "@opskat/ui";
import { toast } from "sonner";
import { cellValueToText } from "@/lib/cellValue";

export interface CellEdit {
  rowIdx: number;
  col: string;
  value: unknown; // new value
}

export interface CellActionContext {
  rowIdx: number;
  col: string;
  value: unknown;
}

export interface SelectedCellContext {
  rowIdx: number;
  col: string;
}

export interface FocusCellRequest extends SelectedCellContext {
  nonce: number;
}

export type SortDir = "asc" | "desc" | null;
export type CopyAsFormat = "insert" | "update" | "tsv-data" | "tsv-fields" | "tsv-fields-data";
export type RowDensity = "compact" | "default" | "comfortable";

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
  onSetCellValue?: (edit: CellEdit) => void;
  onPasteCell?: (edit: CellEdit) => void;
  onGenerateUuid?: (edit: CellEdit) => void;
  onCopyAs?: (format: CopyAsFormat, ctx: CellActionContext) => void;
  onFilterByCellValue?: (ctx: CellActionContext) => void;
  onSortByColumn?: (col: string, dir: Exclude<SortDir, null>) => void;
  onClearFilterSort?: () => void;
  onAddColumnFilter?: (col: string) => void;
  onDeleteRow?: (rowIdx: number) => void;
  onHideColumn?: (col: string) => void;
  onSelectedCellChange?: (cell: SelectedCellContext | null) => void;
  onRefresh?: () => void;
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
  visibleColumns?: string[];
  columnTypes?: Record<string, string>;
  rowDensity?: RowDensity;
  focusCellRequest?: FocusCellRequest | null;
  // Override the display-mode cell rendering. Does not affect edit-mode (input).
  // When provided, the returned node replaces the default NULL / String(value) span.
  renderCell?: (value: unknown, ctx: RenderCellContext) => React.ReactNode;
}

// Sentinel key used to represent NULL / undefined values in the column-filter
// Set so they don't collide with the literal string "null" etc.
const NULL_KEY = "__opskat_null_sentinel__";

const CONTEXT_MENU_ITEM_CLASS =
  "relative flex w-full cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none hover:bg-accent hover:text-accent-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground";

function valueKey(v: unknown): string {
  if (v == null) return NULL_KEY;
  return cellValueToText(v);
}

function cellKey(rowIdx: number, col: string) {
  return `${rowIdx}:${col}`;
}

type DateEditMode = "date" | "datetime";

type CellContextMenu = {
  kind: "cell";
  x: number;
  y: number;
  rowIdx: number;
  col: string;
  value: unknown;
};

type RowContextMenu = {
  kind: "row";
  x: number;
  y: number;
  rowIdx: number;
};

type ColumnContextMenu = {
  kind: "column";
  variant: "context" | "actions";
  x: number;
  y: number;
  col: string;
};

type ContextMenuState = CellContextMenu | RowContextMenu | ColumnContextMenu;

function getColumnTypeIcon(type?: string) {
  const normalized = type?.toLowerCase() ?? "";
  if (/(int|decimal|numeric|float|double|real|serial|number)/.test(normalized)) return Hash;
  if (/(date|time|timestamp)/.test(normalized)) return CalendarClock;
  return Type;
}

function padDatePart(value: string | number): string {
  return String(value).padStart(2, "0");
}

function formatDateToInputValue(value: unknown, mode: DateEditMode): string {
  const fromDate = (date: Date) => {
    const datePart = `${date.getFullYear()}-${padDatePart(date.getMonth() + 1)}-${padDatePart(date.getDate())}`;
    if (mode === "date") return datePart;
    return `${datePart}T${padDatePart(date.getHours())}:${padDatePart(date.getMinutes())}:${padDatePart(date.getSeconds())}`;
  };

  if (value instanceof Date && !Number.isNaN(value.getTime())) return fromDate(value);

  if (typeof value === "string") {
    const trimmed = value.trim();
    const match = trimmed.match(/^(\d{4})[-/](\d{1,2})[-/](\d{1,2})(?:[T\s,]+(\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?/);
    if (match) {
      const [, y, m, d, hh = "0", mm = "0", ss = "0"] = match;
      const datePart = `${y}-${padDatePart(m)}-${padDatePart(d)}`;
      if (mode === "date") return datePart;
      return `${datePart}T${padDatePart(hh)}:${padDatePart(mm)}:${padDatePart(ss)}`;
    }
  }

  return fromDate(new Date());
}

function formatDateInputValue(value: string, mode: DateEditMode): string {
  if (mode === "date") return value;
  const [datePart, timePart = "00:00:00"] = value.split("T");
  const [hh = "00", mm = "00", ss = "00"] = timePart.split(":");
  return `${datePart} ${padDatePart(hh)}:${padDatePart(mm)}:${padDatePart(ss)}`;
}

function getDateEditMode(col: string, type?: string, value?: unknown): DateEditMode | null {
  const normalizedType = type?.toLowerCase() ?? "";
  if (/\b(date)\b/.test(normalizedType) && !/(time|timestamp|datetime)/.test(normalizedType)) return "date";
  if (/(timestamp|datetime|time)/.test(normalizedType)) return "datetime";

  const normalizedCol = col.toLowerCase();
  const dateLikeName =
    /(^|_)(date|time)$/.test(normalizedCol) || /(^|_)(created|updated|deleted)_at$/.test(normalizedCol);
  if (!dateLikeName) return null;
  if (value == null || value instanceof Date) return "datetime";
  if (typeof value !== "string") return null;
  if (/^\d{4}[-/]\d{1,2}[-/]\d{1,2}(?:[T\s,]+\d{1,2}:\d{1,2}(?::\d{1,2})?)?/.test(value.trim())) {
    return value.includes(":") ? "datetime" : "date";
  }
  return null;
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
  onSetCellValue,
  onPasteCell,
  onGenerateUuid,
  onCopyAs,
  onFilterByCellValue,
  onSortByColumn,
  onClearFilterSort,
  onAddColumnFilter,
  onDeleteRow,
  onHideColumn,
  onSelectedCellChange,
  onRefresh,
  showRowNumber,
  rowNumberOffset = 0,
  sortColumn: controlledSortCol,
  sortDir: controlledSortDir,
  onSortChange,
  enableColumnFilter,
  visibleColumns,
  columnTypes,
  rowDensity = "default",
  focusCellRequest,
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
  const displayColumns = useMemo(
    () => (visibleColumns ? columns.filter((col) => visibleColumns.includes(col)) : columns),
    [columns, visibleColumns]
  );
  const headerPaddingClass = rowDensity === "compact" ? "py-1" : rowDensity === "comfortable" ? "py-2" : "py-1.5";
  const cellPaddingClass = rowDensity === "compact" ? "py-0.5" : rowDensity === "comfortable" ? "py-2" : "py-1";

  // Reset all filters whenever the underlying columns/rows change (new query /
  // page / refresh), otherwise stale keys could silently hide everything.
  useEffect(() => {
    setColumnFilters(new Map());
  }, [columns, rows]);

  // Distinct values per column, memoized so the popover doesn't recompute while
  // checkboxes are being toggled.
  const columnDistincts = useMemo(() => {
    const map = new Map<string, { value: unknown; key: string; count: number }[]>();
    for (const col of displayColumns) {
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
  }, [displayColumns, rows]);

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
  const [selectedRowIdx, setSelectedRowIdx] = useState<number | null>(null);
  const [selectedColumn, setSelectedColumn] = useState<string | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // Context menu state
  const [ctxMenu, setCtxMenu] = useState<ContextMenuState | null>(null);
  const [dateEditor, setDateEditor] = useState<{
    rowIdx: number;
    col: string;
    mode: DateEditMode;
    value: string;
  } | null>(null);
  const ctxMenuRef = useRef<HTMLDivElement>(null);

  // Reset local sort and column widths when columns change
  useEffect(() => {
    setLocalSortCol(null);
    setLocalSortDir(null);
    setColWidths({});
    setSelectedCell(null);
    setSelectedRowIdx(null);
    setSelectedColumn(null);
    onSelectedCellChange?.(null);
    setEditingCell(null);
  }, [columns, onSelectedCellChange]);

  // Reset selection / editing when row set changes (paging, refresh, filter)
  useEffect(() => {
    setSelectedCell(null);
    setSelectedRowIdx(null);
    setSelectedColumn(null);
    onSelectedCellChange?.(null);
    setEditingCell(null);
  }, [rows, onSelectedCellChange]);

  useEffect(() => {
    if (!focusCellRequest) return;
    if (!rows[focusCellRequest.rowIdx] || !displayColumns.includes(focusCellRequest.col)) return;
    setSelectedCell({ origIdx: focusCellRequest.rowIdx, col: focusCellRequest.col });
    setSelectedRowIdx(null);
    setSelectedColumn(null);
    setEditingCell(cellKey(focusCellRequest.rowIdx, focusCellRequest.col));
    onSelectedCellChange?.({ rowIdx: focusCellRequest.rowIdx, col: focusCellRequest.col });
  }, [displayColumns, focusCellRequest, onSelectedCellChange, rows]);

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

  const setCellValueHandler = onSetCellValue ?? onCellEdit;
  const pasteCellHandler = onPasteCell ?? onSetCellValue ?? onCellEdit;
  const uuidCellHandler = onGenerateUuid ?? onSetCellValue ?? onCellEdit;
  const canSetCellValue = !!editable && !!setCellValueHandler;
  const canPasteCell = !!editable && !!pasteCellHandler;
  const canGenerateUuid = !!editable && !!uuidCellHandler;
  const dateEditMode =
    ctxMenu?.kind === "cell" ? getDateEditMode(ctxMenu.col, columnTypes?.[ctxMenu.col], ctxMenu.value) : null;
  const canSetDateTime = canSetCellValue && !!dateEditMode;
  const menuColumn = ctxMenu?.kind === "column" ? ctxMenu.col : ctxMenu?.kind === "cell" ? ctxMenu.col : null;

  const handleCopyCell = useCallback(async () => {
    if (!ctxMenu) return;
    try {
      const text =
        ctxMenu.kind === "row"
          ? displayColumns.map((col) => cellValueToText(rows[ctxMenu.rowIdx]?.[col])).join("\t")
          : ctxMenu.kind === "column"
            ? sortedIndices.map((rowIdx) => cellValueToText(rows[rowIdx]?.[ctxMenu.col])).join("\n")
            : cellValueToText(ctxMenu.value);
      await navigator.clipboard.writeText(text);
      toast.success(t("query.copied"));
    } catch (e) {
      toast.error(String(e));
    } finally {
      setCtxMenu(null);
    }
  }, [ctxMenu, displayColumns, rows, sortedIndices, t]);

  const handleCopyFieldName = useCallback(async () => {
    const col = ctxMenu?.kind === "cell" || ctxMenu?.kind === "column" ? ctxMenu.col : null;
    if (!col) return;
    try {
      await navigator.clipboard.writeText(col);
      toast.success(t("query.copied"));
    } catch (e) {
      toast.error(String(e));
    } finally {
      setCtxMenu(null);
    }
  }, [ctxMenu, t]);

  const handleSetCellValue = useCallback(
    (value: unknown) => {
      if (!ctxMenu || ctxMenu.kind !== "cell") return;
      const edit = { rowIdx: ctxMenu.rowIdx, col: ctxMenu.col, value };
      setCellValueHandler?.(edit);
      setCtxMenu(null);
    },
    [ctxMenu, setCellValueHandler]
  );

  const handlePasteCell = useCallback(async () => {
    if (!ctxMenu || ctxMenu.kind !== "cell") return;
    try {
      const text = await navigator.clipboard.readText();
      const edit = { rowIdx: ctxMenu.rowIdx, col: ctxMenu.col, value: text };
      pasteCellHandler?.(edit);
    } catch (e) {
      toast.error(String(e));
    } finally {
      setCtxMenu(null);
    }
  }, [ctxMenu, pasteCellHandler]);

  const handleGenerateUuid = useCallback(() => {
    if (!ctxMenu || ctxMenu.kind !== "cell") return;
    uuidCellHandler?.({ rowIdx: ctxMenu.rowIdx, col: ctxMenu.col, value: crypto.randomUUID() });
    setCtxMenu(null);
  }, [ctxMenu, uuidCellHandler]);

  const handleCopyAs = useCallback(
    (format: CopyAsFormat) => {
      if (!ctxMenu) return;
      const fallbackRowIdx = sortedIndices[0] ?? 0;
      onCopyAs?.(format, {
        rowIdx: ctxMenu.kind === "cell" || ctxMenu.kind === "row" ? ctxMenu.rowIdx : fallbackRowIdx,
        col: ctxMenu.kind === "cell" || ctxMenu.kind === "column" ? ctxMenu.col : (displayColumns[0] ?? ""),
        value:
          ctxMenu.kind === "cell"
            ? ctxMenu.value
            : ctxMenu.kind === "column"
              ? rows[fallbackRowIdx]?.[ctxMenu.col]
              : rows[ctxMenu.rowIdx],
      });
      setCtxMenu(null);
    },
    [ctxMenu, displayColumns, onCopyAs, rows, sortedIndices]
  );

  const handleOpenDateEditor = useCallback(() => {
    if (!ctxMenu || ctxMenu.kind !== "cell") return;
    const mode = getDateEditMode(ctxMenu.col, columnTypes?.[ctxMenu.col], ctxMenu.value);
    if (!mode) return;
    setDateEditor({
      rowIdx: ctxMenu.rowIdx,
      col: ctxMenu.col,
      mode,
      value: formatDateToInputValue(ctxMenu.value, mode),
    });
    setCtxMenu(null);
  }, [columnTypes, ctxMenu]);

  const handleCommitDateEditor = useCallback(() => {
    if (!dateEditor) return;
    setCellValueHandler?.({
      rowIdx: dateEditor.rowIdx,
      col: dateEditor.col,
      value: formatDateInputValue(dateEditor.value, dateEditor.mode),
    });
    setDateEditor(null);
  }, [dateEditor, setCellValueHandler]);

  const handleRefreshFromMenu = useCallback(() => {
    onRefresh?.();
    setCtxMenu(null);
  }, [onRefresh]);

  const handleFilterByCellValue = useCallback(() => {
    if (!ctxMenu || ctxMenu.kind !== "cell") return;
    onFilterByCellValue?.({ rowIdx: ctxMenu.rowIdx, col: ctxMenu.col, value: ctxMenu.value });
    setCtxMenu(null);
  }, [ctxMenu, onFilterByCellValue]);

  const handleSortByColumn = useCallback(
    (dir: Exclude<SortDir, null>) => {
      const col = ctxMenu?.kind === "cell" || ctxMenu?.kind === "column" ? ctxMenu.col : null;
      if (!col) return;
      if (onSortByColumn) onSortByColumn(col, dir);
      else if (isControlledSort) onSortChange?.(col, dir);
      else {
        setLocalSortCol(col);
        setLocalSortDir(dir);
      }
      setCtxMenu(null);
    },
    [ctxMenu, isControlledSort, onSortByColumn, onSortChange]
  );

  const handleAddColumnFilter = useCallback(() => {
    if (!menuColumn) return;
    onAddColumnFilter?.(menuColumn);
    setCtxMenu(null);
  }, [menuColumn, onAddColumnFilter]);

  const handleHideColumn = useCallback(() => {
    if (!menuColumn) return;
    onHideColumn?.(menuColumn);
    setCtxMenu(null);
  }, [menuColumn, onHideColumn]);

  const handleClearFilterSort = useCallback(() => {
    onClearFilterSort?.();
    if (!onClearFilterSort) {
      setLocalSortCol(null);
      setLocalSortDir(null);
    }
    setCtxMenu(null);
  }, [onClearFilterSort]);

  const handleDeleteRow = useCallback(() => {
    if (!ctxMenu || ctxMenu.kind === "column") return;
    onDeleteRow?.(ctxMenu.rowIdx);
    setCtxMenu(null);
  }, [ctxMenu, onDeleteRow]);

  const selectCell = useCallback(
    (origIdx: number, col: string) => {
      setSelectedCell({ origIdx, col });
      setSelectedRowIdx(null);
      setSelectedColumn(null);
      onSelectedCellChange?.({ rowIdx: origIdx, col });
      containerRef.current?.focus();
    },
    [onSelectedCellChange]
  );

  const selectRow = useCallback(
    (origIdx: number) => {
      setSelectedCell(null);
      setSelectedRowIdx(origIdx);
      setSelectedColumn(null);
      onSelectedCellChange?.({ rowIdx: origIdx, col: "" });
      containerRef.current?.focus();
    },
    [onSelectedCellChange]
  );

  const selectColumn = useCallback(
    (col: string) => {
      setSelectedCell(null);
      setSelectedRowIdx(null);
      setSelectedColumn(col);
      onSelectedCellChange?.(null);
      containerRef.current?.focus();
    },
    [onSelectedCellChange]
  );

  const handleCellContextMenu = useCallback(
    (e: React.MouseEvent, origIdx: number, col: string, value: unknown) => {
      e.preventDefault();
      e.stopPropagation();
      selectCell(origIdx, col);
      setCtxMenu({ kind: "cell", x: e.clientX, y: e.clientY, rowIdx: origIdx, col, value });
    },
    [selectCell]
  );

  const handleRowContextMenu = useCallback(
    (e: React.MouseEvent, origIdx: number) => {
      e.preventDefault();
      e.stopPropagation();
      selectRow(origIdx);
      setCtxMenu({ kind: "row", x: e.clientX, y: e.clientY, rowIdx: origIdx });
    },
    [selectRow]
  );

  const handleColumnContextMenu = useCallback(
    (e: React.MouseEvent, col: string) => {
      e.preventDefault();
      e.stopPropagation();
      selectColumn(col);
      setCtxMenu({ kind: "column", variant: "context", x: e.clientX, y: e.clientY, col });
    },
    [selectColumn]
  );

  const handleColumnActionsClick = useCallback(
    (e: React.MouseEvent, col: string) => {
      e.preventDefault();
      e.stopPropagation();
      selectColumn(col);
      const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
      setCtxMenu({ kind: "column", variant: "actions", x: rect.left, y: rect.bottom, col });
    },
    [selectColumn]
  );

  const handleCellClick = useCallback(
    (origIdx: number, col: string) => {
      selectCell(origIdx, col);
    },
    [selectCell]
  );

  // Arrow key navigation + Enter/F2 to edit + Escape to deselect/cancel
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      // When editing, the input owns key events — only Escape handled here already
      // (the input's onKeyDown calls setEditingCell(null) on Escape).
      if (editingCell) return;
      if (!selectedCell) {
        if ((selectedRowIdx != null || selectedColumn != null) && e.key === "Escape") {
          e.preventDefault();
          setSelectedRowIdx(null);
          setSelectedColumn(null);
          onSelectedCellChange?.(null);
        }
        return;
      }

      const colIdx = displayColumns.indexOf(selectedCell.col);
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
          nextColIdx = Math.min(displayColumns.length - 1, colIdx + 1);
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
          onSelectedCellChange?.(null);
          return;
        default:
          return;
      }

      e.preventDefault();
      selectCell(sortedIndices[nextDisplayIdx], displayColumns[nextColIdx]);
    },
    [
      editingCell,
      selectedCell,
      selectedRowIdx,
      selectedColumn,
      sortedIndices,
      displayColumns,
      editable,
      onSelectedCellChange,
      selectCell,
    ]
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
              {displayColumns.map((col) => {
                const isSorted = sortCol === col;
                const width = colWidths[col];
                const isColumnSelected = selectedColumn === col;
                const typeText = columnTypes?.[col];
                const TypeIcon = getColumnTypeIcon(typeText);
                return (
                  <th
                    key={col}
                    data-column-header-key={col}
                    data-column-selected={isColumnSelected ? col : undefined}
                    className={`group relative border border-border px-2 ${headerPaddingClass} text-left font-semibold whitespace-nowrap select-none ${
                      isColumnSelected
                        ? "bg-primary/25 text-foreground ring-2 ring-inset ring-primary/50"
                        : "text-muted-foreground"
                    }`}
                    style={width ? { width: `${width}px`, minWidth: `${width}px` } : undefined}
                    title={col}
                    onClick={() => selectColumn(col)}
                    onContextMenu={(e) => handleColumnContextMenu(e, col)}
                  >
                    <div className="flex items-start gap-1">
                      <div className="min-w-0 flex-1">
                        <div className="flex min-w-0 items-center gap-1">
                          <span className="truncate text-sm text-foreground">{col}</span>
                          {canSort &&
                            (isSorted && sortDir === "asc" ? (
                              <ArrowUp className="h-3 w-3 shrink-0" />
                            ) : isSorted && sortDir === "desc" ? (
                              <ArrowDown className="h-3 w-3 shrink-0" />
                            ) : null)}
                        </div>
                        {typeText && (
                          <div className="mt-1 flex min-w-0 items-center gap-1 text-xs font-normal text-blue-700/80 dark:text-blue-300/80">
                            <TypeIcon className="h-3.5 w-3.5 shrink-0" />
                            <span className="truncate">{typeText}</span>
                          </div>
                        )}
                      </div>
                      <button
                        type="button"
                        className="shrink-0 rounded px-0.5 text-primary opacity-60 hover:bg-accent hover:opacity-100 focus:opacity-100"
                        title={`${t("query.columnActions")}:${col}`}
                        onClick={(e) => handleColumnActionsClick(e, col)}
                      >
                        <MoreHorizontal className="h-4 w-4" />
                      </button>
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
              const isRowSelected = selectedRowIdx === origIdx;
              return (
                <tr key={origIdx} className={idx % 2 === 0 ? "bg-background" : "bg-muted/40"}>
                  {showRowNumber && (
                    <td
                      data-row-header-key={origIdx}
                      data-row-selected={isRowSelected ? "true" : undefined}
                      className={`border border-border px-2 py-1 text-center text-muted-foreground whitespace-nowrap w-[50px] cursor-default select-none ${
                        isRowSelected
                          ? "bg-primary/15 text-foreground ring-2 ring-inset ring-primary/50 relative z-10"
                          : ""
                      }`}
                      onClick={() => selectRow(origIdx)}
                      onContextMenu={(e) => handleRowContextMenu(e, origIdx)}
                    >
                      {rowNumberOffset + origIdx + 1}
                    </td>
                  )}
                  {displayColumns.map((col) => {
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
                        data-row-selected={isRowSelected ? "true" : undefined}
                        data-column-selected={selectedColumn === col ? col : undefined}
                        className={`border border-border px-2 ${cellPaddingClass} whitespace-nowrap cursor-default ${
                          isEdited ? "bg-yellow-100 dark:bg-yellow-900/30" : ""
                        } ${isRowSelected || selectedColumn === col ? "bg-primary/15" : ""} ${focusClass}`}
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
            role="menu"
          >
            {ctxMenu.kind === "column" && ctxMenu.variant === "actions" ? (
              <>
                {(onSortByColumn || canSort) && (
                  <>
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={() => handleSortByColumn("asc")}
                    >
                      <ArrowUp className="h-3.5 w-3.5" />
                      {t("query.sortAsc")}
                    </button>
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={() => handleSortByColumn("desc")}
                    >
                      <ArrowDown className="h-3.5 w-3.5" />
                      {t("query.sortDesc")}
                    </button>
                  </>
                )}
                {(onClearFilterSort || canSort) && (
                  <button
                    type="button"
                    role="menuitem"
                    className={CONTEXT_MENU_ITEM_CLASS}
                    onClick={handleClearFilterSort}
                  >
                    <FilterX className="h-3.5 w-3.5" />
                    {t("query.removeAllSorts")}
                  </button>
                )}
                {onAddColumnFilter && (
                  <>
                    <div className="my-1 h-px bg-border" />
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={handleAddColumnFilter}
                    >
                      <Filter className="h-3.5 w-3.5" />
                      {t("query.addFilter")}
                    </button>
                  </>
                )}
              </>
            ) : (
              <>
                {ctxMenu.kind === "cell" && canSetCellValue && (
                  <>
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={() => handleSetCellValue("")}
                    >
                      <Type className="h-3.5 w-3.5" />
                      {t("query.setEmptyString")}
                    </button>
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={() => handleSetCellValue(null)}
                    >
                      <CircleSlash className="h-3.5 w-3.5" />
                      {t("query.setNull")}
                    </button>
                    {canSetDateTime && (
                      <button
                        type="button"
                        role="menuitem"
                        className={CONTEXT_MENU_ITEM_CLASS}
                        onClick={handleOpenDateEditor}
                      >
                        <CalendarClock className="h-3.5 w-3.5" />
                        {t("query.setDateTime")}
                      </button>
                    )}
                  </>
                )}
                <button type="button" role="menuitem" className={CONTEXT_MENU_ITEM_CLASS} onClick={handleCopyCell}>
                  <Copy className="h-3.5 w-3.5" />
                  {t("query.copyValue")}
                </button>
                {(ctxMenu.kind === "cell" || ctxMenu.kind === "column") && (
                  <button
                    type="button"
                    role="menuitem"
                    className={CONTEXT_MENU_ITEM_CLASS}
                    onClick={handleCopyFieldName}
                  >
                    <ClipboardType className="h-3.5 w-3.5" />
                    {t("query.copyFieldName")}
                  </button>
                )}
                {ctxMenu.kind === "cell" && canPasteCell && (
                  <button type="button" role="menuitem" className={CONTEXT_MENU_ITEM_CLASS} onClick={handlePasteCell}>
                    <ClipboardPaste className="h-3.5 w-3.5" />
                    {t("query.pasteValue")}
                  </button>
                )}
                {ctxMenu.kind === "cell" && canGenerateUuid && (
                  <button
                    type="button"
                    role="menuitem"
                    className={CONTEXT_MENU_ITEM_CLASS}
                    onClick={handleGenerateUuid}
                  >
                    <WandSparkles className="h-3.5 w-3.5" />
                    {t("query.generateUuid")}
                  </button>
                )}
                {onCopyAs && (
                  <>
                    <div className="my-1 h-px bg-border" />
                    <div className="px-2 py-1 text-[11px] font-medium text-muted-foreground">{t("query.copyAs")}</div>
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={() => handleCopyAs("insert")}
                    >
                      <ClipboardList className="h-3.5 w-3.5" />
                      {t("query.copyAsInsert")}
                    </button>
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={() => handleCopyAs("update")}
                    >
                      <ClipboardList className="h-3.5 w-3.5" />
                      {t("query.copyAsUpdate")}
                    </button>
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={() => handleCopyAs("tsv-data")}
                    >
                      <ClipboardList className="h-3.5 w-3.5" />
                      {t("query.copyAsTsvData")}
                    </button>
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={() => handleCopyAs("tsv-fields")}
                    >
                      <ClipboardList className="h-3.5 w-3.5" />
                      {t("query.copyAsTsvFields")}
                    </button>
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={() => handleCopyAs("tsv-fields-data")}
                    >
                      <ClipboardList className="h-3.5 w-3.5" />
                      {t("query.copyAsTsvFieldsAndData")}
                    </button>
                  </>
                )}
                {ctxMenu.kind === "column" && ctxMenu.variant === "context" && onHideColumn && (
                  <>
                    <div className="my-1 h-px bg-border" />
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={handleHideColumn}
                    >
                      <FilterX className="h-3.5 w-3.5" />
                      {t("query.hideColumn")}
                    </button>
                    <div className="my-1 h-px bg-border" />
                    <button type="button" role="menuitem" className={CONTEXT_MENU_ITEM_CLASS}>
                      <span className="w-3.5 text-center">✓</span>
                      {t("query.showFieldType")}
                    </button>
                  </>
                )}
                {ctxMenu.kind === "cell" && onFilterByCellValue && (
                  <button
                    type="button"
                    role="menuitem"
                    className={CONTEXT_MENU_ITEM_CLASS}
                    onClick={handleFilterByCellValue}
                  >
                    <Filter className="h-3.5 w-3.5" />
                    {t("query.filterByCellValue")}
                  </button>
                )}
                {ctxMenu.kind === "cell" && onSortByColumn && (
                  <>
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={() => handleSortByColumn("asc")}
                    >
                      <ArrowUp className="h-3.5 w-3.5" />
                      {t("query.sortAscending")}
                    </button>
                    <button
                      type="button"
                      role="menuitem"
                      className={CONTEXT_MENU_ITEM_CLASS}
                      onClick={() => handleSortByColumn("desc")}
                    >
                      <ArrowDown className="h-3.5 w-3.5" />
                      {t("query.sortDescending")}
                    </button>
                  </>
                )}
                {ctxMenu.kind !== "column" && onClearFilterSort && (
                  <button
                    type="button"
                    role="menuitem"
                    className={CONTEXT_MENU_ITEM_CLASS}
                    onClick={handleClearFilterSort}
                  >
                    <FilterX className="h-3.5 w-3.5" />
                    {t("query.clearFilterSort")}
                  </button>
                )}
                {onRefresh && (
                  <button
                    type="button"
                    role="menuitem"
                    className={CONTEXT_MENU_ITEM_CLASS}
                    onClick={handleRefreshFromMenu}
                  >
                    <RefreshCw className="h-3.5 w-3.5" />
                    {t("query.refreshTable")}
                  </button>
                )}
                {ctxMenu.kind !== "column" && editable && onDeleteRow && (
                  <button
                    type="button"
                    role="menuitem"
                    className={`${CONTEXT_MENU_ITEM_CLASS} text-destructive hover:text-destructive`}
                    onClick={handleDeleteRow}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    {t("query.deleteRecord")}
                  </button>
                )}
              </>
            )}
          </div>,
          document.body
        )}
      {dateEditor &&
        createPortal(
          <div
            className="fixed z-50 w-72 rounded-md border bg-popover p-3 text-popover-foreground shadow-lg"
            style={{ top: "50%", left: "50%", transform: "translate(-50%, -50%)" }}
            role="dialog"
            aria-label={t("query.dateTimeDialogTitle")}
          >
            <div className="mb-2 text-sm font-medium">{t("query.dateTimeDialogTitle")}</div>
            <label className="block">
              <span className="sr-only">{t("query.dateTimeValue")}</span>
              <input
                aria-label={t("query.dateTimeValue")}
                type={dateEditor.mode === "date" ? "date" : "datetime-local"}
                step={dateEditor.mode === "date" ? undefined : 1}
                className="h-8 w-full rounded-md border border-input bg-background px-2 text-sm outline-none focus:ring-2 focus:ring-ring"
                value={dateEditor.value}
                onChange={(e) => setDateEditor((prev) => (prev ? { ...prev, value: e.target.value } : prev))}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleCommitDateEditor();
                  if (e.key === "Escape") setDateEditor(null);
                }}
                autoFocus
              />
            </label>
            <div className="mt-3 flex justify-end gap-2">
              <button
                type="button"
                className="rounded-md border px-3 py-1.5 text-sm hover:bg-accent"
                onClick={() => setDateEditor(null)}
              >
                {t("action.cancel")}
              </button>
              <button
                type="button"
                className="rounded-md bg-primary px-3 py-1.5 text-sm text-primary-foreground hover:bg-primary/90"
                onClick={handleCommitDateEditor}
              >
                {t("action.ok")}
              </button>
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
  const selectedSet = useMemo(() => selected ?? new Set<string>(), [selected]);
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
