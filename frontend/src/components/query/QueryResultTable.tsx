import { useState, useRef, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Loader2 } from "lucide-react";

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
}

function cellKey(rowIdx: number, col: string) {
  return `${rowIdx}:${col}`;
}

export function QueryResultTable({
  columns,
  rows,
  loading,
  error,
  editable,
  edits,
  onCellEdit,
}: QueryResultTableProps) {
  const { t } = useTranslation();

  const [editingCell, setEditingCell] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Focus input when editing starts
  useEffect(() => {
    if (editingCell && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [editingCell]);

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

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="px-3 py-4 text-xs text-destructive whitespace-pre-wrap font-mono">
        {error}
      </div>
    );
  }

  if (columns.length === 0) {
    return (
      <div className="flex items-center justify-center py-8 text-xs text-muted-foreground">
        {t("query.noResult")}
      </div>
    );
  }

  return (
    <div className="overflow-auto flex-1 min-h-0">
      <table className="w-full border-collapse text-xs font-mono">
        <thead className="sticky top-0 z-10 bg-muted">
          <tr>
            {columns.map((col) => (
              <th
                key={col}
                className="border border-border px-2 py-1.5 text-left font-semibold text-muted-foreground whitespace-nowrap"
              >
                {col}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, idx) => (
            <tr
              key={idx}
              className={idx % 2 === 0 ? "bg-background" : "bg-muted/40"}
            >
              {columns.map((col) => {
                const ck = cellKey(idx, col);
                const isEdited = edits?.has(ck);
                const displayValue = isEdited ? edits!.get(ck) : row[col];
                const isEditing = editingCell === ck;

                return (
                  <td
                    key={col}
                    className={`border border-border px-2 py-1 whitespace-nowrap max-w-[400px] ${
                      isEdited
                        ? "bg-yellow-100 dark:bg-yellow-900/30"
                        : ""
                    }`}
                    title={displayValue == null ? "NULL" : String(displayValue)}
                    onDoubleClick={() => {
                      if (!editable) return;
                      setEditingCell(ck);
                    }}
                  >
                    {isEditing ? (
                      <input
                        ref={inputRef}
                        className="w-full bg-transparent outline-none border-none p-0 m-0 text-xs font-mono"
                        defaultValue={
                          displayValue == null ? "" : String(displayValue)
                        }
                        onBlur={(e) => commitEdit(idx, col, e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") {
                            commitEdit(
                              idx,
                              col,
                              (e.target as HTMLInputElement).value
                            );
                          }
                          if (e.key === "Escape") {
                            setEditingCell(null);
                          }
                        }}
                      />
                    ) : displayValue == null ? (
                      <span className="text-muted-foreground italic">
                        NULL
                      </span>
                    ) : (
                      <span className="truncate block max-w-[400px]">
                        {String(displayValue)}
                      </span>
                    )}
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
