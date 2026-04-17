import { useState, useRef, useEffect, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { createPortal } from "react-dom";
import { Loader2, Copy, ArrowUp, ArrowDown, ArrowUpDown } from "lucide-react";
import { toast } from "sonner";

export interface CellEdit {
  rowIdx: number;
  col: string;
  value: unknown; // new value
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
}

function cellKey(rowIdx: number, col: string) {
  return `${rowIdx}:${col}`;
}

type SortDir = "asc" | "desc" | null;

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
}: QueryResultTableProps) {
  const { t } = useTranslation();

  const [editingCell, setEditingCell] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Sort state
  const [sortCol, setSortCol] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<SortDir>(null);

  // Column resize state
  const [colWidths, setColWidths] = useState<Record<string, number>>({});

  // Context menu state
  const [ctxMenu, setCtxMenu] = useState<{ x: number; y: number; value: unknown } | null>(null);
  const ctxMenuRef = useRef<HTMLDivElement>(null);

  // Reset sort and column widths when columns change
  useEffect(() => {
    setSortCol(null);
    setSortDir(null);
    setColWidths({});
  }, [columns]);

  // Sorted row indices (sort on the original indices to preserve edit mapping)
  const sortedIndices = useMemo(() => {
    const indices = rows.map((_, i) => i);
    if (!sortCol || !sortDir) return indices;
    return indices.sort((a, b) => {
      const cmp = compareValues(rows[a][sortCol], rows[b][sortCol]);
      return sortDir === "asc" ? cmp : -cmp;
    });
  }, [rows, sortCol, sortDir]);

  const toggleSort = useCallback(
    (col: string) => {
      if (sortCol !== col) {
        setSortCol(col);
        setSortDir("asc");
      } else if (sortDir === "asc") {
        setSortDir("desc");
      } else {
        setSortCol(null);
        setSortDir(null);
      }
    },
    [sortCol, sortDir]
  );

  // Column resize handler
  const handleResizeStart = useCallback((e: React.MouseEvent, col: string) => {
    e.preventDefault();
    e.stopPropagation();
    const startX = e.clientX;
    const th = (e.target as HTMLElement).closest("th")!;
    const startWidth = th.offsetWidth;

    const onMouseMove = (me: MouseEvent) => {
      const newWidth = Math.max(50, startWidth + me.clientX - startX);
      setColWidths((prev) => ({ ...prev, [col]: newWidth }));
    };

    const onMouseUp = () => {
      document.removeEventListener("mousemove", onMouseMove);
      document.removeEventListener("mouseup", onMouseUp);
      document.body.style.removeProperty("cursor");
      document.body.style.removeProperty("user-select");
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
    const text = ctxMenu.value == null ? "" : String(ctxMenu.value);
    navigator.clipboard.writeText(text);
    toast.success(t("query.copied"));
    setCtxMenu(null);
  }, [ctxMenu, t]);

  const handleCellContextMenu = useCallback((e: React.MouseEvent, value: unknown) => {
    e.preventDefault();
    e.stopPropagation();
    setCtxMenu({ x: e.clientX, y: e.clientY, value });
  }, []);

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
      <div className="flex-1 overflow-auto min-h-0 query-table-scroll">
        <table className="border-collapse text-xs font-mono">
          <thead className="bg-muted sticky top-0 z-10">
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
                    onClick={() => !editable && toggleSort(col)}
                    title={editable ? col : t("query.sortColumn")}
                  >
                    <span className={`inline-flex items-center gap-1 ${!editable ? "cursor-pointer" : ""}`}>
                      {col}
                      {!editable &&
                        (isSorted && sortDir === "asc" ? (
                          <ArrowUp className="h-3 w-3" />
                        ) : isSorted && sortDir === "desc" ? (
                          <ArrowDown className="h-3 w-3" />
                        ) : (
                          <ArrowUpDown className="h-3 w-3 opacity-0 group-hover:opacity-30" />
                        ))}
                    </span>
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
                    const width = colWidths[col];

                    return (
                      <td
                        key={col}
                        className={`border border-border px-2 py-1 whitespace-nowrap ${
                          isEdited ? "bg-yellow-100 dark:bg-yellow-900/30" : ""
                        }`}
                        style={
                          width
                            ? { width: `${width}px`, minWidth: `${width}px`, maxWidth: `${width}px` }
                            : { maxWidth: "400px" }
                        }
                        title={displayValue == null ? "NULL" : String(displayValue)}
                        onDoubleClick={() => {
                          if (!editable) return;
                          setEditingCell(ck);
                        }}
                        onContextMenu={(e) => handleCellContextMenu(e, displayValue)}
                      >
                        {isEditing ? (
                          <input
                            ref={inputRef}
                            className="w-full bg-transparent outline-none border-none p-0 m-0 text-xs font-mono"
                            defaultValue={displayValue == null ? "" : String(displayValue)}
                            onBlur={(e) => commitEdit(origIdx, col, e.target.value)}
                            onKeyDown={(e) => {
                              if (e.key === "Enter") {
                                // IME 合成中：让 Enter 作为候选词确认，不提交编辑
                                // eslint-disable-next-line @typescript-eslint/no-deprecated
                                if (e.nativeEvent.isComposing || e.keyCode === 229) return;
                                commitEdit(origIdx, col, (e.target as HTMLInputElement).value);
                              }
                              if (e.key === "Escape") {
                                setEditingCell(null);
                              }
                            }}
                          />
                        ) : displayValue == null ? (
                          <span className="text-muted-foreground italic">NULL</span>
                        ) : (
                          <span className="truncate block">{String(displayValue)}</span>
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
